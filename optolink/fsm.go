package optolink

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
)

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

type FsmCmd struct {
	id        uuid.UUID
	command   CommandType
	address   [2]byte
	args      []byte
	resultLen byte
}

type FsmResult struct {
	id   uuid.UUID
	err  error
	body []byte
}

type FsmCmdDict struct {
	id   map[uuid.UUID]*FsmCmd
	lock sync.Mutex
}

func Crc8(b []byte) byte {
	crc := byte(0)
	for i := 0; i < len(b); i++ {
		crc += b[i]
	}
	return crc
}

func prepareCmd(cmd *FsmCmd, state VitoState) (b []byte, err error) {
	if state == sendP300 {
		if cmd.command == kwRead {
			cmd.command = p300ReadData
		} else if cmd.command == kwWrite {
			cmd.command = p300WriteData
		}

		switch cmd.command {
		case p300ReadData:
			b = []byte{0x41, byte(5), 0x00, 0x01, cmd.address[0], cmd.address[1], cmd.resultLen}
		case p300WriteData:
			b = []byte{0x41, byte((len(cmd.args) + 5)), 0x00, 0x02, cmd.address[0], cmd.address[1], byte(len(cmd.args))}
			b = append(b, cmd.args...)
			cmd.resultLen = byte(len(cmd.args))
		case p300FunctionCall:
			// b = []byte{0x41, byte((len(cmd.args) + 5)), 0x00, 0x07, cmd.address[0], cmd.address[1], cmd.resultLen}
			err = fmt.Errorf("Not implemented: p300FunctionCall")
			return nil, err
		default:
			err = fmt.Errorf("Not implemented: %v (GWG protocol?)", cmd.command)
			return nil, err
		}
		crc := Crc8(b)
		b = append(b, crc)

		return b, err
	}

	if state == sendKw {
		if cmd.command == p300ReadData {
			cmd.command = kwRead
		} else if cmd.command == p300WriteData {
			cmd.command = kwWrite
		}

		switch cmd.command {
		case kwRead:
			b = []byte{byte(cmd.command), cmd.address[0], cmd.address[1], cmd.resultLen}
		case kwWrite:
			b = []byte{byte(cmd.command), cmd.address[0], cmd.address[1], byte(len(cmd.args))}
			b = append(b, cmd.args...)
			cmd.resultLen = 1
		default:
			err = fmt.Errorf("Not implemented: %v (GWG protocol or P300 function call?)", cmd.command)
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
	c := make(chan []byte, 1)
	e := make(chan error)

	failCount := 0
	canP300 := true

	var cmd FsmCmd
	hasCmd := false

	defer close(c)
	defer close(e)

	waitforbytes := func(i int) ([]byte, error) {
		if i == 0 {
			return nil, nil
		}
		timeOutCount := 0
		var a, b []byte
		for {
			select {
			case b = <-c:
				a = append(a, b...)
				if len(a) == i {
					return a, nil
				}
				if len(a) > i {
					return a, fmt.Errorf("Received %v bytes, expected %v", len(a), i)
				}
			case <-time.After(20 * time.Millisecond):
				timeOutCount++
				if timeOutCount > 2 && len(a) > 0 {
					// Subsequent bytes should be received in short time
					return a, fmt.Errorf("Timed out after receiving %v bytes, expected %v", len(a), i)
				}
				if timeOutCount > ((2500 / 20) + len(a)) {
					// Timeout for overall sequence
					return a, fmt.Errorf("Timed out after receiving %v bytes, expected %v", len(a), i)
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

		/*
			//b := make([]byte, 256)
			var b []byte
			select {
			case b = <-c:
				r := b[len(b)-1]
				if r == w {
					failCount = 0
					return nextState, b
				}
				failCount++
				log.Warnf("received unexpected byte sequence %v", b)
				return failState, b
			case <-time.After(2500 * time.Millisecond):
				if failCount > 0 {
					log.Warn("timed out")
				}
				failCount++
				return failState, nil
			}
		*/
	}
	go func() {
		b := make([]byte, 512)

		for {
			n, err := device.Read(b[0:])
			if err != nil {
				e <- err
				log.Errorf(err.Error())
				return
			}
			if n > 0 {
				c <- b[:n]
			}
		}
	}()

	/*
		go func() {
			for {
				b, err := device.ReadByte()
				if err != nil {
					e <- err
					log.Errorf(err.Error())
				}
				c <- []byte{b}
			}
		}()
	*/

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
			state, _ = waitfor(ENQ, reset, swP300)
			lastEnq = time.Now()
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
				state = idle
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
				if prevstate == idle {
					state = sendKwStart
				} else {
					if time.Now().Sub(lastEnq) > (1500 * time.Millisecond) {
						state, _ = waitfor(ENQ, sendKwStart, reset) // Wait for ENQ
					} else {
						state = sendKw
					}
				}
			} else {
				state, _ = waitfor(ENQ, idle, reset)
				lastEnq = time.Now()
			}
		case sendKwStart:
			if prevstate == idle {
				device.Write([]byte{0x01})
			}
			state = sendKw
		case sendKw:
			//
			state = recvKw
		case recvKw:
			//
			state = idle
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
			if time.Now().Sub(lastSyn) > (10 * time.Second) { // TODO: actual value timeout
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
				state = recvP300Ack
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
				resChan <- FsmResult{cmd.id, err, nil}
				hasCmd = false
				state = wait
			} else {
				err = fmt.Errorf("Did not receive ACK/NACK, going back to wait state")
				resChan <- FsmResult{cmd.id, err, nil}
				//log.Warn(err.Error())
				hasCmd = false
				state = wait
			}
		case recvP300:
			state = recvP300Nak
			hasCmd = false

			// Get frame start (0x41) and frame length
			b, err := waitforbytes(2)
			if err != nil {
				err = fmt.Errorf("Could not get start byte and length of telegram")
				resChan <- FsmResult{cmd.id, err, nil}
				break
			}
			if b[0] != 0x41 {
				err = fmt.Errorf("Error in telegram start byte (expected 0x41, received %x)", b[0])
				resChan <- FsmResult{cmd.id, err, nil}
				break
			}
			l := int(b[1])
			b, err = waitforbytes(l)
			if err != nil {
				err = fmt.Errorf("Could not get telegram")
				resChan <- FsmResult{cmd.id, err, nil}
				break
			}

			if b[0] == 0x03 {
				err = fmt.Errorf("Received error telegram instead of an answer")
				state = recvP300Ack
				resChan <- FsmResult{cmd.id, err, nil}
				break
			}
			if b[0] != 0x01 {
				err = fmt.Errorf("Wrong telegram type (expected answer type 0x01, received %x)", b[0])
				resChan <- FsmResult{cmd.id, err, nil}
				break
			}
			if b[1] != byte(cmd.command) {
				err = fmt.Errorf("Wrong command byte (expected %x, received %x)", b[1], byte(cmd.command))
				resChan <- FsmResult{cmd.id, err, nil}
				break
			}
			crc := Crc8(b[0:len(b)-2]) + byte(l)
			if b[len(b)-1] != crc {
				err = fmt.Errorf("CRC verification failed (calculated %x, received %x)", b[len(b)-1], crc)
				resChan <- FsmResult{cmd.id, err, nil}
				break
			}

			resChan <- FsmResult{cmd.id, err, b[5 : len(b)-2]}
			state = recvP300Ack
		case recvP300Ack:
			device.Write([]byte{ACK})
			state = wait
		case recvP300Nak:
			// Drain receive buffer
			var b []byte
			device.Read(b)

			device.Write([]byte{NAK})
			state = wait
		default:
			log.Error("should not reach")
			state = unknown
		}
	}
}
