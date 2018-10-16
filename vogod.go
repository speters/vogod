package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strconv"
	"syscall"
	"time"

	vogo "./vogo"
	"github.com/gobuffalo/packr"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

var getSysDeviceIdent vogo.FsmCmd = vogo.FsmCmd{ID: [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}, Command: 0x01, Address: [2]byte{0x00, 0xf8}, Args: nil, ResultLen: 8}

// const testDeviceIdent = [8]byte{0x20, 0x92, 0x01, 0x07, 0x00, 0x00, 0x01, 0x5a}

var dpFile = flag.String("d", "ecnDataPointType.xml", "filename of ecnDataPointType.xml like file")
var etFile = flag.String("e", "ecnEventType.xml", "filename of ecnEventType.xml like file")
var httpServe = flag.Bool("s", false, "start http server")
var verbose = flag.Bool("v", false, "verbose logging")

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to `file`")
var memprofile = flag.String("memprofile", "", "write memory profile to `file`")

var conn *vogo.Device

func GetEventTypes(w http.ResponseWriter, r *http.Request) {
	e := json.NewEncoder(w)
	e.SetIndent("", "    ")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	e.Encode(conn.DataPoint.EventTypes)
}
func GetEvent(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	et, ok := conn.DataPoint.EventTypes[params["id"]]
	if !ok {
		w.WriteHeader(404)
		w.Write([]byte(fmt.Sprintf("No such EventType %v", params["id"])))
		return
	}
	b, err := conn.VRead(params["id"])
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	e := json.NewEncoder(w)
	e.SetIndent("", "    ")
	et.Value = b
	e.Encode(et)
}

func SetEvent(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	et, ok := conn.DataPoint.EventTypes[params["id"]]
	if !ok {
		w.WriteHeader(404)
		w.Write([]byte(fmt.Sprintf("No such EventType %v", params["id"])))
		return
	}

	decoder := json.NewDecoder(r.Body)
	var val interface{}
	err := decoder.Decode(&val)

	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	et.Value = val
	err = conn.VWrite(et.ID, val)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("\"OK\""))
}

func main() {
	flag.Parse()

	if *verbose == true {
		log.SetLevel(log.DebugLevel)
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp: true,
		})
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	addressHost := "orangepipc"
	addressPort := 3002
	address := addressHost + ":" + strconv.Itoa(addressPort)

	done := make(chan os.Signal, 1)

	signal.Notify(done,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	go func() {
		<-done

		if *memprofile != "" {
			f, err := os.Create(*memprofile)
			if err != nil {
				log.Fatal("could not create memory profile: ", err)
			}
			runtime.GC() // get up-to-date statistics
			if err := pprof.WriteHeapProfile(f); err != nil {
				log.Fatal("could not write memory profile: ", err)
			}
			f.Close()
		}
		if *cpuprofile != "" {
			pprof.StopCPUProfile()
		}
		os.Exit(0)
	}()

	conn = vogo.NewDevice()
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
		log.Errorf("Error opening file: %s", err)
		return
	}

	err = vogo.FindDataPointType(xmlFile, sysDeviceID, dpt)
	xmlFile.Close()
	if err != nil {
		log.Errorf(err.Error())
		return
	}
	j := len(dpt.EventTypes)

	xmlFile, err = os.Open(*etFile)
	if err != nil {
		log.Errorf("Error opening file: %s", err)
		return
	}

	i := vogo.FindEventTypes(xmlFile, &dpt.EventTypes)
	xmlFile.Close()
	if i == 0 {
		log.Errorf("No EventType definitions found for this DataPoint %v\n", sysDeviceID[:6])
		return
	}

	if i != j {
		log.Infof("Attn: %v EventType definitions found, but %v announced in DataPoint %v definition", i, j, dpt.ID)
	} else {
		log.Infof("All %v EventTypes found for DataPoint %v\n", i, dpt.ID)
	}

	var h *http.Server
	var router *mux.Router
	if *httpServe {
		router = mux.NewRouter()
		box := packr.NewBox("./static")
		router.Handle("/", http.FileServer(box))
		// router.Handle("/user", http.FileServer(http.Dir("./static/")))
		router.HandleFunc("/eventtypes", GetEventTypes).Methods("GET")
		router.HandleFunc("/event/{id}", GetEvent).Methods("GET")
		router.HandleFunc("/event/{id}", SetEvent).Methods("POST")

		h = &http.Server{Addr: ":8000", Handler: router}
		go func() { log.Error(h.ListenAndServe()) }()

		for {
			<-conn.Done
			<-time.After(5 * time.Second)
			err := conn.Reconnect()
			if err != nil {
				log.Error(err)
			} else {
				log.Infof("Reconnected")
			}
		}
	}
	/*
		for i := 0; i < 100; i++ {
			result := conn.RawCmd(getSysDeviceIdent)
			if result.Err != nil {
				return
			}
		}

		if true {
			b, _ := conn.VRead("Uhrzeit~0x088E")
			fmt.Printf("\nTIME: %v\n", b)
			conn.VWrite("Uhrzeit~0x088E", time.Now())
			b, _ = conn.VRead("Uhrzeit~0x088E")
			fmt.Printf("\nTIME: %v\n", b)
		}

		b, err := conn.VRead("BetriebsstundenBrenner1~0x0886")
		if err != nil {
			log.Errorf(err.Error())
		}
		fmt.Printf("BetriebsstundenBrenner1~0x0886: %v\n", b)
	*/
	/*
		n, err := conn.VRead("BedienteilBA_GWGA1~0x2323")
		if err != nil {
			log.Errorf(err.Error())
		}
		fmt.Printf("BedienteilBA_GWGA1~0x2323: %v\n", n)

		conn.VWrite("BedienteilBA_GWGA1~0x2323", 2)
	*/
	/*
		f, err := conn.VRead("Gemischte_AT~0x5527")
		if err != nil {
			log.Errorf(err.Error())
		}
		fmt.Printf("Gemischte_AT~0x5527: %v\n", f)

		f, err = conn.VRead("Solarkollektortemperatur~0x6564")
		if err != nil {
			log.Errorf(err.Error())
		}
		fmt.Printf("Solarkollektortemperatur~0x6564: %v\n", f)

		for i = 0; i < 0; i++ {
			c, err := conn.VRead("ecnsysEventType~Error")
			if err != nil {
				log.Errorf(err.Error())
			}
			fmt.Printf("ecnsysEventType~Error: %v\n", c)
		}

		// <-time.After(4 * time.Second)
		// log.Errorf("Nö!")
	*/
	/*
		id := [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f} // uuid.NewV4()

		cmdChan <- FsmCmd{ID: id, Command: 0x02, Address: [2]byte{0x23, 0x23}, Args: []byte{0x01}, ResultLen: 1}
		result = <-resChan
		fmt.Printf("%# x, %#v\n", result.Body, result.Err)
	*/
}
