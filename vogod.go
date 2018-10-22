package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"syscall"
	"time"

	"./vogo"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	// set back to upstream "github.com/tiborvass/uniline"
	// when pull request 7 (fix #6: panic: Split called after Scan) is merged:
	"github.com/speters/uniline"
)

var dpFile = flag.String("d", "ecnDataPointType.xml", "filename of ecnDataPointType.xml like `file`")
var etFile = flag.String("e", "ecnEventType.xml", "filename of ecnEventType.xml like `file`")
var httpServe = flag.String("s", "", "start http server at [bindtohost][:]port")
var connTo = flag.String("c", "", "connection string, use socket://[host]:[port] for TCP or [serialDevice] for direct serial connection ")
var verbose = flag.Bool("v", false, "verbose logging")

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to `file`")
var memprofile = flag.String("memprofile", "", "write memory profile to `file`")

var conn *vogo.Device

// To be set via go build -ldflags "-X main.buildVersion=$(date -u +%FT%TZ) -X main.buildDate=$(git describe --dirty)"
var buildVersion string = "unspecified"
var buildDate string = "unknown"

var getSysDeviceIdent vogo.FsmCmd = vogo.FsmCmd{ID: [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}, Command: 0x01, Address: [2]byte{0x00, 0xf8}, Args: nil, ResultLen: 8}

// const testDeviceIdent = [8]byte{0x20, 0x92, 0x01, 0x07, 0x00, 0x00, 0x01, 0x5a}

func getEventTypes(w http.ResponseWriter, r *http.Request) {
	e := json.NewEncoder(w)
	e.SetIndent("", "    ")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s.json\"", conn.DataPoint.ID))
	w.WriteHeader(http.StatusOK)
	e.Encode(conn.DataPoint.EventTypes)
}
func getDataPoint(w http.ResponseWriter, r *http.Request) {
	e := json.NewEncoder(w)
	e.SetIndent("", "    ")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	e.Encode(conn.DataPoint)
}
func versionInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	v := struct {
		Version    string `json:"version"`
		Build_date string `json:"build_date"`
	}{Version: buildVersion, Build_date: buildDate}
	j, _ := json.Marshal(v)
	w.Write([]byte(j))
}
func getEvent(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	et, ok := conn.DataPoint.EventTypes[params["id"]]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(fmt.Sprintf("No such EventType %v", params["id"])))
		return
	}
	b, err := conn.VRead(params["id"])
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		w.Write([]byte(err.Error()))
		w.Write([]byte(fmt.Sprintf("\n\n%#v", et)))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	e := json.NewEncoder(w)
	e.SetIndent("", "    ")

	rEt := *et
	rEt.Value = b
	e.Encode(rEt)
}

func setEvent(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	et, ok := conn.DataPoint.EventTypes[params["id"]]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(fmt.Sprintf("No such EventType %v", params["id"])))
		return
	}

	decoder := json.NewDecoder(r.Body)
	var val interface{}
	err := decoder.Decode(&val)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		w.Write([]byte(err.Error()))
		w.Write([]byte(fmt.Sprintf("\n\n%#v", et)))
		return
	}
	et.Value = val
	err = conn.VWrite(et.ID, val)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		w.Write([]byte(err.Error()))
		w.Write([]byte(fmt.Sprintf("\n\n%#v", et)))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("\"OK\"\n"))
}

func cliget(id string) (string, error) {
	et, ok := conn.DataPoint.EventTypes[id]
	if !ok {
		return "", fmt.Errorf("No such EventType %v", id)
	}
	b, err := conn.VRead(id)
	if err != nil {
		return "", err
	}

	rEt := *et
	rEt.Value = b

	bs, err := json.MarshalIndent(rEt, "", "    ")
	return string(bs), err
}

func main() {
	flag.Parse()

	if *verbose == true {
		log.SetLevel(log.DebugLevel)
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp: true,
		})
	}

	if *connTo == "" {
		log.Fatal("Need connection string in -c option")
		os.Exit(1)
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
	conn.Connect(*connTo)

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
	if *httpServe != "" {
		router = mux.NewRouter()

		router.HandleFunc("/eventtypes", getEventTypes).Methods("GET")
		router.HandleFunc("/datapoint", getDataPoint).Methods("GET")
		router.HandleFunc("/version", versionInfo).Methods("GET")
		router.HandleFunc("/event/{id}", getEvent).Methods("GET")
		router.HandleFunc("/event/{id}", setEvent).Methods("POST")

		//box := packr.NewBox("./web/")
		// router.Handle("/b", http.FileServer(box))
		fs := http.FileServer(http.Dir("./web"))
		router.PathPrefix("/").Handler(fs)
		//router.PathPrefix("/assets").Handler(http.StripPrefix("/assets/", fs))

		if i, err := strconv.Atoi(*httpServe); err == nil {
			*httpServe = fmt.Sprintf(":%d", i)
		}

		h = &http.Server{Addr: *httpServe, Handler: router}
		go func() { log.Error(h.ListenAndServe()) }()

		for {
			<-conn.Done
			<-time.After(12 * time.Second)
			err := conn.Reconnect()
			if err != nil {
				log.Error(err)
			} else {
				log.Infof("Reconnected")
			}
		}
	}

	prompt := "> "
	scanner := uniline.DefaultScanner()
	for scanner.Scan(prompt) {
		line := scanner.Text()
		words := []string{}
		if len(line) > 0 {
			scanner2 := bufio.NewScanner(strings.NewReader(line))
			scanner2.Split(bufio.ScanWords)
			for scanner2.Scan() {
				words = append(words, scanner2.Text())
			}
			if err := scanner2.Err(); err != nil {
				log.WithError(err)
				continue
			}

			if len(words) == 1 {
				if words[0] == "eventtypes" {
					s, err := json.MarshalIndent(conn.DataPoint.EventTypes, "", "    ")
					if err != nil {
						log.Error(err)
					}
					fmt.Print(string(s))
					fmt.Printf("\x1e\n")
				} else if words[0] == "datapointtype" {
					s, err := json.MarshalIndent(conn.DataPoint, "", "    ")
					if err != nil {
						log.Error(err)
					}
					fmt.Print(string(s))
					fmt.Printf("\x1e\n")
				} else {
					log.Errorf("No such command '%s'", words[0])
				}
			} else if len(words) == 2 {
				if words[0] == "get" {
					s, err := cliget(words[1])
					if err != nil {
						log.Error(err)
						continue
					}
					fmt.Print(s)
					fmt.Printf("\x1e\n")
				} else {
					log.Errorf("No such command '%s'", words[0])
				}
			} else if len(words) > 2 {
				if words[0] == "set" {
					fmt.Println("set to be implemented")
					fmt.Printf("\x1e\n")
				} else {
					log.Errorf("No such command '%s'", words[0])
				}
			} else {
				// should not reach
				continue
			}
			scanner.AddToHistory(line)
		}
	}
	if err := scanner.Err(); err != nil {
		panic(err)
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
