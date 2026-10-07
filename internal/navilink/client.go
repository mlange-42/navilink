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
)

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
	return decodeTrackpoints(data), nil
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
	return decodeTrackpoints(data), nil
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
