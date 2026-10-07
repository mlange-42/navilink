// Package navilinktest provides an in-memory NaviGPS device for tests.
//
// The device implements the NaviLink protocol independently of package
// navilink, so that tests check the client against a second implementation.
package navilinktest

import (
	"bytes"
	"encoding/binary"
	"slices"
	"sync"
)

// Packet types.
const (
	pidSync            = 0xd6
	pidAck             = 0x0c
	pidNak             = 0x00
	pidQryInformation  = 0x20
	pidQryFwVersion    = 0xfe
	pidData            = 0x03
	pidAddAWaypoint    = 0x3c
	pidQryWaypoints    = 0x28
	pidQryRoute        = 0x24
	pidDelWaypoint     = 0x36
	pidDelAllWaypoint  = 0x37
	pidDelRoute        = 0x34
	pidDelAllRoute     = 0x35
	pidAddARoute       = 0x3d
	pidEraseTrack      = 0x11
	pidReadTrackpoints = 0x14
	pidLogDataAddr     = 0x1c
	pidCmdOk           = 0xf3
	pidQuit            = 0xf2
)

const (
	recordSize  = 32
	maxRoutes   = 20
	nullID      = 0xffff
	trackBuffer = 0x10000
	logStart    = 0x80000
)

var le = binary.LittleEndian

// Device is a simulated NaviGPS. Write sends packets to it,
// Read returns its answers. Read returns (0, nil) if there is no answer,
// like a serial port after a read timeout.
type Device struct {
	Username string
	// Track holds the raw 32-byte trackpoint records.
	Track []byte
	// Log holds the raw 32-byte records of the data logger.
	Log []byte

	mu        sync.Mutex
	waypoints map[uint16][]byte // 32-byte records by waypoint ID
	routes    map[uint8][]byte  // route records by route ID
	in, out   bytes.Buffer
}

// New creates an empty device.
func New() *Device {
	return &Device{
		Username:  "SIMULATOR",
		waypoints: map[uint16][]byte{},
		routes:    map[uint8][]byte{},
	}
}

// Close implements io.Closer.
func (d *Device) Close() error { return nil }

// Read reads answers of the device.
func (d *Device) Read(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.out.Len() == 0 {
		return 0, nil
	}
	return d.out.Read(p)
}

// Write sends packets to the device.
func (d *Device) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.in.Write(p)
	for d.handleNext() {
	}
	return len(p), nil
}

// AddWaypoint stores a waypoint record and returns its ID.
// Only the name and the coordinates are set.
func (d *Device) AddWaypoint(name string, lat, lon float64) uint16 {
	rec := make([]byte, recordSize)
	le.PutUint16(rec, 0x4000)
	copy(rec[4:10], name)
	le.PutUint32(rec[12:], uint32(int32(lat*1e7)))
	le.PutUint32(rec[16:], uint32(int32(lon*1e7)))
	rec[23], rec[24] = 1, 1 // January 1st
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.storeWaypoint(rec)
}

// WaypointNames returns the names of all waypoints, ordered by ID.
func (d *Device) WaypointNames() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var names []string
	for _, id := range d.waypointIDs() {
		names = append(names, cString(d.waypoints[id][4:11]))
	}
	return names
}

// Routes returns the names of all routes and the names of their waypoints,
// ordered by route name.
func (d *Device) Routes() map[string][]string {
	d.mu.Lock()
	defer d.mu.Unlock()
	routes := map[string][]string{}
	for _, rec := range d.routes {
		var names []string
		for _, id := range routeWaypointIDs(rec) {
			names = append(names, cString(d.waypoints[id][4:11]))
		}
		routes[cString(rec[4:18])] = names
	}
	return routes
}

// handleNext handles the next complete packet. It returns false if there is none.
func (d *Device) handleNext() bool {
	buf := d.in.Bytes()
	start := bytes.Index(buf, []byte{0xa0, 0xa2})
	if start < 0 || len(buf) < start+4 {
		return false
	}
	length := int(le.Uint16(buf[start+2:]))
	end := start + 4 + length + 4
	if len(buf) < end {
		return false
	}
	payload := slices.Clone(buf[start+4 : start+4+length])
	d.in.Next(end)
	d.handle(payload[0], payload[1:])
	return true
}

func (d *Device) answer(typ byte, data []byte) {
	payload := append([]byte{typ}, data...)
	var sum uint16
	for _, b := range payload {
		sum += uint16(b)
	}
	d.out.Write([]byte{0xa0, 0xa2})
	d.out.Write(le.AppendUint16(nil, uint16(len(payload))))
	d.out.Write(payload)
	d.out.Write(le.AppendUint16(nil, sum&0x7fff))
	d.out.Write([]byte{0xb0, 0xb3})
}

func (d *Device) handle(typ byte, data []byte) {
	switch typ {
	case pidSync:
		d.answer(pidAck, nil)
	case pidAck, pidQuit:
		// no answer
	case pidQryInformation:
		d.answer(pidData, d.info())
	case pidQryFwVersion:
		d.answer(pidData, []byte("1.00,simulator\x00"))
	case pidQryWaypoints:
		d.queryWaypoints(data)
	case pidAddAWaypoint:
		d.addWaypoint(data)
	case pidDelWaypoint:
		d.deleteWaypoint(data)
	case pidDelAllWaypoint:
		d.deleteAllWaypoints()
	case pidQryRoute:
		d.queryRoute(data)
	case pidAddARoute:
		d.addRoute(data)
	case pidDelRoute:
		d.deleteRoute(data)
	case pidDelAllRoute:
		clear(d.routes)
		d.answer(pidAck, nil)
	case pidReadTrackpoints:
		d.readMemory(data)
	case pidLogDataAddr:
		addr := le.AppendUint32(nil, logStart)
		addr = append(addr, make([]byte, 8)...)
		d.answer(pidData, le.AppendUint32(addr, logStart+uint32(len(d.Log))))
	case pidEraseTrack:
		d.Track = nil
		d.answer(pidCmdOk, nil)
	default:
		d.answer(pidNak, nil)
	}
}

func (d *Device) info() []byte {
	data := le.AppendUint16(nil, uint16(len(d.waypoints)))
	data = append(data, byte(len(d.routes)), 1)
	data = le.AppendUint32(data, trackBuffer)
	data = le.AppendUint32(data, 12345)
	data = le.AppendUint16(data, uint16(len(d.Track)/recordSize))
	data = le.AppendUint16(data, 1)
	data = append(data, make([]byte, 16)...)
	name := make([]byte, 16)
	copy(name, d.Username)
	return append(data, name...)
}

func (d *Device) waypointIDs() []uint16 {
	ids := make([]uint16, 0, len(d.waypoints))
	for id := range d.waypoints {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (d *Device) storeWaypoint(rec []byte) uint16 {
	id := uint16(0)
	for d.waypoints[id] != nil {
		id++
	}
	rec = slices.Clone(rec)
	le.PutUint16(rec[2:], id)
	d.waypoints[id] = rec
	return id
}

func (d *Device) queryWaypoints(data []byte) {
	ids := d.waypointIDs()
	start, count := int(le.Uint32(data)), int(le.Uint16(data[4:]))
	if len(data) != 7 || start+count > len(ids) {
		d.answer(pidNak, nil)
		return
	}
	var out []byte
	for _, id := range ids[start : start+count] {
		out = append(out, d.waypoints[id]...)
	}
	d.answer(pidData, out)
}

func (d *Device) addWaypoint(data []byte) {
	if len(data) != recordSize || data[31] != 0x7e {
		d.answer(pidNak, nil)
		return
	}
	id := d.storeWaypoint(data)
	d.answer(pidData, le.AppendUint16(nil, id))
}

func (d *Device) inRoute(id uint16) bool {
	for _, rec := range d.routes {
		if slices.Contains(routeWaypointIDs(rec), id) {
			return true
		}
	}
	return false
}

func (d *Device) deleteWaypoint(data []byte) {
	if len(data) != 4 {
		d.answer(pidNak, nil)
		return
	}
	id := le.Uint16(data[2:])
	if d.waypoints[id] == nil || d.inRoute(id) {
		d.answer(pidNak, nil)
		return
	}
	delete(d.waypoints, id)
	d.answer(pidAck, nil)
}

// deleteAllWaypoints fails if there are routes, as described in the specification.
func (d *Device) deleteAllWaypoints() {
	if len(d.routes) > 0 {
		d.answer(pidNak, nil)
		return
	}
	clear(d.waypoints)
	d.answer(pidAck, nil)
}

func (d *Device) queryRoute(data []byte) {
	recs := make([][]byte, 0, len(d.routes))
	for _, rec := range d.routes {
		recs = append(recs, rec)
	}
	slices.SortFunc(recs, func(a, b []byte) int { return bytes.Compare(a[4:18], b[4:18]) })

	idx := int(le.Uint32(data))
	if len(data) != 7 || idx >= len(recs) {
		d.answer(pidNak, nil)
		return
	}
	d.answer(pidData, recs[idx])
}

func (d *Device) addRoute(data []byte) {
	numSub := len(data)/recordSize - 1
	if len(data)%recordSize != 0 || numSub < 1 || numSub > 9 || len(d.routes) >= maxRoutes {
		d.answer(pidNak, nil)
		return
	}
	if data[recordSize*(numSub+1)-2] != 0x7f { // last subroute tag
		d.answer(pidNak, nil)
		return
	}
	ids := routeWaypointIDs(data)
	if len(ids) == 0 || len(ids) == numSub*14 { // missing null ID
		d.answer(pidNak, nil)
		return
	}
	for _, id := range ids {
		if d.waypoints[id] == nil {
			d.answer(pidNak, nil)
			return
		}
	}
	id := uint8(0)
	for d.routes[id] != nil {
		id++
	}
	rec := slices.Clone(data)
	rec[2] = id
	d.routes[id] = rec
	d.answer(pidData, []byte{id})
}

func (d *Device) deleteRoute(data []byte) {
	if len(data) != 4 || d.routes[uint8(le.Uint16(data[2:]))] == nil {
		d.answer(pidNak, nil)
		return
	}
	delete(d.routes, uint8(le.Uint16(data[2:])))
	d.answer(pidAck, nil)
}

func (d *Device) readMemory(data []byte) {
	addr, size := le.Uint32(data), uint32(le.Uint16(data[4:]))
	mem, base := d.Track, uint32(trackBuffer)
	if addr >= logStart {
		mem, base = d.Log, logStart
	}
	if addr < base || addr-base+size > uint32(len(mem)) {
		d.answer(pidNak, nil)
		return
	}
	d.answer(pidData, mem[addr-base:addr-base+size])
}

// routeWaypointIDs returns the waypoint IDs of a route record, up to the first null ID.
func routeWaypointIDs(rec []byte) []uint16 {
	var ids []uint16
	for off := recordSize; off+recordSize <= len(rec); off += recordSize {
		for i := range 14 {
			id := le.Uint16(rec[off+2+2*i:])
			if id == nullID {
				return ids
			}
			ids = append(ids, id)
		}
	}
	return ids
}

func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
