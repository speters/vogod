package optolink

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
	Address     string
	Description string

	//AccessMode string // Redundant, see FCRead, FCWrite
	FCRead  string
	FCWrite string

	Parameter string
	/*
			SDKDataType string // Redundant, see Parameter

		    SDKDataType <- Parameter
			"ByteArray"   "Array"
			"ByteArray"   "Byte"
			"Double"	  "Int"
			"Int"         "Int4"
			"Int"         "IntHighByteFirst"
			"Double"      "SByte"
			"Int"         "SInt"
			"Int"         "SInt4"
			"Double"      "SIntHighByteFirst"
			"ByteArray"   "String"
			"ByteArray"   "StringCR"
			"ByteArray"   "StringNT"
	*/
	PrefixRead   string
	PrefixWrite  string
	BlockLength  string
	BlockFactor  string
	BytePosition string
	ByteLength   string
	BitPosition  string
	BitLength    string

	ALZ string // AuslieferZuStand

	// Conversion is the converter function to use
	/*
	   "DateBCD"
	   "DateMBus"
	   "DateTimeBCD"
	   "DateTimeMBus"
	   "DatenpunktADDR"
	   "Div10"
	   "Div100"
	   "Div1000"
	   "Div2"
	   "Estrich"
	   "HexByte2AsciiByte"
	   "HexByte2DecimalByte"
	   "HexToFloat"
	   "HourDiffSec2Hour"
	   "IPAddress"
	   "Kesselfolge"
	   "Mult10"
	   "Mult100"
	   "Mult2"
	   "Mult5"
	   "MultOffset"
	   "MultOffsetBCD"
	   "MultOffsetFloat"
	   "NoConversion"
	   "Phone2BCD"
	   "RotateBytes"
	   "Sec2Hour"
	   "Sec2Minute"
	   "Time53"
	   "UTCDiff2Month"
	*/
	Conversion string

	ConversionFactor string
	ConversionOffset string
	LowerBorder      string
	UpperBorder      string
	Stepping         string
	MappingType      string
	// DataType         string // Redundant, is set to "OptionList" when ValueList is not empty
	// OptionList       string
	ValueList string
	Unit      string
}

// EventTypeList is just a map of EventTyp (aka command) elements
type EventTypeList map[string]EventType

// EventTypeAliasList may hold aliases or translated names for commands
type EventTypeAliasList map[string]*EventType
