package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	vogo "./vogo"
	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

var getSysDeviceIdent vogo.FsmCmd = vogo.FsmCmd{ID: [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}, Command: 0x01, Address: [2]byte{0x00, 0xf8}, Args: nil, ResultLen: 8}

// const testDeviceIdent = [8]byte{0x20, 0x92, 0x01, 0x07, 0x00, 0x00, 0x01, 0x5a}

var dpFile = flag.String("d", "ecnDataPointType.xml", "filename of ecnDataPointType.xml like file")
var etFile = flag.String("e", "ecnEventType.xml", "filename of ecnEventType.xml like file")

func main() {
	flag.Parse()
	addressHost := "orangepipc"
	addressPort := 3002
	address := addressHost + ":" + strconv.Itoa(addressPort)

	conn := &vogo.Device{}
	conn.Connect("socket://" + address)

	conn.DataPoint = &vogo.DataPointType{}
	dpt := conn.DataPoint
	dpt.EventTypes = make(vogo.EventTypeList)

	result := conn.RawCmd(getSysDeviceIdent)
	if result.Err != nil {
		return
	}

	var sysDeviceID [8]byte
	copy(sysDeviceID[:], result.Body[:8])

	xmlFile, err := os.Open(*dpFile)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}

	err = vogo.FindDataPointType(xmlFile, sysDeviceID, dpt)
	xmlFile.Close()
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	xmlFile, err = os.Open(*etFile)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}

	i := vogo.FindEventTypes(xmlFile, &dpt.EventTypes)
	xmlFile.Close()
	if i == 0 {
		fmt.Printf("No EventType definitions found for this DataPoint %v\n", sysDeviceID[:6])
		return
	}

	//fmt.Printf("num et: %v\n %#v\n", i, dpt.EventTypes)
	if i != len(dpt.EventTypes) {
		fmt.Printf("Attn: %v EventType definitions found, but %v announced in DataPoint %v definition", i, len(dpt.EventTypes), dpt.ID)
	} else {
		fmt.Printf("All %v EventTypes found for DataPoint %v\n", i, dpt.ID)
	}

	fmt.Printf("\nNum conn.DataPoint.EventTypes: %v\n", len(conn.DataPoint.EventTypes))

	for i := 0; i < 100; i++ {
		result := conn.RawCmd(getSysDeviceIdent)
		if result.Err != nil {
			return
		}
	}

	if false {
		b, _ := conn.VReadTime("Uhrzeit~0x088E")
		fmt.Printf("\nTIME: %v\n", b)
		conn.VWriteTime("Uhrzeit~0x088E", time.Now())
		b, _ = conn.VReadTime("Uhrzeit~0x088E")
		fmt.Printf("\nTIME: %v\n", b)
	}
	t, _ := conn.VReadTime("Uhrzeit~0x088E")
	fmt.Printf("\nTIME: %v\n", t)
	b, err := conn.VRead("BetriebsstundenBrenner1~0x0886")
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Printf("BetriebsstundenBrenner1~0x0886: %v\n", b)

	f, err := conn.VRead("Gemischte_AT~0x5527")
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Printf("Gemischte_AT~0x5527: %v\n", f)

	// <-time.After(4 * time.Second)
	// fmt.Println("Nö!")

	/*
		id := [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f} // uuid.NewV4()

		cmdChan <- FsmCmd{ID: id, Command: 0x02, Address: [2]byte{0x23, 0x23}, Args: []byte{0x01}, ResultLen: 1}
		result = <-resChan
		fmt.Printf("%# x, %#v\n", result.Body, result.Err)
	*/
}
