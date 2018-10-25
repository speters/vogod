package vogo

import (
	"fmt"
	"time"
)

// DataPointType is a type to describe a DataPoint (aka a Vito* device)
type DataPointType struct {
	ID             string          `json:"id"`
	Description    string          `json:"description,omitempty"`
	SysDeviceIdent SysDeviceIdentT `json:"sys_device_ident"`
	EventTypes     EventTypeList   `json:"-"`
}

// SysDeviceIdentT holds the full system id of a device (type, hardware revision, software revision, ...)
type SysDeviceIdentT [8]byte

func (sdi SysDeviceIdentT) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"% X\"", sdi)), nil
}

// EventType holds low-level info for commands like address, data format and conversion hints
type EventType struct {
	ID          string   `json:"id"`
	Address     AddressT `json:"address"`
	Description string   `json:"description,omitempty"`

	FCRead  CommandType `json:"fcread"`
	FCWrite CommandType `json:"fcwrite"`

	Parameter string `json:"-"` // `json:"parameter"`

	PrefixRead   []byte `json:"-"` // `json:"prefix_read,omitempty"`
	PrefixWrite  []byte `json:"-"` // `json:"prefix_write,omitempty"`
	BlockLength  uint8  `json:"block_length,omitempty"`
	BlockFactor  uint8  `json:"block_factor,omitempty"`
	MappingType  uint8  `json:"mapping_type,omitempty"`
	BytePosition uint8  `json:"byte_position,omitempty"`
	ByteLength   uint8  `json:"byte_length,omitempty"`
	BitPosition  uint8  `json:"bit_position,omitempty"`
	BitLength    uint8  `json:"bit_length,omitempty"`

	ALZ string `json:"-"` //`json:"alz,omitempty"` // AuslieferZuStand

	Conversion string `json:"-"` // `json:"conversion,omitempty"`

	ConversionFactor float32 `json:"conversion_factor,omitempty"`
	ConversionOffset float32 `json:"conversion_offset,omitempty"`
	LowerBorder      float32 `json:"lower_border,omitempty"`
	UpperBorder      float32 `json:"upper_border,omitempty"`

	ValueList string `json:"value_list,omitempty"` // TODO: save as map[string]string or even map[uint32]string?
	Unit      string `json:"unit,omitempty"`

	Codec Codec `json:"codec"`

	Value EventValueType `json:"value,omitempty"`
}

type AddressT uint16

func (a AddressT) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"0x%X\"", a)), nil
}

type EventValueType interface{}

// EventTypeList is just a map of EventTyp (aka command) elements
type EventTypeList map[string]*EventType

// EventTypeAliasList may hold aliases or translated names for commands
type EventTypeAliasList map[string]*EventType

// MemMap holds data of an address space
type MemMap map[uint16]*MemType

// MemType is to hold raw data, including a timestamp used for caching
type MemType struct {
	// The actual raw data
	Data byte
	// Date of last refresh
	CacheTime time.Time
}
