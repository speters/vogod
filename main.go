package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"time"
	//"github.com/tarm/serial"
)

func main() {
	network := "tcp"
	addressHost := "orangepipc"
	addressPort := 3002
	address := addressHost + ":" + strconv.Itoa(addressPort)

	n := connNet(network, address)
	for {
		fmt.Printf("%v", <-n)
	}
}

func connNet(network string, address string) <-chan []byte {
	out := make(chan []byte)
	b := make([]byte, 1)

	go func() {
		const (
			stateUnconnected = iota
			stateConnected
			stateError
			stateEot
		)

		state := stateUnconnected
		var conn net.Conn
		var err error
		start := time.Now()
		t := start
		i := 0
		n := 0
		for {
			switch state {
			case stateUnconnected:
				conn, err = net.Dial(network, address)
				if err != nil {
					log.Printf("(TODO:) ERR: %v", err)
					state = stateError
				} else {
					state = stateEot
				}
			case stateEot:
				conn.Write([]byte("\x04"))
			case stateConnected:
				start = t
				n, err = conn.Read(b[0:])
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
				close(out)
				break
			}
		}
	}()
	return out
}

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

/*

 */
