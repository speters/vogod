package vogo

import (
	"fmt"
	"math/rand"
	"time"

	log "github.com/sirupsen/logrus"
)

func (o *Device) getCache(addr uint16, len uint16) (b []byte, oldestCacheTime time.Time) {
	aok := true
	var c []byte
	for a := addr; a < (addr + len); a++ {
		cacheMem, ok := (*o.Mem)[a]
		if !ok {
			aok = false
			break
		}
		if cacheMem.CacheTime.IsZero() {
			aok = false
			break
		}
		if oldestCacheTime.IsZero() || cacheMem.CacheTime.Before(oldestCacheTime) {
			oldestCacheTime = cacheMem.CacheTime
		}
		c = append(c, cacheMem.Data)

	}
	if !aok {
		c = nil
	}
	return c, oldestCacheTime
}

// RawCmd takes a raw FsmCmd and returns FsmResult
func (o *Device) RawCmd(cmd FsmCmd) FsmResult {
	ress := o.RawCmds(cmd)
	return ress[0]
}

// RawCmds takes a raw FsmCmd... and returns []FsmResult
// It makes use of caching. Set Device.CacheDuration to 0 to disable
// Operates in chunks of chunkSize if cmd.ResultLen exceeds chunkSize
// TODO: Check if chunking at raw cmd level is save or if EventTypes should be split at a higher level
func (o *Device) RawCmds(cmds ...FsmCmd) []FsmResult {
	const chunkSize = 32 // Max is 37?

	ress := []FsmResult{}
	o.cmdLock.Lock()
	defer o.cmdLock.Unlock()
	for n := 0; n < len(cmds); n++ {
		cmd := cmds[n]
		addr := bytes2Addr(cmd.Address)
		now := time.Now()

		if IsReadCmd(cmd.Command) && o.CacheDuration > 0 && cmd.ResultLen > 0 {
			c, oldestCacheTime := o.getCache(addr, uint16(cmd.ResultLen))
			if c != nil && now.Sub(oldestCacheTime) < o.CacheDuration {
				log.Debugf("Cache hit for FsmCmd at addr: %#x, Body: %# x", addr, c)
				ress = append(ress, FsmResult{ID: cmd.ID, Err: nil, Body: c})
				continue
			}
		}
		var err error
		i := 0

		var result FsmResult
		var body []byte
		for remainder := int(cmd.ResultLen); remainder > 0; remainder -= chunkSize {
			if remainder > chunkSize {
				cmd.ResultLen = chunkSize
			} else {
				cmd.ResultLen = byte(remainder)
			}
			o.cmdChan <- cmd
			result, _ = <-o.resChan
			if result.Err == nil {
				var t time.Time
				if IsReadCmd(cmd.Command) {
					t = now
				}

				for i := uint16(0); i < uint16(len(result.Body)); i++ {
					(*o.Mem)[addr+i] = &MemType{result.Body[i], t}
				}
			} else {
				// Save an error for multi-block cmds
				err = result.Err
			}
			body = append(body, result.Body...)

			addr += chunkSize
			i++
		}
		result.Err = err
		result.Body = body
		ress = append(ress, result)
	}
	return ress
}

func (e *EventTypeList) getEventTypeByID(ID string) (et *EventType, err error) {
	et, ok := (*e)[ID]
	if !ok {
		err = fmt.Errorf("EventType %v not found", ID)
	}
	return et, err
}

func newUUID() [16]byte {
	var uuid [16]byte
	for i := 0; i < 16; i++ {
		uuid[i] = byte(rand.Uint32())
	}
	return uuid
}

// VRead is the generic command to read Events of arbitrary data types
func (o *Device) VRead(ID string) (data interface{}, err error) {
	et, ok := o.DataPoint.EventTypes[ID]
	if !ok {
		return data, fmt.Errorf("EventType %v not found", ID)
	}

	if et.FCRead == 0 {
		return data, fmt.Errorf("EventType %v is not readable at address %v", et.ID, et.Address)
	}

	cmd := FsmCmd{ID: newUUID(), Command: et.FCRead, Address: addr2Bytes(et.Address), ResultLen: byte(et.BlockLength)}
	res := o.RawCmd(cmd)

	if res.Err != nil {
		return data, res.Err
	}

	data, err = et.Codec.Decode(et, &res.Body)
	return data, err
}

// VWrite is the generic command to write Events of arbitrary data types
func (o *Device) VWrite(ID string, data interface{}) (err error) {
	et, ok := o.DataPoint.EventTypes[ID]
	if !ok {
		return fmt.Errorf("EventType %v not found", ID)
	}

	if et.FCWrite == 0 {
		return fmt.Errorf("EventType %v is not writable at address %v", et.ID, et.Address)
	}

	if et.FCRead == 0 && (et.BytePosition != 0 || et.BitLength > 0) {
		return fmt.Errorf("EventType %v is not writable at address %v: can not read data prior to writing", et.ID, et.Address)
	}

	o.cmdWLock.Lock()
	defer o.cmdWLock.Unlock()

	cmd := FsmCmd{ID: newUUID(), Command: et.FCRead, Address: addr2Bytes(et.Address), ResultLen: byte(et.BlockLength)}
	res := o.RawCmd(cmd)

	if res.Err != nil {
		return res.Err
	}

	err = et.Codec.Encode(et, &res.Body, data)
	fmt.Printf("\nres.Body:\n%#v\n\n", res.Body)

	if err != nil {
		return err
	}

	cmd.Command = et.FCWrite
	cmd.Args = res.Body
	res = o.RawCmd(cmd)

	return err
}

func fromBCD(b byte) int {
	return ((int(b)>>4)*10 + (int(b) & 0x0f))
}
func toBCD(i int) byte {
	return byte(((i) / 10 * 16) + ((i) % 10))
}
