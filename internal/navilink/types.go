package navilink

import (
	"bytes"
	"encoding/binary"
	"fmt"
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

func decodeTrackpoints(data []byte) []Trackpoint {
	points := make([]Trackpoint, 0, len(data)/recordSize)
	for i := 0; i+recordSize <= len(data); i += recordSize {
		points = append(points, decodeTrackpoint(data[i:i+recordSize]))
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

func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
