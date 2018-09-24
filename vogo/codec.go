package vogo

import (
	"fmt"
	"time"
)

type Codec interface {
	Decode(et *EventType, b *[]byte) (v interface{}, err error)
	Encode(et *EventType, b *[]byte, v interface{}) (err error)
}
type dateTimeCodec struct{}

func (dateTimeCodec) Decode(et *EventType, b *[]byte) (v interface{}, err error) {
	var t time.Time

	c := (*b)[et.BytePosition : et.BytePosition+et.ByteLength]
	t = time.Date(fromBCD(c[0])*100+fromBCD(c[1]), time.Month(fromBCD(c[2])), fromBCD(c[3]), fromBCD(c[5]), fromBCD(c[6]), fromBCD(c[7]), 0, time.Local)
	return t, err
}

func (dateTimeCodec) Encode(et *EventType, b *[]byte, v interface{}) (err error) {
	var t time.Time

	if len(*b) < int(et.BlockLength) {
		return fmt.Errorf("Could not encode: data length does not match BlockLength")
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
	(*b)[et.BytePosition+1] = byte(toBCD(int(t.Month())))
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
