package vogo

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Codec is the interface providing Decode and Encode functions for conversion between raw and typed data
// Checking for validity should be done prior to assigning a codec to an EventType, as the Code/Decode functions
// only do very minimal checking for runtime optimization.
type Codec interface {
	Decode(et *EventType, b *[]byte) (v interface{}, err error)
	// Encode only encodes Bytes or Bits affected by the given EventType,
	// so []byte data must be interweaved with data from a previous read of the current block
	// prior to writing them to the device
	Encode(et *EventType, b *[]byte, v interface{}) (err error)
}

type nopCodec struct{}

func (nopCodec) Decode(et *EventType, b *[]byte) (v interface{}, err error) { return (*b), nil }
func (nopCodec) Encode(et *EventType, b *[]byte, v interface{}) (err error) { return nil }
func (codec nopCodec) MarshalJSON() ([]byte, error) {
	t := strings.Split(reflect.TypeOf(codec).String(), ".")
	return []byte(fmt.Sprintf("\"%s\"", t[len(t)-1])), nil
}

type valueListCodec struct{}

func (valueListCodec) Decode(et *EventType, b *[]byte) (v interface{}, err error) {
	if et.BitLength > 8 {
		return nil, fmt.Errorf("valueListCodec: can not handle BitLength > 8")
	}

	// While there are few EventTypes with ByteLength of 3, 4, 6, it seems sufficient to treat the ValueList as uint16
	var d uint16

	if et.BitLength > 0 {
		// BytePosition seems not always correct in the data from Vit*soft, so calculate
		bytepos := et.BitPosition / 8
		// bitpos in bytepos' byte
		bitpos := et.BitPosition % 8
		d = uint16(((*b)[bytepos] >> bitpos) & ((1 << et.BitLength) - 1))
	} else {
		if et.ByteLength == 1 {
			d = uint16((*b)[et.BytePosition])
		} else {
			d = (uint16((*b)[et.BytePosition+1]) << 8) | uint16((*b)[et.BytePosition])
		}
	}
	return d, nil
}
func (valueListCodec) Encode(et *EventType, b *[]byte, v interface{}) (err error) {
	if et.BitLength > 8 {
		return fmt.Errorf("valueListCodec: can not handle BitLength > 8")
	}

	// While there are few EventTypes with ByteLength of 3, 4, 6, it seems sufficient to treat the ValueList as uint16
	var d uint16

	switch v.(type) {
	case float32:
		d = uint16(v.(float32))
	case float64:
		d = uint16(v.(float64))
	case int:
		d = uint16(v.(int))
	case int8:
		d = uint16(v.(int8))
	case int16:
		d = uint16(v.(int16))
	case int32:
		d = uint16(v.(int32))
	case int64:
		d = uint16(v.(int64))
	case uint:
		d = uint16(v.(uint))
	case uint8:
		d = uint16(v.(uint8))
	case uint16:
		d = uint16(v.(uint16))
	case uint32:
		d = uint16(v.(uint32))
	case uint64:
		d = uint16(v.(uint64))
	default:
		return fmt.Errorf("Value must be a basic numeric type")
	}

	if et.BitLength > 0 {
		// BytePosition seems not always correct in the data from Vit*soft, so calculate
		bytepos := et.BitPosition / 8
		// bitpos in bytepos' byte
		bitpos := et.BitPosition % 8
		e := (*b)[bytepos] &^ (((1 << et.BitLength) - 1) << bitpos)
		f := byte(d) << bitpos
		(*b)[bytepos] = e | f
	} else {
		if et.ByteLength == 1 {
			(*b)[et.BytePosition] = byte(d)
		} else {
			(*b)[et.BytePosition+1] = byte(d >> 8)
			(*b)[et.BytePosition] = byte(d & 0xff)
		}
	}

	return nil
}

func (codec valueListCodec) MarshalJSON() ([]byte, error) {
	t := strings.Split(reflect.TypeOf(codec).String(), ".")
	return []byte(fmt.Sprintf("\"%s\"", t[len(t)-1])), nil
}

type divMulOffsetCodec struct{}

func (divMulOffsetCodec) Decode(et *EventType, b *[]byte) (v interface{}, err error) {
	if len((*b)) < (int(et.BytePosition) + int(et.ByteLength)) {
		return nil, fmt.Errorf("dateDivMulOffsetCodec: Data length mismatch")
	}

	if (et.BitLength > 0) && !((et.ByteLength == 1) && (et.BitLength == 4)) {
		return nil, fmt.Errorf("dateDivMulOffsetCodec: Can not handle arbitrary BitLength")
	}
	var f float32

	c := (*b)[et.BytePosition:(int(et.BytePosition) + int(et.ByteLength))]
	switch et.ByteLength {
	case 1:
		if et.Parameter == "SByte" || et.Parameter == "SInt" {
			f = float32(int8(c[0]))
		} else {
			d := uint8(c[0])
			if et.BitLength == 4 {
				// Handle nibbles
				if et.BitPosition == 0 {
					d = d >> 4
				} else if et.BitPosition == 4 {
					d = d & 0xf
				}
			}
			f = float32(d)
		}
	case 2:
		h := 1
		l := 0
		if et.Parameter == "SIntHighByteFirst" || et.Parameter == "IntHighByteFirst" {
			l = 1
			h = 0
		}
		v = (uint16(c[h]) << 8) | uint16(c[l])
		f = float32(v.(uint16))

		if et.Parameter == "SInt" || et.Parameter == "SIntHighByteFirst" {
			v = int16(v.(uint16))
			f = float32(v.(int16))
		}
	case 3:
		v = (uint32(c[2]) << 16) | (uint32(c[1]) << 8) | uint32(c[0])
		f = float32(v.(uint32))
	case 4:
		v = (uint32(c[3]) << 24) | (uint32(c[2]) << 16) | (uint32(c[1]) << 8) | uint32(c[0])
		f = float32(v.(uint32))
		if et.Parameter == "SInt" || et.Parameter == "SInt4" {
			v = int32(v.(uint32))
			f = float32(v.(int32))
		}
	default:
		return nil, fmt.Errorf("dateDivMulOffsetCodec: can not convert ByteLength %v", et.ByteLength)
	}

	return ((f * et.ConversionFactor) + et.ConversionOffset), nil
}

func (divMulOffsetCodec) Encode(et *EventType, b *[]byte, v interface{}) (err error) {
	if len((*b)) < (int(et.BytePosition) + int(et.ByteLength)) {
		return fmt.Errorf("dateDivMulOffsetCodec: Data length mismatch")
	}

	if (et.BitLength > 0) && !((et.ByteLength == 1) && (et.BitLength == 4)) {
		return fmt.Errorf("dateDivMulOffsetCodec: Can not handle arbitrary BitLength")
	}

	var f float32

	switch v.(type) {
	case float32:
		f = v.(float32)
	case float64:
		f = float32(v.(float64))
	case int:
		f = float32(v.(int))
	case int8:
		f = float32(v.(int8))
	case int16:
		f = float32(v.(int16))
	case int32:
		f = float32(v.(int32))
	case int64:
		f = float32(v.(int64))
	case uint:
		f = float32(v.(uint))
	case uint8:
		f = float32(v.(uint8))
	case uint16:
		f = float32(v.(uint16))
	case uint32:
		f = float32(v.(uint32))
	case uint64:
		f = float32(v.(uint64))
	default:
		return fmt.Errorf("Value must be a basic numeric type")
	}

	if et.LowerBorder != et.UpperBorder {
		if f < et.LowerBorder {
			f = et.LowerBorder
		}
		if f > et.UpperBorder {
			f = et.UpperBorder
		}
	}

	f = (f - et.ConversionOffset) / et.ConversionFactor

	switch et.ByteLength {
	case 1:
		if et.Parameter == "SByte" || et.Parameter == "SInt" {
			(*b)[et.BytePosition] = byte(int8(f))
		} else {
			d := uint8(f)
			if et.BitLength == 4 {
				// Handle nibbles
				if et.BitPosition == 0 {
					(*b)[et.BytePosition] = ((*b)[et.BytePosition] & 0xf) | (d << 4)
				} else if et.BitPosition == 4 {
					(*b)[et.BytePosition] = ((*b)[et.BytePosition] & 0xf0) | d
				}
			} else {
				(*b)[et.BytePosition] = byte(d)
			}
		}
	case 2:
		var d interface{}
		d = uint16(f)

		if et.Parameter == "SInt" || et.Parameter == "SIntHighByteFirst" {
			d = int16(f)
		}
		if et.Parameter == "SIntHighByteFirst" || et.Parameter == "IntHighByteFirst" {
			(*b)[et.BytePosition+1] = byte(d.(int16) & 0xff)
			(*b)[et.BytePosition] = byte(d.(int16) >> 8)
		} else {
			(*b)[et.BytePosition] = byte(d.(uint16) & 0xff)
			(*b)[et.BytePosition+1] = byte(d.(uint16) >> 8)
		}

	case 3:
		d := uint32(f)
		(*b)[et.BytePosition] = byte(d & 0xff)
		(*b)[et.BytePosition+1] = byte((d >> 8) & 0xff)
		(*b)[et.BytePosition+2] = byte((d >> 16) & 0xff)
	case 4:
		if et.Parameter == "SInt" || et.Parameter == "SInt4" {
			d := int32(f)
			(*b)[et.BytePosition] = byte(d & 0xff)
			(*b)[et.BytePosition+1] = byte((d >> 8) & 0xff)
			(*b)[et.BytePosition+2] = byte((d >> 16) & 0xff)
			(*b)[et.BytePosition+2] = byte((d >> 24) & 0xff)
		} else {
			d := uint32(f)
			(*b)[et.BytePosition] = byte(d & 0xff)
			(*b)[et.BytePosition+1] = byte((d >> 8) & 0xff)
			(*b)[et.BytePosition+2] = byte((d >> 16) & 0xff)
			(*b)[et.BytePosition+2] = byte((d >> 24) & 0xff)
		}
	default:
		return fmt.Errorf("dateDivMulOffsetCodec: can not convert ByteLength %v", et.ByteLength)
	}

	//return ((f * et.ConversionFactor) + et.ConversionOffset), nil
	return nil
}

func (codec divMulOffsetCodec) MarshalJSON() ([]byte, error) {
	t := strings.Split(reflect.TypeOf(codec).String(), ".")
	return []byte(fmt.Sprintf("\"%s\"", t[len(t)-1])), nil
}

type dateTimeBCDCodec struct{}

func decodeBCDDate(c []byte) (t time.Time, err error) {
	if len(c) < 4 {
		return t, fmt.Errorf("decodeBCDDate needs at least 4 bytes for a date, 8 for date and time)")
	}
	for i := len(c); i <= 8; i++ {
		// Fill up with zeroes in case only a date is to be decoded
		c = append(c, byte(0))
	}
	return time.Date(fromBCD(c[0])*100+fromBCD(c[1]), time.Month(fromBCD(c[2])), fromBCD(c[3]), fromBCD(c[5]), fromBCD(c[6]), fromBCD(c[7]), 0, time.Local), nil
}

func (dateTimeBCDCodec) Decode(et *EventType, b *[]byte) (v interface{}, err error) {
	if len(*b) < int(et.BlockLength) {
		return nil, fmt.Errorf("Could not decode: data length does not match BlockLength")
	}

	c := (*b)[et.BytePosition : et.BytePosition+et.ByteLength]
	return decodeBCDDate(c)
}

func (dateTimeBCDCodec) Encode(et *EventType, b *[]byte, v interface{}) (err error) {
	var t time.Time

	if len(*b) < int(et.BytePosition)+8 {
		return fmt.Errorf("Could not encode: data length does not fit")
	}

	switch v.(type) {
	case time.Time:
		t = v.(time.Time)
	default:
		t, err = time.Parse(time.RFC3339, v.(string))
		if err != nil {
			return fmt.Errorf("Time parse error: need time.Time type or a parseable string")
		}
	}

	if t.IsZero() {
		t = time.Now()
	}
	t = t.Local()

	(*b)[et.BytePosition] = byte(toBCD(t.Year() / 100))
	(*b)[et.BytePosition+1] = byte(toBCD(t.Year() % 100))
	(*b)[et.BytePosition+2] = byte(toBCD(int(t.Month())))
	(*b)[et.BytePosition+3] = byte(toBCD(t.Day()))
	wday := int(t.Weekday())
	if wday == 0 {
		wday = 7
	}
	(*b)[et.BytePosition+4] = byte(toBCD(wday))
	(*b)[et.BytePosition+5] = byte(toBCD(t.Hour()))
	(*b)[et.BytePosition+6] = byte(toBCD(t.Minute()))
	(*b)[et.BytePosition+7] = byte(toBCD(t.Second()))

	return nil
}

func (codec dateTimeBCDCodec) MarshalJSON() ([]byte, error) {
	t := strings.Split(reflect.TypeOf(codec).String(), ".")
	return []byte(fmt.Sprintf("\"%s\"", t[len(t)-1])), nil
}

type dateBCDCodec struct{}

func (dateBCDCodec) Decode(et *EventType, b *[]byte) (v interface{}, err error) {
	if len(*b) < int(et.BlockLength) {
		return nil, fmt.Errorf("Could not decode: data length does not match BlockLength")
	}

	c := (*b)[et.BytePosition : et.BytePosition+et.ByteLength]
	return decodeBCDDate(c)
}

func (dateBCDCodec) Encode(et *EventType, b *[]byte, v interface{}) (err error) {
	var t time.Time

	if len(*b) < int(et.BytePosition)+4 {
		return fmt.Errorf("Could not encode: data length does not fit")
	}

	switch v.(type) {
	case time.Time:
		t = v.(time.Time)
	default:
		t, err = time.Parse(time.RFC3339, v.(string))
		if err != nil {
			return fmt.Errorf("Time parse error: need time.Time type or a parseable string")
		}
	}

	if t.IsZero() {
		t = time.Now()
	}
	t = t.Local()

	(*b)[et.BytePosition] = byte(toBCD(t.Year() / 100))
	(*b)[et.BytePosition+1] = byte(toBCD(t.Year() % 100))
	(*b)[et.BytePosition+2] = byte(toBCD(int(t.Month())))
	(*b)[et.BytePosition+3] = byte(toBCD(t.Day()))
	if len(*b) > (int(et.BytePosition) + 4) {
		wday := int(t.Weekday())
		if wday == 0 {
			wday = 7
		}
		(*b)[et.BytePosition+4] = byte(toBCD(wday))
		if len(*b) > (int(et.BytePosition) + 5) {
			(*b)[et.BytePosition+5] = byte(0)
			if len(*b) > (int(et.BytePosition) + 6) {
				(*b)[et.BytePosition+6] = byte(0)
				if len(*b) > (int(et.BytePosition) + 7) {
					(*b)[et.BytePosition+7] = byte(0)
				}
			}
		}
	}

	return nil
}

func (codec dateBCDCodec) MarshalJSON() ([]byte, error) {
	t := strings.Split(reflect.TypeOf(codec).String(), ".")
	return []byte(fmt.Sprintf("\"%s\"", t[len(t)-1])), nil
}

type sec2DurationCodec struct{}

func (sec2DurationCodec) Decode(et *EventType, b *[]byte) (v interface{}, err error) {
	var t time.Duration
	var secs uint

	if len((*b)) < (int(et.BytePosition) + int(et.ByteLength)) {
		return nil, fmt.Errorf("sec2DurationCodec: Data length mismatch")
	}

	secs = 0
	for i := int(et.ByteLength) - 1; i >= 0; i-- {
		secs = secs << 8
		secs += uint((*b)[int(et.BytePosition)+i])
	}

	t = time.Duration(secs) * time.Second

	return t.String(), nil
}
func (sec2DurationCodec) Encode(et *EventType, b *[]byte, v interface{}) (err error) {
	var t time.Duration
	var secs uint

	if len((*b)) < (int(et.BytePosition) + int(et.ByteLength)) {
		return fmt.Errorf("sec2DurationCodec: Data length mismatch")
	}

	switch v.(type) {
	case time.Duration:
		t = v.(time.Duration)
	case string:
		var err error
		t, err = time.ParseDuration(v.(string))
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Value must be a time.Duration type or a string that is parseable by time.ParseDuration(string)")
	}

	secs = uint(t.Seconds())
	for i := 0; i < int(et.ByteLength); i++ {
		(*b)[int(et.BytePosition)+i] = byte(secs & 0xff)
		secs = secs >> 8
	}

	return nil
}

func (codec sec2DurationCodec) MarshalJSON() ([]byte, error) {
	t := strings.Split(reflect.TypeOf(codec).String(), ".")
	return []byte(fmt.Sprintf("\"%s\"", t[len(t)-1])), nil
}

/*
// Codec mappingTime53
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
type onoff53 struct {
	on  time.Duration
	off time.Duration
}

type onoff53d struct {
	onHour uint8
	onMin  uint8
	off    time.Duration
}

func (t onoff53) String() string {
	return fmt.Sprintf("%s .. %s", fmtDuration53(t.on), fmtDuration53(t.off))
}

func (t onoff53) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("[\"%s\", \"%s\"]", fmtDuration53(t.on), fmtDuration53(t.off))), nil
}

func fmtDuration53(d time.Duration) string {
	d = d.Round(10 * time.Minute)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	return fmt.Sprintf("%02dh%02d", h, m)
}

type mappingTime53 struct{}

func time532Duration(b byte) time.Duration {
	return (time.Duration(b>>3) * time.Hour) + (time.Duration(b&7) * time.Minute * 10)
}

func (mappingTime53) Decode(et *EventType, b *[]byte) (v interface{}, err error) {
	var t onoff53
	var d []onoff53
	var w [][]onoff53

	chunkSize := (et.BlockLength / et.BlockFactor)
	chunkNum := uint8(len(*b) / int(chunkSize))

	if (len(*b) % int(chunkSize)) != 0 {
		return v, fmt.Errorf("Codec mappingTime53 can not decode: data length is not a multiple of chunk size (%d)", chunkSize)
	}

	for i := uint8(0); i < chunkNum; i++ {
		for j := uint8(0); j < (chunkSize - 1); j += 2 {
			if (*b)[i*chunkSize+j] == 255 || (*b)[i*chunkSize+j+1] == 255 {
				break
			}
			t = onoff53{on: time532Duration((*b)[i*chunkSize+j]), off: time532Duration((*b)[i*chunkSize+j+1])}
			d = append(d, t)
		}
		w = append(w, d)
		d = []onoff53{}
	}

	return w, nil
}
func (mappingTime53) Encode(et *EventType, b *[]byte, v interface{}) (err error) { return nil }
func (codec mappingTime53) MarshalJSON() ([]byte, error) {
	t := strings.Split(reflect.TypeOf(codec).String(), ".")
	return []byte(fmt.Sprintf("\"%s\"", t[len(t)-1])), nil
}

/*
// Code mappingRaster152
Timer 24h, für jede 1/4 Stunde 2 Bit.
Werteliste:  0: Stand by
            1: Reduziert
            2: Normal
            3: Festwert

Beispiel:
    ByteLength 186 / BlockFactor 7 = 24
    2 Bit je 15min
    Bit 0,1 = 0min..<15min
    Bit 2,3 = 15min..<30min
    Bit 4,5 = 30min..<45min
    Bit 6,7 = 45min..<60min
*/
type mappingRaster152 struct{}

func (mappingRaster152) Decode(et *EventType, b *[]byte) (v interface{}, err error) { return (*b), nil }
func (mappingRaster152) Encode(et *EventType, b *[]byte, v interface{}) (err error) { return nil }
func (codec mappingRaster152) MarshalJSON() ([]byte, error) {
	t := strings.Split(reflect.TypeOf(codec).String(), ".")
	return []byte(fmt.Sprintf("\"%s\"", t[len(t)-1])), nil
}

/*
// Codec mappingErrors
   TODO: Format spec check
   Fehlerhistorie
   ByteLenght 90 / BlockFactor 10 =  9 Bytes / Eintrag
   Byte 0 Fehler?, Bytes1..8 DateTimeBCD

*/
type mappingErrors struct{}

type ErrEntry struct {
	errType byte
	errDate time.Time
}

func (e ErrEntry) String() string {
	return fmt.Sprintf("0x%0x: %v", e.errType, e.errDate)
}

func (e ErrEntry) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("{ \"errtype\": \"%d\", \"errdate\": \"%s\" }", e.errType, e.errDate)), nil
}

func (mappingErrors) Decode(et *EventType, b *[]byte) (v interface{}, err error) {
	e := []ErrEntry{}
	var errNum byte
	var errDate time.Time

	for j := 0; j < len((*b)); j += 9 {
		errNum = (*b)[j]
		c := append([]byte{}, (*b)[j+1:j+8]...)
		errDate, _ = decodeBCDDate(c)
		e = append(e, ErrEntry{errNum, errDate})
	}
	return e, nil
}
func (mappingErrors) Encode(et *EventType, b *[]byte, v interface{}) (err error) { return nil }

func (codec mappingErrors) MarshalJSON() ([]byte, error) {
	t := strings.Split(reflect.TypeOf(codec).String(), ".")
	return []byte(fmt.Sprintf("\"%s\"", t[len(t)-1])), nil
}

func fromBCD(b byte) int {
	return ((int(b)>>4)*10 + (int(b) & 0x0f))
}
func toBCD(i int) byte {
	return byte(((i) / 10 * 16) + ((i) % 10))
}

func bytes2Addr(a [2]byte) AddressT { return AddressT(uint16(a[0])<<8 + uint16(a[1])) }
func addr2Bytes(a AddressT) [2]byte { return [2]byte{byte(a >> 8), byte(a & 0xff)} }
