package optolink

import (
	"fmt"
	"io"
	"time"

	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
)

// VitoState is a type for possible states of the VitoFsm state machine
type VitoState byte

const (
	unknown  VitoState = iota // Maybe wait for ENQ/0x05
	reset                     // Send EOT/0x04
	resetAck                  // Wait for ENQ
	resetP300
	resetP300Ack
	idle
	sendKwStart
	sendKw
	recvKw
	swP300  // Send SYN NUL NUL / 0x16 0x00 0x00 to switch to P300
	waitAck // Wait for ACK / 0x06
	wait    // Occasionally send SNY NUL NULL
	sendP300
	sendP300Ack
	recvP300
	recvP300Ack
	recvP300Nak
)

// CommandType sets the command type for GWG protocol, which has a limited address space.
// KW uses kw..., P300 p300 command types
type CommandType byte

const (
	p300ReadData     CommandType = 0x01
	p300WriteData    CommandType = 0x02
	p300FunctionCall CommandType = 0x07

	kwRead  CommandType = 0xf7 // The "normal" read type for KW
	kwWrite CommandType = 0xf4 // The "normal" write type for KW

	virtualRead             CommandType = 0xc7
	virtualWrite            CommandType = 0xc4
	physicalRead            CommandType = 0xcb
	physicalWrite           CommandType = 0xc8
	eepromRead              CommandType = 0xae
	eepromWrite             CommandType = 0xad
	physicalXramRead        CommandType = 0xc5
	physicalXramWrite       CommandType = 0xc3
	physicalPortRead        CommandType = 0x6e
	physicalPortWrite       CommandType = 0x6d
	physicalBeRead          CommandType = 0x9e
	physicalBeWrite         CommandType = 0x9d
	physicalKmbusRAMRead    CommandType = 0x33
	physicalKmBusEepromRead CommandType = 0x43
)

/*
	inject []struct {
		state      vitoState
		vals       []byte
		injectFunc *func()
		ejectFunc  *func()
	}
*/

func init() {
	log.SetLevel(log.DebugLevel)
}

// FsmCmd holds a command for the VitoFsm state machine
type FsmCmd struct {
	ID        uuid.UUID
	Command   CommandType
	Address   [2]byte
	Args      []byte
	ResultLen byte
}

// FsmResult is the result type for a previously issued FsmCmd command
type FsmResult struct {
	ID   uuid.UUID
	Err  error
	Body []byte
}

// Crc8 computes a checksum
func Crc8(b []byte) byte {
	crc := byte(0)
	for i := 0; i < len(b); i++ {
		crc += b[i]
	}
	return crc
}

func prepareCmd(cmd *FsmCmd, state VitoState) (b []byte, err error) {
	if state == sendP300 {
		if cmd.Command == kwRead {
			cmd.Command = p300ReadData
		} else if cmd.Command == kwWrite {
			cmd.Command = p300WriteData
		}

		switch cmd.Command {
		case p300ReadData:
			b = []byte{0x41, byte(5), 0x00, 0x01, cmd.Address[0], cmd.Address[1], cmd.ResultLen}
		case p300WriteData:
			b = []byte{0x41, byte((len(cmd.Args) + 5)), 0x00, 0x02, cmd.Address[0], cmd.Address[1], byte(len(cmd.Args))}
			b = append(b, cmd.Args...)
			cmd.ResultLen = byte(len(cmd.Args))
		case p300FunctionCall:
			// b = []byte{0x41, byte((len(cmd.Args) + 5)), 0x00, 0x07, cmd.Address[0], cmd.Address[1], cmd.ResultLen}
			err = fmt.Errorf("Not implemented: p300FunctionCall")
			return nil, err
		default:
			err = fmt.Errorf("Not implemented: %v (GWG protocol?)", cmd.Command)
			return nil, err
		}
		crc := Crc8(b[1:]) // Omit start byte
		b = append(b, crc)

		return b, err
	}

	if state == sendKw {
		if cmd.Command == p300ReadData {
			cmd.Command = kwRead
		} else if cmd.Command == p300WriteData {
			cmd.Command = kwWrite
		}

		switch cmd.Command {
		case kwRead:
			b = []byte{byte(cmd.Command), cmd.Address[0], cmd.Address[1], cmd.ResultLen}
		case kwWrite:
			b = []byte{byte(cmd.Command), cmd.Address[0], cmd.Address[1], byte(len(cmd.Args))}
			b = append(b, cmd.Args...)
			cmd.ResultLen = 1
		default:
			err = fmt.Errorf("Not implemented: %v (GWG protocol or P300 function call?)", cmd.Command)
			return nil, err
		}

		return b, err
	}

	err = fmt.Errorf("Could not prepare command for state %v", state)
	return nil, err
}

// VitoFsm handles the state machine for the KW and P300 protocols
func VitoFsm(device io.ReadWriter, cmdChan <-chan FsmCmd, resChan chan<- FsmResult) error { //, peer *io.ReadWriter, inChan <-chan byte, outChan chan<- byte) {
	var state, prevstate VitoState
	state, prevstate = unknown, unknown
	lastSyn, lastEnq := time.Now(), time.Now()
	c := make(chan byte)
	e := make(chan error)

	failCount := 0
	canP300 := true

	var cmd FsmCmd
	hasCmd := false

	waitforbytes := func(i int) ([]byte, error) {
		if i == 0 {
			return nil, nil
		}
		timeOutCount := 0
		var a []byte
		var b byte
		for {
			select {
			case b = <-c:
				// log.Debugf("waitforbytes: appending '%# 0x' (a='%# 0x')", b, a)
				a = append(a, b)
				if len(a) == i {
					// log.Debugf("TimeoutCount: %v", timeOutCount)
					return a, nil
				}
				if len(a) > i {
					return a, fmt.Errorf("Received %v bytes, expected %v", len(a), i)
				}
			case <-time.After(40 * time.Millisecond):
				timeOutCount++
				if timeOutCount > 2 && len(a) > 1 {
					// Subsequent bytes should be received in short time
					return a, fmt.Errorf("Timed out (%v times) on single byte after receiving %v bytes, expected %v", timeOutCount, len(a), i)
				}
				if timeOutCount > (150 + i) {
					// Timeout for overall sequence, allow a reasonable amount of time for device to answer
					return a, fmt.Errorf("Time out (%v times) on byte sequence after receiving %v bytes, expected %v", timeOutCount, len(a), i)
				}
			}
		}
	}

	waitfor := func(w byte, nextState VitoState, failState VitoState) (VitoState, []byte) {
		log.Debugf("State: %v, WaitingFor: %x, nextState: %v, failState: %v, failCount: %v", state, w, nextState, failState, failCount)

		b, err := waitforbytes(1)
		if err != nil {
			if state == unknown {
				log.Debug(err.Error())
			} else {
				log.Error(err.Error())
			}
			failCount++

			return failState, b
		}
		if w == b[len(b)-1] {
			failCount = 0
			return nextState, b
		}
		log.Warnf("Received unexpected byte sequence %x (expected %x)", b, w)
		return failState, b
	}
	go func() {
		b := make([]byte, 512)

		for {
			n, err := device.Read(b[0:])
			if err != nil {
				e <- err
				log.Errorf(err.Error())
				close(c)
				return
			}
			if n > 0 {
				for i := 0; i < n; i++ {
					// log.Debugf("Reading %v into chan %v", n, time.Now())
					// TODO: prevent writing on closed channel
					c <- b[i]
				}
			}
		}
	}()

	for {
		if prevstate != state {
			log.Debugf("State changed: %v --> %v", prevstate, state)
		}
		prevstate = state

		select {
		case err := <-e:
			log.Error(err.Error())
			return err
		default:
			// cont
		}
		switch state {
		case unknown:
			// state, _ = waitfor(ENQ, reset, swP300)
			// lastEnq = time.Now()
			state = reset
		case reset:
			device.Write([]byte{EOT})
			state = resetAck
		case resetAck:
			b, err := waitforbytes(1)
			if b[0] == ENQ {
				lastEnq = time.Now()
				failCount = 0
				state = idle
			} else if b[0] == ACK {
				lastEnq = time.Now()
				failCount = 0
				state = idle // TODO: Check how to switch to P300 immediately
			} else {
				log.Warn(err.Error())
				failCount++
				state = reset
			}
		case resetP300:
			device.Write([]byte{EOT})
			state = resetP300Ack
		case resetP300Ack:
			state, _ = waitfor(ACK, idle, resetAck)
			lastEnq = time.Now()
		case idle:
			if canP300 {
				state, _ = waitfor(ENQ, swP300, reset)
				lastEnq = time.Now()
				break
			}

			if !hasCmd {
				select {
				case cmd = <-cmdChan:
					hasCmd = true
				default:
					hasCmd = false
				}
			}

			if hasCmd {
				if time.Now().Sub(lastEnq) > (1500 * time.Millisecond) {
					state, _ = waitfor(ENQ, sendKwStart, reset)
				} else {
					state = sendKwStart
				}
			} else {
				state, _ = waitfor(ENQ, idle, reset)
			}
			lastEnq = time.Now()
		case sendKwStart:
			if prevstate != recvKw {
				device.Write([]byte{0x01})
			}
			state = sendKw
		case sendKw:
			hasCmd = false
			b, err := prepareCmd(&cmd, state)

			if err == nil {
				device.Write(b)
				state = recvKw
				break
			}
			resChan <- FsmResult{cmd.ID, err, nil}
			log.Error(err.Error())
			state = idle
		case recvKw:
			b, err := waitforbytes(int(cmd.ResultLen))
			if err != nil {
				log.Error(err)
				resChan <- FsmResult{cmd.ID, err, nil}
				state = idle
				break
			}
			select {
			case cmd = <-cmdChan:
				hasCmd = true
				state = sendKwStart
			default:
				hasCmd = false
				state = idle
			}
			resChan <- FsmResult{cmd.ID, err, b}
			lastEnq = time.Now()
		case swP300:
			// Emit sync packet / switch to P300
			device.Write([]byte{SYN, NUL, NUL})
			state = waitAck
		case waitAck:
			if failCount < 3 {
				state, _ = waitfor(ACK, wait, swP300)
			} else {
				state, _ = waitfor(ACK, wait, reset)
				canP300 = false
			}
			lastSyn = time.Now()
		case wait:
			if time.Now().Sub(lastSyn) > (10 * time.Second) { // TODO: check if 10s timout is ok
				// Emit a sync packet
				state = swP300
				break
			}
			if !hasCmd {
				select {
				case cmd = <-cmdChan:
					hasCmd = true
				default:
					hasCmd = false
				}
			}

			if hasCmd {
				state = sendP300
			}
		case sendP300:
			b, err := prepareCmd(&cmd, state)
			if err == nil {
				device.Write(b)
				state = sendP300Ack
				break
			} else {
				log.Warn(err.Error())
				state = wait
			}
		case sendP300Ack:
			b, err := waitforbytes(1)
			if err != nil {
				log.Warn(err.Error())
			}
			if b[0] == ACK {
				state = recvP300
			} else if b[0] == NAK {
				err = fmt.Errorf("Received NAK, going back to wait state")
				log.Debug(err.Error())
				resChan <- FsmResult{cmd.ID, err, nil}
				hasCmd = false
				state = wait
			} else {
				err = fmt.Errorf("Did not receive ACK/NACK, going back to wait state")
				log.Debug(err.Error())

				resChan <- FsmResult{cmd.ID, err, nil}
				//log.Warn(err.Error())
				hasCmd = false
				state = wait
			}
		case recvP300:
			state = recvP300Nak
			hasCmd = false

			// Get frame start (0x41) and frame length
			telegramPart1, err := waitforbytes(2)
			if err != nil {
				err = fmt.Errorf("Could not get start byte and length of telegram")
				resChan <- FsmResult{cmd.ID, err, nil}
				// Severe error --> reset
				state = reset
				break
			}
			if telegramPart1[0] != 0x41 {
				err = fmt.Errorf("Error in telegram start byte (expected 0x41, received %x)", telegramPart1[0])
				resChan <- FsmResult{cmd.ID, err, nil}
				// Severe error --> reset
				state = reset
				break
			}
			l := int(telegramPart1[1])
			telegramPart2, err := waitforbytes(l + 1)
			if err != nil {
				err = fmt.Errorf("Could not get telegram")
				resChan <- FsmResult{cmd.ID, err, nil}
				// Severe error --> reset
				state = reset
				break
			}

			if telegramPart2[0] != 0x01 {
				err = fmt.Errorf("Wrong telegram type (expected answer type 0x01, received %x)", telegramPart2[0])
				resChan <- FsmResult{cmd.ID, err, nil}
				break
			}
			if telegramPart2[1] != byte(cmd.Command) {
				err = fmt.Errorf("Wrong command byte (expected %x, received %x)", telegramPart2[1], byte(cmd.Command))
				resChan <- FsmResult{cmd.ID, err, nil}
				break
			}
			telegram := append(telegramPart1[1:], telegramPart2...)
			crc := Crc8(telegram[:len(telegram)-1])
			if telegram[len(telegram)-1] != crc {
				log.Errorf("telegram='%# x' calc-crc=%x", telegram, crc)
				err = fmt.Errorf("CRC verification failed (calculated %x, received %x)", crc, telegram[len(telegram)-1])
				resChan <- FsmResult{cmd.ID, err, nil}
				break
			}

			if telegramPart2[0] == 0x03 {
				err = fmt.Errorf("Received error telegram instead of an answer")
			}

			if telegramPart2[4] != cmd.ResultLen {
				err = fmt.Errorf("Expected result length %x != received length %x", cmd.ResultLen, telegramPart2[4])
			}

			if cmd.Command != p300WriteData {
				// Return data in Body
				resChan <- FsmResult{ID: cmd.ID, Err: err, Body: telegram[6 : len(telegram)-1]}
			} else {
				// Return number of written bytes in Body
				resChan <- FsmResult{ID: cmd.ID, Err: err, Body: []byte{telegram[5]}}
			}
			state = recvP300Ack
		case recvP300Ack:
			device.Write([]byte{ACK})
			state = wait
		case recvP300Nak:
			// TODO: Drain receive buffer?
			device.Write([]byte{NAK})
			state = wait
		default:
			log.Error("should not reach")
			state = unknown
		}
	}
}
