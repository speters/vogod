package main

import (
	"fmt"
	"strconv"
	"time"

	"./optolink"
	//"github.com/tarm/serial"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func main() {
	addressHost := "orangepipc"
	addressPort := 3002
	address := addressHost + ":" + strconv.Itoa(addressPort)

	conn := &optolink.Device{}
	conn.Connect("socket://" + address)

	cmdChan := make(chan optolink.FsmCmd)
	resChan := make(chan optolink.FsmResult)
	go optolink.VitoFsm(conn, cmdChan, resChan)

	<-time.After(4 * time.Second)
	id, _ := uuid.NewV4()
	cmdChan <- optolink.FsmCmd{ID: id, Command: 0x01, Address: [2]byte{0x00, 0xf8}, ResultLen: 4}
	result := <-resChan

	fmt.Printf("%# x, %#v\n", result.Body, result.Err)
	cmdChan <- optolink.FsmCmd{ID: id, Command: 0x02, Address: [2]byte{0x23, 0x23}, Args: []byte{0x01}, ResultLen: 1}
	result = <-resChan
	fmt.Printf("%# x, %#v\n", result.Body, result.Err)

}
