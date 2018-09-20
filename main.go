package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"./optolink"
	//"github.com/tarm/serial"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

var getSysDeviceIdent optolink.FsmCmd = optolink.FsmCmd{ID: [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}, Command: 0x01, Address: [2]byte{0x00, 0xf8}, Args: nil, ResultLen: 8}

// const testDeviceIdent = [8]byte{0x20, 0x92, 0x01, 0x07, 0x00, 0x00, 0x01, 0x5a}

var dpFile = flag.String("d", "ecnDataPointType.xml", "filename of ecnDataPointType.xml like file")
var etFile = flag.String("e", "ecnEventType.xml", "filename of ecnEventType.xml like file")

func main() {
	flag.Parse()
	addressHost := "orangepipc"
	addressPort := 3002
	address := addressHost + ":" + strconv.Itoa(addressPort)

	dpt := &optolink.DataPointType{}
	var etl optolink.EventTypeList
	etl = make(optolink.EventTypeList)
	dpt.EventTypes = etl

	conn := &optolink.Device{}
	conn.Connect("socket://" + address)

	cmdChan := make(chan optolink.FsmCmd)
	resChan := make(chan optolink.FsmResult)
	go optolink.VitoFsm(conn, cmdChan, resChan)

	cmdChan <- getSysDeviceIdent
	result, _ := <-resChan
	fmt.Printf("%# x, %#v\n", result.Body, result.Err)

	xmlFile, err := os.Open(*dpFile)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}

	var id [8]byte
	copy(id[:], result.Body[:8])
	err = optolink.FindDataPointType(xmlFile, id, dpt)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Printf("num ET: %#v\n", len(dpt.EventTypes))
	xmlFile.Close()

	xmlFile, err = os.Open(*etFile)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	i := optolink.FindEventTypes(xmlFile, &etl)
	fmt.Printf("num et: %v\n %#v\n", i, etl)
	xmlFile.Close()

	// <-time.After(4 * time.Second)
	// fmt.Println("Nö!")

	/*
		id := [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f} // uuid.NewV4()

		cmdChan <- optolink.FsmCmd{ID: id, Command: 0x02, Address: [2]byte{0x23, 0x23}, Args: []byte{0x01}, ResultLen: 1}
		result = <-resChan
		fmt.Printf("%# x, %#v\n", result.Body, result.Err)
	*/
}
