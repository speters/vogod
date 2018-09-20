package main

import (
	"fmt"
	"strconv"
	"time"

	"./optolink"
	//"github.com/tarm/serial"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

var getSysDeviceIdent optolink.FsmCmd = optolink.FsmCmd{ID: [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}, Command: 0x01, Address: [2]byte{0x00, 0xf8}, Args: nil, ResultLen: 8}

func main() {
	addressHost := "orangepipc"
	addressPort := 3002
	address := addressHost + ":" + strconv.Itoa(addressPort)

	conn := &optolink.Device{}
	conn.Connect("socket://" + address)

	cmdChan := make(chan optolink.FsmCmd)
	resChan := make(chan optolink.FsmResult)
	go optolink.VitoFsm(conn, cmdChan, resChan)
	cmdChan <- getSysDeviceIdent
	result, _ := <-resChan
	fmt.Printf("%# x, %#v\n", result.Body, result.Err)
     <-time.After(4 * time.Second)

	fmt.Println("Nö!")

	/*
		id := [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f} // uuid.NewV4()

		cmdChan <- optolink.FsmCmd{ID: id, Command: 0x02, Address: [2]byte{0x23, 0x23}, Args: []byte{0x01}, ResultLen: 1}
		result = <-resChan
		fmt.Printf("%# x, %#v\n", result.Body, result.Err)
	*/
}
