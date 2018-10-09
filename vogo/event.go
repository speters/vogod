package vogo

import "time"

// DataPointType is a type to describe a DataPoint (aka a Vito* device)
type DataPointType struct {
	ID             string
	Description    string
	SysDeviceIdent [8]byte
	EventTypes     EventTypeList
}

// EventType holds low-level info for commands like address, data format and conversion hints
type EventType struct {
	ID          string
	Address     uint16
	Description string `json:",omitempty"`

	FCRead  CommandType
	FCWrite CommandType

	Parameter string

	PrefixRead   []byte `json:",omitempty"`
	PrefixWrite  []byte `json:",omitempty"`
	BlockLength  uint8  `json:",omitempty"`
	BlockFactor  uint8  `json:",omitempty"`
	MappingType  uint8  `json:",omitempty"`
	BytePosition uint8  `json:",omitempty"`
	ByteLength   uint8  `json:",omitempty"`
	BitPosition  uint8  `json:",omitempty"`
	BitLength    uint8  `json:",omitempty"`

	ALZ string `json:",omitempty"` // AuslieferZuStand

	Conversion string `json:",omitempty"`

	ConversionFactor float32 `json:",omitempty"`
	ConversionOffset float32 `json:",omitempty"`
	LowerBorder      float32 `json:",omitempty"`
	UpperBorder      float32 `json:",omitempty"`

	ValueList string `json:",omitempty"` // TODO: save as map[string]string or even map[uint32]string?
	Unit      string `json:",omitempty"`

	Codec Codec

	Value EventValueType `json:",omitempty"`
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
