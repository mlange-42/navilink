package navilink

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

const (
	feetToMeters = 0.3048
	coordScale   = 1e7
	recordSize   = 32
)

// Info holds general information about the device.
type Info struct {
	Waypoints   uint16
	Routes      uint8
	Tracks      uint8
	TrackBuffer uint32 // start address of the track buffer
	Serial      uint32
	Trackpoints uint16
	Protocol    uint16
	Username    string
}

func decodeInfo(data []byte) (Info, error) {
	if len(data) < 32 {
		return Info{}, fmt.Errorf("info data too short: %d bytes", len(data))
	}
	le := binary.LittleEndian
	return Info{
		Waypoints:   le.Uint16(data[0:]),
		Routes:      data[2],
		Tracks:      data[3],
		TrackBuffer: le.Uint32(data[4:]),
		Serial:      le.Uint32(data[8:]),
		Trackpoints: le.Uint16(data[12:]),
		Protocol:    le.Uint16(data[14:]),
		Username:    cString(data[len(data)-16:]),
	}, nil
}

// Trackpoint is a single point of a track or the data log.
type Trackpoint struct {
	Lat, Lon float64 // degrees
	Altitude float64 // meters
	Time     time.Time
	Speed    float64 // km/h
}

func decodeTrackpoint(rec []byte) Trackpoint {
	le := binary.LittleEndian
	return Trackpoint{
		Lat:      float64(int32(le.Uint32(rec[12:]))) / coordScale,
		Lon:      float64(int32(le.Uint32(rec[16:]))) / coordScale,
		Altitude: float64(le.Uint16(rec[20:])) * feetToMeters,
		Time:     decodeTime(rec[22:28]),
		Speed:    float64(rec[29]) * 2,
	}
}

// decodeLogPoint decodes a record of the internal data logger
// (Locosys SBP format, as used by the BGT-31/GT-31).
func decodeLogPoint(rec []byte) Trackpoint {
	le := binary.LittleEndian
	return Trackpoint{
		Lat:      float64(int32(le.Uint32(rec[12:]))) / coordScale,
		Lon:      float64(int32(le.Uint32(rec[16:]))) / coordScale,
		Altitude: float64(int32(le.Uint32(rec[20:]))) / 100, // cm
		Time:     decodePackedTime(le.Uint32(rec[4:])),
		Speed:    float64(le.Uint16(rec[24:])) * 0.036, // cm/s
	}
}

// decodeRecords decodes all complete 32-byte records in data.
func decodeRecords(data []byte, decode func([]byte) Trackpoint) []Trackpoint {
	points := make([]Trackpoint, 0, len(data)/recordSize)
	for i := 0; i+recordSize <= len(data); i += recordSize {
		points = append(points, decode(data[i:i+recordSize]))
	}
	return points
}

// Waypoint is a waypoint stored on the device.
type Waypoint struct {
	ID       uint16
	Name     string  // up to 6 characters
	Lat, Lon float64 // degrees
	Altitude float64 // meters
	Time     time.Time
	Symbol   uint8
}

func decodeWaypoint(rec []byte) Waypoint {
	le := binary.LittleEndian
	return Waypoint{
		ID:       le.Uint16(rec[2:]),
		Name:     string(bytes.TrimRight(rec[4:11], "\x00 \t\r\n")),
		Lat:      float64(int32(le.Uint32(rec[12:]))) / coordScale,
		Lon:      float64(int32(le.Uint32(rec[16:]))) / coordScale,
		Altitude: float64(le.Uint16(rec[20:])) * feetToMeters,
		Time:     decodeTime(rec[22:28]),
		Symbol:   rec[28],
	}
}

// decodeTime decodes a 6-byte date and time (year since 2000, month, day,
// hour, minute, second). Times are assumed to be UTC.
func decodeTime(b []byte) time.Time {
	return time.Date(2000+int(b[0]), time.Month(b[1]), int(b[2]),
		int(b[3]), int(b[4]), int(b[5]), 0, time.UTC)
}

// decodePackedTime decodes a date and time packed into 32 bits:
// year*12+month (bits 22-31, years since 2000), day (17-21), hour (12-16),
// minute (6-11) and second (0-5). Times are UTC.
func decodePackedTime(v uint32) time.Time {
	ym := int(v >> 22)
	return time.Date(2000+ym/12, time.Month(ym%12), int(v>>17&0x1f),
		int(v>>12&0x1f), int(v>>6&0x3f), int(v&0x3f), 0, time.UTC)
}

func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// encodeWaypoint encodes a waypoint for upload with PidAddAWaypoint.
// The device assigns the ID, so the ID of the waypoint is ignored.
func encodeWaypoint(wp Waypoint) []byte {
	le := binary.LittleEndian
	msg := []byte{0x00, 0x40, 0x00, 0x00}

	name := make([]byte, 6)
	copy(name, wp.Name)
	msg = append(msg, name...)
	msg = append(msg, 0x00, 0x00)

	msg = le.AppendUint32(msg, uint32(int32(math.Round(wp.Lat*coordScale))))
	msg = le.AppendUint32(msg, uint32(int32(math.Round(wp.Lon*coordScale))))
	alt := math.Round(wp.Altitude / feetToMeters)
	msg = le.AppendUint16(msg, uint16(max(0, min(alt, math.MaxUint16))))
	msg = append(msg, encodeTime(wp.Time)...)
	msg = append(msg, wp.Symbol, 0x00, 0x7e)
	return msg
}

// encodeTime encodes a time as 6 bytes (see decodeTime).
// Times outside the years 2000-2255 are encoded as 2000-01-01T00:00:00.
func encodeTime(t time.Time) []byte {
	t = t.UTC()
	if t.Year() < 2000 || t.Year() > 2255 {
		return []byte{0, 1, 1, 0, 0, 0}
	}
	return []byte{byte(t.Year() - 2000), byte(t.Month()), byte(t.Day()),
		byte(t.Hour()), byte(t.Minute()), byte(t.Second())}
}
