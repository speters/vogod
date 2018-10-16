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
	ID          string `json:"id"`
	Address     uint16 `json:"address"`
	Description string `json:"description,omitempty"`

	FCRead  CommandType `json:"fcread"`
	FCWrite CommandType `json:"fcwrite"`

	Parameter string `json:"parameter"`

	PrefixRead   []byte `json:"prefixread,omitempty"`
	PrefixWrite  []byte `json:"prefixwrite,omitempty"`
	BlockLength  uint8  `json:"blocklength,omitempty"`
	BlockFactor  uint8  `json:"blockfactor,omitempty"`
	MappingType  uint8  `json:"mappingtype,omitempty"`
	BytePosition uint8  `json:"byteposition,omitempty"`
	ByteLength   uint8  `json:"bytelength,omitempty"`
	BitPosition  uint8  `json:"bitposition,omitempty"`
	BitLength    uint8  `json:"bitlength,omitempty"`

	ALZ string `json:"alz,omitempty"` // AuslieferZuStand

	Conversion string `json:"conversion,omitempty"`

	ConversionFactor float32 `json:"conversionfactor,omitempty"`
	ConversionOffset float32 `json:"conversionoffset,omitempty"`
	LowerBorder      float32 `json:"lowerborder,omitempty"`
	UpperBorder      float32 `json:"upperborder,omitempty"`

	ValueList string `json:"valuelist,omitempty"` // TODO: save as map[string]string or even map[uint32]string?
	Unit      string `json:"unit,omitempty"`

	Codec Codec `json:"codec"`

	Value EventValueType `json:"value,omitempty"`
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
