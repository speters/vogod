package main

import (
	"strconv"

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

	conn := &optolink.Device{}
	conn.Connect("socket://" + address)

	cmdChan := make(chan optolink.FsmCmd)
	resChan := make(chan optolink.FsmResult)
	optolink.VitoFsm(conn, cmdChan, resChan)
}
