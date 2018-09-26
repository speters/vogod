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
// It makes use of caching. Set Device.CacheDuration to 0 to disable
// Operates in chunks of chunkSize if cmd.ResultLen exceeds chunkSize
// TODO: Check if chunking at raw cmd level is save or if EventTypes should be split at a higher level
func (o *Device) RawCmd(cmd FsmCmd) FsmResult {
	const chunkSize = 32 // Max is 37?

	o.cmdLock.Lock()
	defer o.cmdLock.Unlock()
	addr := bytes2Addr(cmd.Address)
	now := time.Now()

	if IsReadCmd(cmd.Command) && o.CacheDuration > 0 && cmd.ResultLen > 0 {
		c, oldestCacheTime := o.getCache(addr, uint16(cmd.ResultLen))
		if c != nil && now.Sub(oldestCacheTime) < o.CacheDuration {
			log.Debugf("Cache hit for FsmCmd at addr: %v, Body: %# x", addr, c)
			return FsmResult{ID: cmd.ID, Err: nil, Body: c}
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
	return result
}

// RawCmds takes a raw FsmCmd... and returns []FsmResult
// It works similar to RawCmd, but useful for combined read/writes, as it keeps the cmdLock during operations
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
				log.Debugf("Cache hit for FsmCmd at addr: %v, Body: %# x", addr, c)
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
	//ress := o.RawCmds(cmd)
	//res := ress[0]
	res := o.RawCmd(cmd)

	// TODO: Plugin for result manipulation for e.g. ecnsysEventType~Error
	if res.Err != nil {
		return data, res.Err
	}

	data, err = et.Codec.Decode(et, &res.Body)
	return data, err
}
func (o *Device) oerkRead(ID string) (data interface{}, err error) {
	et, _ := o.DataPoint.EventTypes[ID]
	if et.BlockFactor > 0 {
		switch et.MappingType {
		case 1:
			/*
							   Vier Schaltfenster mit je einem Ein- u. Ausschaltpunkt.
							   Speicherung der Zeiten im 5+3 Format (Stunde + 10-Minuten Raster)

							   Beispiel:
							       ByteLength 56 / BlockFactor 7 (jeder Tag) = 8 = 4*2 Schaltfenster
							       byte[0] : Schaltfenster 0 an
							       byte[1] : Schaltfenster 0 aus
							       byte[2] : Schaltfenster 1 an
							       byte[3] : Schaltfenster 1 aus
				                   ...

			*/
		case 2:
			/*
			   Timer 24h, für jede 1/4 Stunde 2 Bit.
			   Werteliste:  0: Stand by
			                1: Reduziert
			                2: Normal
			                3: Festwert

			    Beispiel:
			        ByteLength 186 / BlockFactor 7 = 24
			        2 Bit je 15min
			        Bit 0,1 = 0min..15min
			        Bit 2,3 = 15min..30min
			        Bit 4,5 = 30min..45min
			        Bit 6,7 = 45min..60min
			*/
		case 3:
			/*
			   TODO: Format spec check
			   Fehlerhistorie
			   ByteLenght 90 / BlockFactor 10 =  9 Bytes / Eintrag
			   Byte 0 Fehler?, Bytes1..8 DateTimeBCD???

			*/
		case 4:
			// Unknown RPC call related to error history?
			return data, fmt.Errorf("EventType %v with unknown MappingType 4", et.ID)
		default:
			//  No other MappingType known
			return data, fmt.Errorf("EventType %v with unknown MappingType", et.ID)

		}
		return data, fmt.Errorf("BlockFactor>0 EventTypes not supported")
	}

	return data, nil
}

// VReadTime reads an EventType as time.Time value
func (o *Device) VReadTime(ID string) (t time.Time, err error) {
	et, ok := o.DataPoint.EventTypes[ID]
	if !ok {
		return t, fmt.Errorf("EventType %v not found", ID)
	}

	if et.FCRead == 0 {
		return t, fmt.Errorf("EventType %v is not readable", et.ID)
	}

	if et.Conversion != "DateTimeBCD" {
		return t, fmt.Errorf("EventType %v can not be read into a time.Time value", et.ID)
	}

	cmd := FsmCmd{ID: newUUID(), Command: et.FCRead, Address: addr2Bytes(et.Address), ResultLen: byte(et.BlockLength)}
	res := o.RawCmd(cmd)

	if res.Err != nil {
		return t, res.Err
	}

	//t = time.Date(fromBCD(res.Body[0])*100+fromBCD(res.Body[1]), time.Month(fromBCD(res.Body[2])), fromBCD(res.Body[3]), fromBCD(res.Body[5]), fromBCD(res.Body[6]), fromBCD(res.Body[7]), 0, time.Local)
	v, _ := et.Codec.Decode(et, &res.Body)
	t = v.(time.Time)
	return t, err
}
func (o *Device) VWriteTime(ID string, t time.Time) (err error) {
	et, ok := o.DataPoint.EventTypes[ID]
	if !ok {
		return fmt.Errorf("EventType %v not found", ID)
	}

	if et.FCWrite == 0 {
		return fmt.Errorf("EventType %v is not readable", et.ID)
	}

	b := make([]byte, 8)

	err = et.Codec.Encode(et, &b, t)
	if err != nil {
		return err
	}

	cmd := FsmCmd{ID: newUUID(), Command: et.FCWrite, Address: addr2Bytes(et.Address), Args: b, ResultLen: byte(et.BlockLength)}
	res := o.RawCmd(cmd)

	return res.Err
}

func fromBCD(b byte) int {
	return ((int(b)>>4)*10 + (int(b) & 0x0f))
}
func toBCD(i int) byte {
	return byte(((i) / 10 * 16) + ((i) % 10))
}
