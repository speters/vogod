package main

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"./optolink"
	//"github.com/tarm/serial"
	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func main() {
	addressHost := "orangepipc"
	addressPort := 3002
	address := addressHost + ":" + strconv.Itoa(addressPort)

	if false {
		n, _ := connNet("socket://" + address)
		for {
			fmt.Printf("%v", <-n)
		}
	} else {
		/*
			conn, err := net.Dial("tcp", address)
			if err != nil {
				panic("Uhh")
			}
		*/
		conn := &optolink.Device{}
		conn.Connect("socket://" + address)
		/*
		           b := []byte{}
		   		for {
		   			n, _ := conn.Read(b)
		   			if n > 0 {
		   				fmt.Printf("%v ", b)
		   			}
		   		}
		*/
		optolink.VitoFsm(conn)

		/*
			        // test, works,
					var conn optolink.Device
					var err error
					err = conn.Connect("socket://" + address)
					if err != nil {
						log.Printf("(TODO:) ERR: %v", err)
					}
					b := make([]byte, 1) //TODO: why???
					for {
						n, _ := conn.Read(b)
						fmt.Printf("Read n=%v: %v ", n, b)
					}
		*/
	}
}

func connNet(link string) (<-chan []byte, chan<- []byte) {
	out := make(chan []byte)
	in := make(chan []byte)
	b := make([]byte, 1)

	go func() {
		const (
			stateUnconnected = iota
			stateConnected
			stateError
			stateEot
		)
		state := stateUnconnected
		var conn optolink.Device
		var err error
		start := time.Now()
		t := start
		i := 0
		n := 0
		for {
			switch state {
			case stateUnconnected:
				i = 0
				err = conn.Connect(link)
				if err != nil {
					log.Printf("(TODO:) ERR: %v", err)
					state = stateError
				} else {
					state = stateEot
				}
			case stateEot:
				conn.Write([]byte("\x04"))
				conn.Write([]byte("\x04"))
				state = stateConnected
			case stateConnected:
				start = t
				n, err = conn.Read(b)
				if err != nil {
					if err == io.EOF {
						log.Printf("ERR: %v (possibly connection timeout, reconnect)\n", err)
						state = stateUnconnected
					} else {
						log.Printf("ERR: %v\n", err)
						state = stateError
					}
				} else {
					out <- b
					if n != 1 {
						log.Printf("\nATTN: Read %v bytes\n", n)
					}
					i++
					if i == 3 {
						conn.Write([]byte("\x16\x00\x00"))
						log.Printf(" [% x] ", "\x16\x00\x00")
					}

					t = time.Now()
					log.Printf("(dt=%v, i=%v) 0x%x ", t.Sub(start), i, b)
				}
			case stateError:
				//close(out)
				break
			}
		}
	}()
	return out, in
}

/*
func fsm() {
	const (
		dead = iota
		unknwn
		gwg
		kw
		kw_init
		kw_idle
		kw_sync
		kw_send
		kw_recv
		p300

		eot
		ack
		nack
		sync
	)
}
*/
