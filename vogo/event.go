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

	//AccessMode string // Redundant, see FCRead, FCWrite
	FCRead  CommandType
	FCWrite CommandType

	Parameter   string // TODO: keep this or SDKDataType?
	SDKDataType string
	/*
			SDKDataType string // Redundant, see Parameter

		    SDKDataType <- Parameter
			ByteArray   Array
			ByteArray   Byte
			Double      Int
			Int         Int4
			Int         IntHighByteFirst
			Double      SByte
			Int         SInt
			Int         SInt4
			Double      SIntHighByteFirst
			ByteArray   String
			ByteArray   StringCR
			ByteArray   StringNT
	*/
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

	// TODO: Could this be solved with function pointers?
	// Textual representation of the conversion function
	Conversion string

	ConversionFactor float32
	ConversionOffset float32
	LowerBorder      float32
	UpperBorder      float32
	Stepping         float32 // TODO: check if this is given implicitely by conversion

	// DataType         string // Redundant, is set to "OptionList" when ValueList is not empty
	// OptionList       string
	ValueList string
	Unit      string
}

// EventTypeList is just a map of EventTyp (aka command) elements
type EventTypeList map[string]EventType

// EventTypeAliasList may hold aliases or translated names for commands
type EventTypeAliasList map[string]*EventType

// MemMap holds data of an address space
type MemMap map[uint16]MemType

// MemType is to hold raw data, including a timestamp used for caching
type MemType struct {
	// The actual raw data
	Data byte
	// Date of last refresh
	CacheTime time.Time
}

/*
type EventDecoder interface {
	Decode(data *MemType, e *EventType) (val interface{}, err error)
}
type EventEncoder interface {
	Encode(val interface{}, e *EventType) (data *MemType, err error)
}

func Decode(data *MemType, e *EventType) (val interface{}, err error) {
	return val, err
}
func Encode(val interface{}, e *EventType) (data MemType, err error) {
	return data, err
}
*/
