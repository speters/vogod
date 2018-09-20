package optolink

import (
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

// xDataPointType is a type to hold raw information read from xml
type xDataPointType struct {
	ID                          string `xml:"ID"`
	EventtTypeList              string `xml:"EventTypeList"`
	Description                 string `xml:"Description"`
	Identification              string `xml:"Identification"`
	IdentificationExtension     string `xml:"IdentificationExtension"`
	IdentificationExtensionTill string `xml:"IdentificationExtensionTill"`
}

// DataPointType is a type to describe a DataPoint (aka a Vito* device)
type DataPointType struct {
	ID             string
	Description    string
	SysDeviceIdent [8]byte
	EventTypes     EventTypeList
}

// EventType holds low-level info for commands like address, data format and conversion hints
type EventType struct {
	ID      string
	Address string

	AccessMode string
	FCRead     string
	FCWrite    string

	Parameter   string
	SDKDataType string

	BlockLength  string
	BytePosition string
	ByteLength   string
	BitPosition  string
	BitLength    string

	Conversion string
}

// EventTypeList is just a map of EventTyp (aka command) elements
type EventTypeList map[string]EventType

// EventTypeAliasList may hold aliases or translated names for commands
type EventTypeAliasList map[string]*EventType

// ErrNotFound is returned when data could not be found
var ErrNotFound = errors.New("Not found")

// FindDataPointType reads DataPoint info from xml in a format similar to VitoSofts ecnDataPointType.xml format
func FindDataPointType(xmlReader io.Reader, sysDeviceIdent [8]byte, dpt *DataPointType) error {
	decoder := xml.NewDecoder(xmlReader)
	var dp xDataPointType

	for {
		t, _ := decoder.Token()
		if t == nil {
			break
		}
		switch se := t.(type) {
		case xml.StartElement:
			if se.Name.Local == "DataPointType" {
				var d xDataPointType
				decoder.DecodeElement(&d, &se)

				if len(d.Identification) != 4 {
					break
				}

				if (len(d.IdentificationExtension) == 0 || (len(d.IdentificationExtension) >= 4 && len(d.IdentificationExtension) <= 6)) &&
					(len(d.IdentificationExtensionTill) == 0 || (len(d.IdentificationExtensionTill) >= 4 && len(d.IdentificationExtensionTill) <= 6)) {

					i, err := strconv.ParseUint("0x00"+d.Identification, 0, 16)
					if err != nil {
						break
					}

					// Match sysDeviceGroupIdent
					if sysDeviceIdent[0] != byte((i>>8)&0xff) {
						break
					}
					// Match sysDeviceIdent
					if sysDeviceIdent[1] != byte(i&0xff) {
						break
					}

					idExt, err := strconv.ParseUint("0x00"+d.IdentificationExtension, 0, 24)
					if err != nil {
						idExt = 0

					}
					idExtTill, err := strconv.ParseUint("0x00"+d.IdentificationExtensionTill, 0, 24)
					if err != nil {
						idExtTill = 0
					}

					var dataPointIdExt uint64
					dataPointIdExt = uint64(sysDeviceIdent[2])<<8 | uint64(sysDeviceIdent[3])
					if (len(d.IdentificationExtension) > 4) || (len(d.IdentificationExtensionTill) > 4) {
						dataPointIdExt = uint64(dataPointIdExt)<<16 | uint64(sysDeviceIdent[4])<<8 | uint64(sysDeviceIdent[5])
					}
					if dataPointIdExt >= idExt && (dataPointIdExt < idExtTill || idExtTill == 0) {
						if dp.ID == "" {
							// First match, break as there is nothing to compare
							dp = d
							break
						}

						dpidExt, err := strconv.ParseUint("0x00"+dp.IdentificationExtension, 0, 24)
						if err != nil {
							dpidExt = 0
						}
						dpidExtTill, err := strconv.ParseUint("0x00"+dp.IdentificationExtensionTill, 0, 24)
						if err != nil {
							dpidExtTill = 0
						}

						if idExt >= dpidExt && (idExtTill < dpidExtTill || dpidExtTill == 0) {
							// A better match than the previously found one
							dp = d
						}
					}
				}
			}
		default:
			//
		}
	}
	if dp.ID != "" {
		r := dpt
		r.ID = dp.ID
		r.Description = dp.Description
		r.SysDeviceIdent = sysDeviceIdent
		etl := dpt.EventTypes

		for _, et := range strings.Split(dp.EventtTypeList, ";") {
			etl[et] = EventType{ID: et}
		}
		r.EventTypes = etl

		return nil
	}
	return ErrNotFound
}

// FindEventTypes reads EventType infos from xml in a format similar to VitoSofts ecnEventType.xml format
func FindEventTypes(xmlReader io.Reader, etl *EventTypeList) int {
	decoder := xml.NewDecoder(xmlReader)
	found := 0

	for {
		t, _ := decoder.Token()
		if t == nil {
			break
		}
		switch se := t.(type) {
		case xml.StartElement:
			if se.Name.Local == "EventType" {
				var et EventType
				decoder.DecodeElement(&et, &se)

				if _, ok := (*etl)[et.ID]; !ok {
					break
				}
				(*etl)[et.ID] = et
				found++

			}
		default:
			//
		}
	}
	return found
}
