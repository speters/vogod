package vogo

import (
	"fmt"
	"io"
	"math/rand"
	"time"

	log "github.com/sirupsen/logrus"
)

func (o *Device) getCache(addr AddressT, len uint16) (b []byte, oldestCacheTime time.Time) {
	aok := true
	var c []byte
	for a := uint16(addr); a < (uint16(addr) + len); a++ {
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
// ATTN: Operates in chunks of chunkSize if cmd.ResultLen exceeds chunkSize
func (o *Device) RawCmds(cmds ...FsmCmd) (ress []FsmResult) {
	const chunkSize = 32 // Max is 37?

	o.cmdLock.Lock()
	defer o.cmdLock.Unlock()
	defer func() {
		// recover from panic caused by writing to a closed channel
		if r := recover(); r != nil {
			err := fmt.Errorf("%v", r)
			ress = append(ress, FsmResult{Err: err})
		}
	}()

	for n := 0; n < len(cmds); n++ {
		cmd := cmds[n]
		addr := bytes2Addr(cmd.Address)
		now := time.Now()

		if isReadCmd(cmd.Command) && o.CacheDuration > 0 && cmd.ResultLen > 0 {
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
		var ok bool
		var body []byte
		for remainder := int(cmd.ResultLen); remainder > 0; remainder -= chunkSize {
			if remainder > chunkSize {
				cmd.ResultLen = chunkSize
			} else {
				cmd.ResultLen = byte(remainder)
			}
			select {
			case o.cmdChan <- cmd:
				break
			case <-time.After(10 * time.Second):
				log.Errorf("Device not connected")
				return []FsmResult{FsmResult{Err: io.EOF}}
			}
			result, ok = <-o.resChan
			if !ok {
				return []FsmResult{FsmResult{Err: io.EOF}}
			}
			if result.Err == nil {
				var t time.Time
				if isReadCmd(cmd.Command) {
					t = now
				}

				for i := uint16(0); i < uint16(len(result.Body)); i++ {
					(*o.Mem)[uint16(addr)+i] = &MemType{result.Body[i], t}
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

	step := et.BlockLength
	if et.BlockFactor > 0 {
		step = et.BlockLength / et.BlockFactor
	}

	cmd := FsmCmd{ID: newUUID(), Command: et.FCRead, Address: addr2Bytes(et.Address), ResultLen: byte(step)}
	var res FsmResult
	b := []byte{}
	for i := uint8(0); i < et.BlockLength; i += step {

		cmd.Address = addr2Bytes(et.Address + AddressT(i))

		res = o.RawCmd(cmd)

		b = append(b, res.Body...)
		if res.Err != nil {
			return data, res.Err
		}
	}

	data, err = et.Codec.Decode(et, &b)
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

	//TODO: Chunked writes
	step := et.BlockLength
	if et.BlockFactor > 0 {
		step = et.BlockLength / et.BlockFactor
	}

	cmd := FsmCmd{ID: newUUID(), Command: et.FCRead, Address: addr2Bytes(et.Address), ResultLen: byte(step)}
	var res FsmResult
	b := []byte{}
	for i := uint8(0); i < et.BlockLength; i += step {

		cmd.Address = addr2Bytes(et.Address + AddressT(i))
		res = o.RawCmd(cmd)
		b = append(b, res.Body...)

		if res.Err != nil {
			return res.Err
		}
	}
	err = et.Codec.Encode(et, &b, data)
	if err != nil {
		return err
	}

	cmd.Command = et.FCWrite
	for i := uint8(0); i < et.BlockLength; i += step {
		cmd.Address = addr2Bytes(et.Address + AddressT(i))
		cmd.Args = b[i : i+step]
		res = o.RawCmd(cmd)
	}

	return err
}
