package optolink

import (
	"io"
	"time"

	log "github.com/sirupsen/logrus"
)

type VitoState byte

const (
	unknown  VitoState = iota // Maybe wait for ENQ/0x05
	reset                     // Send EOT/0x04
	resetAck                  // Wait for ENQ
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
)

type injectexpect struct {
}

type Bridge struct {
	Device *Device
	Peer   *io.ReadWriteCloser
	state  *VitoState
	/*
		inject []struct {
			state      vitoState
			vals       []byte
			injectFunc *func()
			ejectFunc  *func()
		}
	*/
}

/*

[]struct {
    uid
    command byte // read, write, func
}

type Injector struct
inject(state, data) (state, expect, err)
*/

func init() {
	log.SetLevel(log.DebugLevel)
}

func VitoFsm(device io.ReadWriter) error { //, peer *io.ReadWriter, inChan <-chan byte, outChan chan<- byte) {
	var state, prevstate VitoState
	state, prevstate = unknown, unknown
	lastSyn, lastEnq := time.Now(), time.Now()
	c := make(chan []byte, 1)
	e := make(chan error)
	failCount := 0
	canP300 := true

	pendingP300 := false
	pendingKW := false

	defer close(c)
	defer close(e)

	waitfor := func(w byte, nextState VitoState, failState VitoState) (VitoState, []byte) {
		log.Debugf("State: %v, WaitingFor: %v, nextState: %v, failState: %v", state, w, nextState, failState)
		b := make([]byte, 256)
		select {
		case b = <-c:
			r := b[len(b)-1]
			if r == w {
				failCount = 0
				return nextState, b
			} else {
				failCount++
				log.Warnf("received unexpected byte sequence %v", b)
				return failState, b
			}
		case <-time.After(3 * time.Second):
			failCount++
			log.Warn("timed out")
			return failState, nil
		}
	}
	go func() {
		b := make([]byte, 256) // TODO: check for actually needed size (1?)
		for {
			n, err := device.Read(b[0:]) // TODO: replace with device.ReadByte() ?
			if err != nil {
				e <- err
				log.Errorf(err.Error())
			}
			if n > 0 {
				c <- b[:n]
				// log.Debugf("Received %v bytes: %v", n, b)
			}
		}
	}()

	for {
		if prevstate != state {
			log.Debugf("entered state %v", state)
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
			state, _ = waitfor(ENQ, reset, reset)
			lastEnq = time.Now()
		case reset:
			device.Write([]byte{EOT})
			state = resetAck
		case resetAck:
			state, _ = waitfor(ENQ, idle, unknown)
			lastEnq = time.Now()
		case idle:
			// TODO: atomic pending...
			if pendingKW {
				if prevstate == idle {
					state = sendKwStart
				} else {
					if time.Now().Sub(lastEnq) > (2 * time.Second) {
						state, _ = waitfor(ENQ, sendKwStart, reset) // Wait for ENQ
					} else {
						state = sendKw
					}
				}
			} else if pendingP300 || canP300 {
				state = swP300
			} else {
				state, _ = waitfor(ENQ, idle, reset)
				lastEnq = time.Now()
			}
		case sendKwStart:
			if prevstate == idle {
				device.Write([]byte{0x01})
			}
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
			} else {
				if pendingKW {
					state = reset
				} else if pendingP300 {
					state = sendP300
				}
			}
		case sendP300:
			//
			state = recvP300Ack
		case sendP300Ack:
			state, _ = waitfor(ACK, recvP300, reset)
		case recvP300:
			//
			state = recvP300Ack
		case recvP300Ack:
			device.Write([]byte{EOT, NUL, NUL})
			state = wait
		default:
			log.Error("should not reach")
			state = unknown
		}
	}
}
