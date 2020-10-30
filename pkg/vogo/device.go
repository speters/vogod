package vogo

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tarm/serial"
)

// Device is the basic ReadWriteCloser representation of a physical Optolink device
type Device struct {
	conn         io.ReadWriteCloser
	r            *bufio.Reader
	rlock, wlock sync.Mutex

	link      string
	connected bool
	Done      chan struct{}

	DataPoint     *DataPointType
	Mem           *MemMap
	CacheDuration time.Duration

	cmdChan  chan FsmCmd
	resChan  chan FsmResult
	cmdLock  sync.Mutex
	cmdWLock sync.Mutex
}

const cacheDuration = 3 * time.Second

// Close closes Device, closing underlying connection via serial or network
func (o *Device) Close() error {
	var err error

	o.rlock.Lock()
	o.wlock.Lock()
	defer o.rlock.Unlock()
	defer o.wlock.Unlock()

	select {
	case <-o.Done:
		// return fmt.Errorf("Close failed: Closing")
		o.connected = false
		return io.ErrClosedPipe
	default:
	}
	o.r.Reset(o.conn) // TODO: check if useful
	err = o.conn.Close()
	close(o.Done)
	o.connected = false
	return err
}

func (o *Device) Read(b []byte) (int, error) {
	o.rlock.Lock()
	defer o.rlock.Unlock()

	if o.connected == false {
		return 0, io.EOF
	}

	select {
	case <-o.Done:
		o.connected = false
		return 0, io.EOF
	default:
		n, err := o.r.Read(b)
		log.Debugf("Read b='%# x', n=%v, err=%v", b[0:n], n, err)
		return n, err
	}
}

// ReadByte reads and returns a single byte. If no byte is available, returns an error.
func (o *Device) ReadByte() (byte, error) {
	o.rlock.Lock()
	defer o.rlock.Unlock()
	if o.connected == false {
		return 0, io.EOF
	}
	select {
	case <-o.Done:
		o.connected = false
		return 0, io.EOF
	default:
		return o.r.ReadByte()
	}
}

// Peek returns the next n bytes without advancing the reader.
func (o *Device) Peek(n int) ([]byte, error) {
	o.rlock.Lock()
	defer o.rlock.Unlock()
	if o.connected == false {
		return nil, io.EOF
	}
	select {
	case <-o.Done:
		o.connected = false
		return nil, io.EOF
	default:
		return o.r.Peek(n)
	}
}

func (o *Device) Write(b []byte) (int, error) {
	o.wlock.Lock()
	defer o.wlock.Unlock()
	if o.connected == false {
		return 0, io.EOF
	}
	select {
	case <-o.Done:
		o.connected = false
		return 0, io.EOF
	default:
		n, err := o.conn.Write(b)
		log.Debugf("Write b='%# x', n=%v, err=%v", b, n, err)
		return n, err
	}
}

// NewDevice is the factory method to create a new Device
func NewDevice() *Device {
	o := &Device{}

	o.cmdChan = make(chan FsmCmd)
	o.resChan = make(chan FsmResult)

	o.DataPoint = &DataPointType{}
	o.DataPoint.EventTypes = make(EventTypeList)
	m := make(MemMap, (1 << 16))
	o.Mem = &m

	o.CacheDuration = cacheDuration

	return o
}

// Connect attaches to the OptoLink device via serial device or a tcp socket
func (o *Device) Connect(link string) error {
	o.rlock.Lock()
	o.wlock.Lock()
	defer o.rlock.Unlock()
	defer o.wlock.Unlock()
	var err error

	u, err := url.Parse(link)
	if err != nil {
		o.connected = false
		return err
	}

	if (u.Scheme == "socket") || (u.Scheme == "tcp") {
		// Connect via network
		o.conn, err = net.Dial("tcp", u.Host)
		if err != nil {
			return err
		}
		o.conn.(*net.TCPConn).SetKeepAlive(true)
		o.conn.(*net.TCPConn).SetKeepAlivePeriod(30 * time.Second)
	} else if (u.Scheme == "file") || (u.Scheme == "") {
		// Connect via serial
		o.conn, err = serial.OpenPort(&serial.Config{Name: u.Path, Baud: 4800, Size: 8, Parity: serial.ParityEven, StopBits: serial.Stop2})
		if err != nil {
			return err
		}
	} else {
		o.connected = false
		return fmt.Errorf("Can not find a valid connection string in \"%v\"", link)
	}
	o.connected = true
	o.link = link

	o.Done = make(chan struct{})
	o.r = bufio.NewReader(o.conn)

	go o.vitoFsm()

	return nil
}

// Reconnect device
func (o *Device) Reconnect() error {
	o.Close()
	close(o.cmdChan)
	close(o.resChan)
	o.cmdChan = make(chan FsmCmd)
	o.resChan = make(chan FsmResult)
	return o.Connect(o.link)
}
