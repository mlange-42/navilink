package navilink

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

const (
	trackChunkSize    = 512 * recordSize // bytes per track/log read request
	waypointChunkSize = 32               // waypoints per read request
	eraseBlockPoints  = 4096             // trackpoints per erase request
)

// ErrRefused is returned when the device answers a request with
// PidNak or PidCmdFail.
var ErrRefused = errors.New("refused by the device")

// Client implements the NaviLink commands on top of a Conn.
type Client struct {
	conn *Conn
}

// NewClient creates a new client using the given connection.
func NewClient(conn *Conn) *Client {
	return &Client{conn: conn}
}

// request sends a packet and checks that the response has the wanted type.
func (c *Client) request(typ byte, data []byte, want byte) ([]byte, error) {
	if err := c.conn.Send(typ, data); err != nil {
		return nil, err
	}
	rTyp, rData, err := c.conn.Receive()
	if err != nil {
		return nil, err
	}
	if rTyp != want {
		if rTyp == PidNak || rTyp == PidCmdFail {
			return nil, ErrRefused
		}
		return nil, fmt.Errorf("unexpected response type 0x%02x to request 0x%02x (want 0x%02x)", rTyp, typ, want)
	}
	return rData, nil
}

// Sync starts the communication with the device.
func (c *Client) Sync() error {
	if _, err := c.request(PidSync, nil, PidAck); err != nil {
		return fmt.Errorf("failed to start the communication, did you enable the NaviLink mode? %w", err)
	}
	return nil
}

// Quit ends the NaviLink mode of the device.
func (c *Client) Quit() error {
	return c.conn.Send(PidQuit, nil)
}

// Info reads general information from the device.
func (c *Client) Info() (Info, error) {
	data, err := c.request(PidQryInformation, nil, PidData)
	if err != nil {
		return Info{}, fmt.Errorf("reading device info: %w", err)
	}
	return decodeInfo(data)
}

// FirmwareVersion reads the firmware version of the device.
func (c *Client) FirmwareVersion() (string, error) {
	data, err := c.request(PidQryFwVersion, nil, PidData)
	if err != nil {
		return "", fmt.Errorf("reading firmware version: %w", err)
	}
	version, _, _ := strings.Cut(cString(data), ",")
	return version, nil
}

// Waypoints reads all waypoints from the device.
func (c *Client) Waypoints() ([]Waypoint, error) {
	info, err := c.Info()
	if err != nil {
		return nil, err
	}

	total := int(info.Waypoints)
	points := make([]Waypoint, 0, total)
	for read := 0; read < total; {
		count := min(total-read, waypointChunkSize)

		msg := binary.LittleEndian.AppendUint32(nil, uint32(read))
		msg = binary.LittleEndian.AppendUint16(msg, uint16(count))
		msg = append(msg, 0x01)
		data, err := c.request(PidQryWaypoints, msg, PidData)
		if err != nil {
			return nil, fmt.Errorf("reading waypoints: %w", err)
		}
		if len(data) < count*recordSize {
			return nil, fmt.Errorf("reading waypoints: expected %d bytes, got %d", count*recordSize, len(data))
		}
		for i := range count {
			points = append(points, decodeWaypoint(data[i*recordSize:(i+1)*recordSize]))
		}
		read += count
	}
	return points, nil
}

// Trackpoints reads all trackpoints from the device.
func (c *Client) Trackpoints() ([]Trackpoint, error) {
	info, err := c.Info()
	if err != nil {
		return nil, err
	}
	data, err := c.readMemory(info.TrackBuffer, uint32(info.Trackpoints)*recordSize)
	if err != nil {
		return nil, fmt.Errorf("reading trackpoints: %w", err)
	}
	return decodeRecords(data, decodeTrackpoint), nil
}

// LogPoints reads all points from the internal data logger (BGT-31/GT-31 only).
func (c *Client) LogPoints() ([]Trackpoint, error) {
	data, err := c.request(PidLogDataAddr, nil, PidData)
	if err != nil {
		return nil, fmt.Errorf("reading log address: %w", err)
	}
	if len(data) < 16 {
		return nil, fmt.Errorf("reading log address: data too short: %d bytes", len(data))
	}
	start := binary.LittleEndian.Uint32(data[0:])
	end := binary.LittleEndian.Uint32(data[12:])
	if end < start {
		return nil, errors.New("reading log address: end address before start address")
	}

	data, err = c.readMemory(start, end-start)
	if err != nil {
		return nil, fmt.Errorf("reading log data: %w", err)
	}
	return decodeRecords(data, decodeLogPoint), nil
}

// readMemory reads size bytes starting at addr, in chunks.
func (c *Client) readMemory(addr, size uint32) ([]byte, error) {
	out := make([]byte, 0, size)
	for read := uint32(0); read < size; {
		count := min(size-read, trackChunkSize)

		msg := binary.LittleEndian.AppendUint32(nil, addr+read)
		msg = binary.LittleEndian.AppendUint16(msg, uint16(count))
		msg = append(msg, 0x00)
		data, err := c.request(PidReadTrackpoints, msg, PidData)
		if err != nil {
			return nil, err
		}
		out = append(out, data...)
		read += count

		if err := c.conn.Send(PidAck, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// AddWaypoint uploads a waypoint to the device.
func (c *Client) AddWaypoint(wp Waypoint) error {
	if _, err := c.request(PidAddAWaypoint, encodeWaypoint(wp), PidData); err != nil {
		return fmt.Errorf("adding waypoint %s: %w", wp.Name, err)
	}
	return nil
}

// DeleteWaypoint deletes the waypoint with the given ID from the device.
func (c *Client) DeleteWaypoint(id uint16) error {
	msg := binary.LittleEndian.AppendUint16([]byte{0x00, 0x00}, id)
	if _, err := c.request(PidDelWaypoint, msg, PidAck); err != nil {
		return fmt.Errorf("deleting waypoint %d: %w", id, err)
	}
	return nil
}

// DeleteAllWaypoints deletes all waypoints from the device.
func (c *Client) DeleteAllWaypoints() error {
	if _, err := c.request(PidDelAllWaypoint, []byte{0x00, 0xf0, 0x00, 0x00}, PidAck); err != nil {
		return fmt.Errorf("deleting all waypoints: %w", err)
	}
	return nil
}

// DeleteTrack deletes all trackpoints from the device.
// Trackpoints are erased in blocks of eraseBlockPoints points.
func (c *Client) DeleteTrack() error {
	info, err := c.Info()
	if err != nil {
		return err
	}
	for start := 0; start == 0 || start < int(info.Trackpoints); start += eraseBlockPoints {
		addr := info.TrackBuffer + uint32(start)*recordSize
		msg := binary.LittleEndian.AppendUint32(nil, addr)
		msg = append(msg, 0x00, 0x00, 0x00)
		if _, err := c.request(PidEraseTrack, msg, PidCmdOk); err != nil {
			return fmt.Errorf("deleting trackpoints: %w", err)
		}
	}
	return nil
}

// Routes reads all routes from the device.
func (c *Client) Routes() ([]Route, error) {
	info, err := c.Info()
	if err != nil {
		return nil, err
	}
	routes := make([]Route, 0, info.Routes)
	for i := range uint32(info.Routes) {
		msg := binary.LittleEndian.AppendUint32(nil, i)
		msg = append(msg, 0x00, 0x00, 0x01)
		data, err := c.request(PidQryRoute, msg, PidData)
		if err != nil {
			return nil, fmt.Errorf("reading route %d: %w", i, err)
		}
		r, err := decodeRoute(data)
		if err != nil {
			return nil, fmt.Errorf("reading route %d: %w", i, err)
		}
		routes = append(routes, r)
	}
	return routes, nil
}

// AddRoute uploads a route to the device.
// All waypoints referenced by the route must exist on the device.
func (c *Client) AddRoute(r Route) error {
	msg, err := encodeRoute(r)
	if err != nil {
		return err
	}
	if _, err := c.request(PidAddARoute, msg, PidData); err != nil {
		return fmt.Errorf("adding route %s: %w", r.Name, err)
	}
	return nil
}

// DeleteRoute deletes the route with the given ID from the device.
func (c *Client) DeleteRoute(id uint8) error {
	msg := binary.LittleEndian.AppendUint16([]byte{0x00, 0x00}, uint16(id))
	if _, err := c.request(PidDelRoute, msg, PidAck); err != nil {
		return fmt.Errorf("deleting route %d: %w", id, err)
	}
	return nil
}

// DeleteAllRoutes deletes all routes from the device.
func (c *Client) DeleteAllRoutes() error {
	if _, err := c.request(PidDelAllRoute, []byte{0x00, 0xf0, 0x00, 0x00}, PidAck); err != nil {
		return fmt.Errorf("deleting all routes: %w", err)
	}
	return nil
}
