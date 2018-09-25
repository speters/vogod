package vogo

import (
	"fmt"
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

type dateDivMulOffsetCodec struct{}

func (dateDivMulOffsetCodec) Decode(et *EventType, b *[]byte) (v interface{}, err error) {

	return (*b), nil
}
func (dateDivMulOffsetCodec) Encode(et *EventType, b *[]byte, v interface{}) (err error) {
	return nil
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
		return fmt.Errorf("Value must be a time.Time type")
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
		return fmt.Errorf("Value must be a time.Time type")
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

type sec2DurationCodec struct{}

func (sec2DurationCodec) Decode(et *EventType, b *[]byte) (v interface{}, err error) {
	var t time.Duration
	var secs uint

	if len((*b)) < (int(et.BytePosition) + int(et.ByteLength)) {
		return nil, fmt.Errorf("sec2DurationCodec: Data length mismatch")
	}

	secs = 0
	for i := 0; i < int(et.ByteLength); i++ {
		secs += uint((*b)[int(et.BytePosition)+i])
		secs = secs << 8
	}

	t = time.Duration(secs) * time.Second

	return t, nil
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
	default:
		return fmt.Errorf("Value must be a time.Duration type")
	}

	secs = uint(t.Seconds())
	for i := int(et.ByteLength) - 1; i >= 0; i++ {
		(*b)[int(et.BytePosition)+i] = byte(secs & 0xff)
		secs = secs >> 8
	}

	return nil
}
