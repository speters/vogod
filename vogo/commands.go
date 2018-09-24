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
func (o *Device) RawCmd(cmd FsmCmd) FsmResult {
	addr := bytes2Addr(cmd.Address)
	now := time.Now()

	if IsReadCmd(cmd.Command) && o.CacheDuration > 0 && cmd.ResultLen > 0 {
		c, oldestCacheTime := o.getCache(addr, uint16(cmd.ResultLen))
		if c != nil && now.Sub(oldestCacheTime) < o.CacheDuration {
			log.Debugf("Cache hit for FsmCmd at addr: %v, Body: %# x", addr, c)
			return FsmResult{ID: cmd.ID, Err: nil, Body: c}
		}
	}
	o.cmdChan <- cmd
	result, _ := <-o.resChan
	if result.Err == nil {
		var t time.Time
		if IsReadCmd(cmd.Command) {
			t = now
		}

		for i := uint16(0); i < uint16(len(result.Body)); i++ {
			(*o.Mem)[addr+i] = &MemType{result.Body[i], t}
		}
	}
	return result
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

func (et *EventType) DecodeInt(b []byte) (data interface{}, typeHint string, err error) {
	typeHint = "Int"
	data = int(b[0])
	return data, typeHint, nil
}

// VRead is the generic command to read Events of arbitrary data types
func (o *Device) VRead(ID string) (data interface{}, err error) {
	et, ok := o.DataPoint.EventTypes[ID]
	if !ok {
		return data, fmt.Errorf("EventType %v not found", ID)
	}

	if et.FCRead == 0 {
		return data, fmt.Errorf("EventType %v is not readable", et.ID)
	}

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

func (o *Device) VReadByteArr(et *EventType) (b []byte, err error) {
	return b, err
}
func (o *Device) VWriteByteArr(et *EventType, b []byte) (err error) {
	return err
}
func (o *Device) VReadByte(et *EventType) (b byte, err error) {
	return b, err
}
func (o *Device) VWriteByte(et *EventType, b byte) (err error) {
	return err
}
func (o *Device) VReadInt32(et *EventType) (i int32, err error) {
	return i, err
}
func (o *Device) VWriteInt32(et *EventType, i int32) (err error) {
	return err
}
func (o *Device) VReadFloat32(et *EventType) (f float32, err error) {
	return f, err
}
func (o *Device) VWriteFloat32(et *EventType, f float32) (err error) {
	return err
}
func (o *Device) VReadTimeArr(et *EventType) (b []time.Time, err error) {
	return b, err
}
func (o *Device) VWriteTimeArr(et *EventType, b []time.Time) (err error) {
	return err
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
func (o *Device) VReadDuration(et *EventType) (t time.Duration, err error) {
	return t, err
}
func (o *Device) VWriteDuration(et *EventType, t time.Duration) (err error) {
	return err
}

/*
func DecodeDateMBus(b []byte) (r []byte)                        { return r }
func EncodeDateMBus(r []byte) (b []byte)                        { return b }
func DecodeDateTimeMBus(b []byte) (r []byte)                    { return r }
func EncodeDateTimeMBus(r []byte) (b []byte)                    { return b }
func DecodeDateTimeVitocom(b []byte) (r []byte)                 { return r }
func EncodeDateTimeVitocom(r []byte) (b []byte)                 { return b }
func DecodeDatenpunktADDR(b []byte) (r []byte)                  { return r }
func EncodeDatenpunktADDR(r []byte) (b []byte)                  { return b }
func DecodeEstrich(b []byte) (r []byte)                         { return r }
func EncodeEstrich(r []byte) (b []byte)                         { return b }
func DecodeHexByte2AsciiByte(b []byte) (r []byte)               { return r }
func EncodeHexByte2AsciiByte(r []byte) (b []byte)               { return b }
func DecodeHexByte2DecimalByte(b []byte) (r []byte)             { return r }
func EncodeHexByte2DecimalByte(r []byte) (b []byte)             { return b }
func DecodeHexToFloat(b []byte) (r []byte)                      { return r }
func EncodeHexToFloat(r []byte) (b []byte)                      { return b }
func DecodeKesselfolge(b []byte) (r []byte)                     { return r }
func EncodeKesselfolge(r []byte) (b []byte)                     { return b }
func DecodeNoConversion(b []byte) (r []byte)                    { return r }
func EncodeNoConversion(r []byte) (b []byte)                    { return b }
func DecodePhone2BCD(b []byte) (r []byte)                       { return r }
func EncodePhone2BCD(r []byte) (b []byte)                       { return b }
func DecodeRotateBytes(b []byte) (r []byte)                     { return r }
func EncodeRotateBytes(r []byte) (b []byte)                     { return b }
func DecodeVitocom300SGEinrichtenKanalLON(b []byte) (r []byte)  { return r }
func EncodeVitocom300SGEinrichtenKanalLON(r []byte) (b []byte)  { return b }
func DecodeVitocom300SGEinrichtenKanalMBUS(b []byte) (r []byte) { return r }
func EncodeVitocom300SGEinrichtenKanalMBUS(r []byte) (b []byte) { return b }
func DecodeVitocom300SGEinrichtenKanalWILO(b []byte) (r []byte) { return r }
func EncodeVitocom300SGEinrichtenKanalWILO(r []byte) (b []byte) { return b }
func DecodeVitocom3NV(b []byte) (r []byte)                      { return r }
func EncodeVitocom3NV(r []byte) (b []byte)                      { return b }
func DecodeVitocomEingang(b []byte) (r []byte)                  { return r }
func EncodeVitocomEingang(r []byte) (b []byte)                  { return b }
func DecodeVitocomNV(b []byte) (r []byte)                       { return r }
func EncodeVitocomNV(r []byte) (b []byte)                       { return b }

func DecodeDateBCD(b []byte) (t time.Time)                      { return t }
func EncodeDateBCD(t time.Time) (b []byte)                      { return b }
func DecodeDateTimeBCD(b []byte) (t time.Time)                  { return t }
func EncodeDateTimeBCD(t time.Time) (b []byte)                  { return b }

func DecodeDiv10(b []byte) (f float32)                          { return f }
func EncodeDiv10(f float32) (b []byte)                          { return b }
func DecodeDiv100(b []byte) (f float32)                         { return f }
func EncodeDiv100(f float32) (b []byte)                         { return b }
func DecodeDiv1000(b []byte) (f float32)                        { return f }
func EncodeDiv1000(f float32) (b []byte)                        { return b }
func DecodeDiv2(b []byte) (f float32)                           { return f }
func EncodeDiv2(f float32) (b []byte)                           { return b }
func DecodeMult10(b []byte) (f float32)                         { return f }
func EncodeMult10(f float32) (b []byte)                         { return b }
func DecodeMult100(b []byte) (f float32)                        { return f }
func EncodeMult100(f float32) (b []byte)                        { return b }
func DecodeMult2(b []byte) (f float32)                          { return f }
func EncodeMult2(f float32) (b []byte)                          { return b }
func DecodeMult5(b []byte) (f float32)                          { return f }
func EncodeMult5(f float32) (b []byte)                          { return b }
func DecodeMultOffset(b []byte) (f float32)                     { return f }
func EncodeMultOffset(f float32) (b []byte)                     { return b }
func DecodeMultOffsetBCD(b []byte) (f float32)                  { return f }
func EncodeMultOffsetBCD(f float32) (b []byte)                  { return b }
func DecodeMultOffsetFloat(b []byte) (f float32)                { return f }
func EncodeMultOffsetFloat(f float32) (b []byte)                { return b }
func DecodeSec2Hour(b []byte) (f float32)                       { return f }
func EncodeSec2Hour(f float32) (b []byte)                       { return b }
func DecodeSec2Minute(b []byte) (f float32)                     { return f }
func EncodeSec2Minute(f float32) (b []byte)                     { return b }

func DecodeHourDiffSec2Hour(b []byte) (i int)                   { return i }
func EncodeHourDiffSec2Hour(i int) (b []byte)                   { return b }
func DecodeUTCDiff2Month(b []byte) (i int)                      { return i }
func EncodeUTCDiff2Month(i int) (b []byte)                      { return b }
func DecodeIPAddress(b []byte) (s string)                       { return s }
func EncodeIPAddress(s string) (b []byte)                       { return b }
func DecodeTime53(b []byte) (s string)                          { return s }
func EncodeTime53(s string) (b []byte)                          { return b }
*/

func fromBCD(b byte) int {
	return ((int(b)>>4)*10 + (int(b) & 0x0f))
}
func toBCD(i int) byte {
	return byte(((i) / 10 * 16) + ((i) % 10))
}
