package vogo

import (
	"fmt"
	"io"
	"time"

	log "github.com/sirupsen/logrus"
)

//go:generate stringer -type VitoState,CommandType

// useSeqCnt == true includes a 3bit counter into bits 7,6,5 of the 4th telegram Byte as done by VitoConnect
const useSeqCnt bool = false

// Constants for OptoLink communications in GWG, KW, and P300 protocols
const (
	NUL byte = 0x00 // Used as part of P300SYN
	SOH byte = 0x01 // Start of heading - used for start of KW frame
	EOT byte = 0x04 // End of transmission - also used similar to a reset from P300 to KW
	ENQ byte = 0x05 // "ping" in KW mode
	ACK byte = 0x06 // Acknowledge in P300
	NAK byte = 0x15 // Negative acknowledge in P300
	SYN byte = 0x16 // Start of sync sequence SYN NUL NULL in P300, switches also from KW to P300
	SO3 byte = 0x41 // Start of frame in P300, ASCII "a"
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
	wait    // Occasionally send SYN NUL NULL
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
	nop              CommandType = 0x00
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

var readCmds = map[CommandType]bool{
	p300ReadData:            true,
	kwRead:                  true,
	virtualRead:             true,
	physicalRead:            true,
	eepromRead:              true,
	physicalXramRead:        true,
	physicalPortRead:        true,
	physicalBeRead:          true,
	physicalKmbusRAMRead:    true,
	physicalKmBusEepromRead: true,
}
var writeCmds = map[CommandType]bool{
	p300WriteData:     true,
	kwWrite:           true,
	virtualWrite:      true,
	physicalWrite:     true,
	eepromWrite:       true,
	physicalXramWrite: true,
	physicalPortWrite: true,
	physicalBeWrite:   true,
}

func isReadCmd(c CommandType) bool {
	if _, ok := readCmds[c]; ok {
		return true
	}
	if (c & 0x1f) == p300ReadData {
		return true
	}
	return false
}

func isWriteCmd(c CommandType) bool {
	if _, ok := writeCmds[c]; ok {
		return true
	}
	if (c & 0x1f) == p300WriteData {
		return true
	}
	return false
}

func init() {
	log.SetLevel(log.DebugLevel)
}

// FsmCmd holds a command for the VitoFsm state machine
type FsmCmd struct {
	ID        [16]byte // uuid.UID
	Command   CommandType
	Address   [2]byte
	Args      []byte
	ResultLen byte
}

// FsmResult is the result type for a previously issued FsmCmd command
type FsmResult struct {
	ID   [16]byte // uuid.UID
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

// cmdSeq is used as a 3bit counter for telegram sequences
var cmdSeq byte

// prepareCmd prepares a KW or P300 byte sequence from a FsmCmd command
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

		if useSeqCnt {
			// Introduce command sequence counter bits as VitoConnect does
			b[3] = b[3] | (cmdSeq << 5)
			cmdSeq++
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
func (device *Device) vitoFsm() error { //, peer *io.ReadWriter, inChan <-chan byte, outChan chan<- byte) {
	var state, prevstate VitoState
	state, prevstate = unknown, unknown
	lastSyn, lastEnq := time.Now(), time.Now()
	c := make(chan byte)
	e := make(chan error)

	defer func() {
		log.Warnf("Exiting vitoFSM")
		device.Done <- struct{}{}
	}()

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
		var ok bool
		for {
			select {
			case b, ok = <-c:
				if !ok {
					return nil, io.EOF
				}
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

	waitfor := func(w byte, nextState VitoState, failState VitoState) (VitoState, error) {
		log.Debugf("State: %v, WaitingFor: %x, nextState: %v, failState: %v, failCount: %v", state, w, nextState, failState, failCount)

		b, err := waitforbytes(1)
		if err != nil {
			if state == unknown {
				log.Debug(err.Error())
			} else {
				log.Error(err.Error())
			}
			failCount++

			return failState, err
		}
		if w == b[len(b)-1] {
			failCount = 0
			return nextState, err
		}
		log.Warnf("Received unexpected byte sequence %x (expected %x)", b, w)
		return failState, err
	}

	go func() {
		b := make([]byte, 512)

		for {
			select {
			case <-device.Done:
				log.Debugf("Closing, returning from reading loop goroutine")

				return
			default:
			}

			n, err := device.Read(b[0:])
			if err != nil {
				e <- err
				log.Errorf(err.Error())
				close(c) // TODO: should we?
				return
			}
			if n > 0 {
				for i := 0; i < n; i++ {
					c <- b[i]
				}
			}
		}
	}()

	for {
		select {
		case <-device.Done:
			log.Debugf("Closing, returning from fsm")
			return nil
		default:
		}

		if prevstate != state {
			log.Debugf("State changed: %v --> %v", prevstate, state)
		}
		prevstate = state

		select {
		case err, ok := <-e:
			if !ok {
				return fmt.Errorf("Closed chan")
			}
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
			fallthrough
		case reset:
			_, err := device.Write([]byte{EOT})
			if err != nil {
				return err
			}
			state = resetAck
		case resetAck:
			b, err := waitforbytes(1)
			if err != nil {
				return err
			}
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
			_, err := device.Write([]byte{EOT})
			if err != nil {
				return err
			}
			state = resetP300Ack
		case resetP300Ack:
			state, _ = waitfor(ACK, idle, resetAck)
			lastEnq = time.Now()
		case idle:
			var err error
			if canP300 {
				state, err = waitfor(ENQ, swP300, reset)
				if err == io.EOF {
					return err
				}
				lastEnq = time.Now()
				break
			}

			if !hasCmd {
				var ok bool
				select {
				case cmd, ok = <-device.cmdChan:
					if !ok {
						return fmt.Errorf("Read on closed channel")
					}
					hasCmd = true
				default:
					hasCmd = false
				}
			}

			if hasCmd {
				if time.Now().Sub(lastEnq) > (1500 * time.Millisecond) {
					state, err = waitfor(ENQ, sendKwStart, reset)
					if err == io.EOF {
						return err
					}
				} else {
					state = sendKwStart
				}
			} else {
				state, err = waitfor(ENQ, idle, reset)
				if err == io.EOF {
					return err
				}
				lastEnq = time.Now()
			}
		case sendKwStart:
			if prevstate != recvKw {
				_, err := device.Write([]byte{0x01})
				if err == io.EOF {
					return err
				}
			}
			state = sendKw
		case sendKw:
			hasCmd = false
			b, err := prepareCmd(&cmd, state)

			if err == nil {
				_, err = device.Write(b)
				if err == io.EOF {
					return err
				}

				state = recvKw
				break
			}
			device.resChan <- FsmResult{cmd.ID, err, nil}
			log.Error(err.Error())
			state = idle
		case recvKw:
			b, err := waitforbytes(int(cmd.ResultLen))
			if err != nil {
				if err == io.EOF {
					return err
				}

				log.Error(err)
				device.resChan <- FsmResult{cmd.ID, err, nil}
				state = idle
				break
			}
			if cmd.Command == kwWrite {
				// Should return 0x00 on successful write
				if b[0] != 0x00 {
					err = fmt.Errorf("kwWrite returned %v, expected 0x00", b)
					log.Error(err)
					device.resChan <- FsmResult{cmd.ID, err, nil}
					state = idle
					break
				}
				// Set returned Body value to contain length of written bytes, as in P300
				b = []byte{byte(len(cmd.Args))}
			}
			var ok bool
			select {
			case cmd, ok = <-device.cmdChan:
				hasCmd = true
				if !ok {
					return fmt.Errorf("Read on closed channel")
				}
				state = sendKwStart
			default:
				hasCmd = false
				state = idle
			}
			device.resChan <- FsmResult{cmd.ID, err, b}
			lastEnq = time.Now()
		case swP300:
			// Emit sync packet / switch to P300
			_, err := device.Write([]byte{SYN, NUL, NUL})
			if err == io.EOF {
				return err
			}
			state = waitAck
		case waitAck:
			var err error
			if failCount < 3 {
				state, err = waitfor(ACK, wait, swP300)
			} else {
				state, err = waitfor(ACK, wait, reset)
				canP300 = false
			}
			if err == io.EOF {
				return err
			}

			lastSyn = time.Now()
		case wait:
			if time.Now().Sub(lastSyn) > (10 * time.Second) { // TODO: check if 10s timout is ok
				// Emit a sync packet
				state = swP300
				break
			}
			if !hasCmd {
				var ok bool
				select {
				case cmd, ok = <-device.cmdChan:
					if !ok {
						return fmt.Errorf("Reading on closed channel")
					}
					hasCmd = true

				case _, ok := <-c:
					if !ok {
						device.Done <- struct{}{}
					}
					state = reset
				case <-time.After(10 * time.Second):
					// Emit a sync packet
					state = swP300
				}
			}

			if hasCmd {
				state = sendP300
			}
		case sendP300:
			b, err := prepareCmd(&cmd, state)
			if err == nil {
				_, err = device.Write(b)
				if err == io.EOF {
					return err
				}
				state = sendP300Ack
				break
			} else {
				log.Warn(err.Error())
				state = wait
			}
		case sendP300Ack:
			b, err := waitforbytes(1)
			if err != nil {
				if err == io.EOF {
					return err
				}

				log.Warn(err.Error())
			}
			if b[0] == ACK {
				state = recvP300
			} else if b[0] == NAK {
				err = fmt.Errorf("Received NAK, going back to wait state")
				log.Debug(err.Error())
				device.resChan <- FsmResult{cmd.ID, err, nil}
				hasCmd = false
				state = wait
			} else {
				err = fmt.Errorf("Did not receive ACK/NACK, going back to wait state")
				log.Debug(err.Error())

				device.resChan <- FsmResult{cmd.ID, err, nil}
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
				if err == io.EOF {
					return err
				}

				err = fmt.Errorf("Could not get start byte and length of telegram")
				device.resChan <- FsmResult{cmd.ID, err, nil}
				// Severe error --> reset
				state = reset
				break
			}
			if telegramPart1[0] != 0x41 {
				err = fmt.Errorf("Error in telegram start byte (expected 0x41, received %x)", telegramPart1[0])
				device.resChan <- FsmResult{cmd.ID, err, nil}
				// Severe error --> reset
				state = reset
				break
			}
			l := int(telegramPart1[1])
			telegramPart2, err := waitforbytes(l + 1)
			if err != nil {
				if err == io.EOF {
					return err
				}

				err = fmt.Errorf("Could not get telegram")
				device.resChan <- FsmResult{cmd.ID, err, nil}
				// Severe error --> reset
				state = reset
				break
			}

			if telegramPart2[0] != 0x01 {
				err = fmt.Errorf("Wrong telegram type (expected answer type 0x01, received %x)", telegramPart2[0])
				device.resChan <- FsmResult{cmd.ID, err, nil}
				break
			}
			// cmd.Command & 0x1F to strip the sequence counting bits in some protocol implementations
			if (telegramPart2[1] & 0x1F) != byte(cmd.Command) {
				err = fmt.Errorf("Wrong command byte (expected %x, received %x)", telegramPart2[1], byte(cmd.Command))
				device.resChan <- FsmResult{cmd.ID, err, nil}
				break
			}
			telegram := append(telegramPart1[1:], telegramPart2...)
			crc := Crc8(telegram[:len(telegram)-1])
			if telegram[len(telegram)-1] != crc {
				log.Errorf("telegram='%# x' calc-crc=%x", telegram, crc)
				err = fmt.Errorf("CRC verification failed (calculated %x, received %x)", crc, telegram[len(telegram)-1])
				device.resChan <- FsmResult{cmd.ID, err, nil}
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
				device.resChan <- FsmResult{ID: cmd.ID, Err: err, Body: telegram[6 : len(telegram)-1]}
			} else {
				// Return number of written bytes in Body
				device.resChan <- FsmResult{ID: cmd.ID, Err: err, Body: []byte{telegram[5]}}
			}
			state = recvP300Ack
		case recvP300Ack:
			_, err := device.Write([]byte{ACK})
			if err == io.EOF {
				return err
			}
			state = wait
		case recvP300Nak:
			// TODO: Drain receive buffer?
			_, err := device.Write([]byte{NAK})
			if err == io.EOF {
				return err
			}

			state = wait
		default:
			panic("Should not reach default state")
		}
	}
}
