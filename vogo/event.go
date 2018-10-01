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
	Description string

	FCRead  CommandType
	FCWrite CommandType

	Parameter string

	PrefixRead   []byte
	PrefixWrite  []byte
	BlockLength  uint8
	BlockFactor  uint8
	MappingType  uint8
	BytePosition uint8
	ByteLength   uint8
	BitPosition  uint8
	BitLength    uint8

	ALZ string // AuslieferZuStand

	Conversion string

	ConversionFactor float32
	ConversionOffset float32
	LowerBorder      float32
	UpperBorder      float32

	ValueList string // TODO: save as map[string]string or even map[uint32]string?
	Unit      string

	Codec Codec
}

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
