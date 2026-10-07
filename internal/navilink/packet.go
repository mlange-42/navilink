// Package navilink implements the NaviLink protocol used to communicate
// with Locosys NaviGPS devices over a serial connection.
//
// See http://wiki.splitbrain.org/navilink for the protocol specification.
package navilink

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Packet types.
const (
	PidSync            byte = 0xd6
	PidAck             byte = 0x0c
	PidNak             byte = 0x00
	PidQryInformation  byte = 0x20
	PidQryFwVersion    byte = 0xfe
	PidData            byte = 0x03
	PidAddAWaypoint    byte = 0x3c
	PidQryWaypoints    byte = 0x28
	PidQryRoute        byte = 0x24
	PidDelWaypoint     byte = 0x36
	PidDelAllWaypoint  byte = 0x37
	PidDelRoute        byte = 0x34
	PidDelAllRoute     byte = 0x35
	PidAddARoute       byte = 0x3d
	PidEraseTrack      byte = 0x11
	PidReadTrackpoints byte = 0x14
	PidReadLogHeader   byte = 0x50
	PidLogDataAddr     byte = 0x1c
	PidCmdOk           byte = 0xf3
	PidCmdFail         byte = 0xf4
	PidQuit            byte = 0xf2
)

var (
	startSeq = []byte{0xa0, 0xa2}
	endSeq   = []byte{0xb0, 0xb3}
)

// DefaultTimeout is the default time to wait for a complete packet.
const DefaultTimeout = 8 * time.Second

// checksum calculates the checksum over a packet payload (type and data).
func checksum(payload []byte) uint16 {
	var sum uint32
	for _, b := range payload {
		sum += uint32(b)
	}
	return uint16(sum & 0x7fff)
}

// encodePacket frames a packet of the given type and data.
func encodePacket(typ byte, data []byte) ([]byte, error) {
	if len(data)+1 > 0xffff {
		return nil, fmt.Errorf("packet data too long: %d bytes", len(data))
	}
	payload := append([]byte{typ}, data...)

	packet := make([]byte, 0, len(payload)+8)
	packet = append(packet, startSeq...)
	packet = binary.LittleEndian.AppendUint16(packet, uint16(len(payload)))
	packet = append(packet, payload...)
	packet = binary.LittleEndian.AppendUint16(packet, checksum(payload))
	packet = append(packet, endSeq...)
	return packet, nil
}

// Conn sends and receives NaviLink packets over a byte stream.
//
// Reads from the underlying stream may return (0, nil) on a read timeout,
// as serial ports do; Conn keeps reading until Timeout has elapsed.
type Conn struct {
	rw      io.ReadWriter
	Timeout time.Duration
	// Debug receives hex dumps of all packets if not nil.
	Debug io.Writer

	buf     []byte
	pending []byte
}

// NewConn creates a new connection on top of the given stream.
func NewConn(rw io.ReadWriter) *Conn {
	return &Conn{
		rw:      rw,
		Timeout: DefaultTimeout,
		buf:     make([]byte, 4096),
	}
}

// Send sends a packet of the given type and data.
func (c *Conn) Send(typ byte, data []byte) error {
	packet, err := encodePacket(typ, data)
	if err != nil {
		return err
	}
	if c.Debug != nil {
		_, _ = fmt.Fprintf(c.Debug, "-> %s\n", hexdump(packet))
	}
	_, err = c.rw.Write(packet)
	return err
}

// Receive reads the next packet and returns its type and data.
func (c *Conn) Receive() (byte, []byte, error) {
	deadline := time.Now().Add(c.Timeout)

	// Find the start sequence, skipping any garbage before it.
	prev := byte(0)
	for {
		b, err := c.readFull(1, deadline)
		if err != nil {
			return 0, nil, err
		}
		if prev == startSeq[0] && b[0] == startSeq[1] {
			break
		}
		prev = b[0]
	}

	lenBytes, err := c.readFull(2, deadline)
	if err != nil {
		return 0, nil, err
	}
	length := int(binary.LittleEndian.Uint16(lenBytes))
	if length == 0 {
		return 0, nil, errors.New("received packet with empty payload")
	}

	rest, err := c.readFull(length+4, deadline)
	if err != nil {
		return 0, nil, err
	}

	if c.Debug != nil {
		packet := append(append(append([]byte{}, startSeq...), lenBytes...), rest...)
		_, _ = fmt.Fprintf(c.Debug, "<- %s\n", hexdump(packet))
	}

	payload := rest[:length]
	sum := binary.LittleEndian.Uint16(rest[length : length+2])
	if !bytesEqual(rest[length+2:], endSeq) {
		return 0, nil, fmt.Errorf("invalid end sequence %x", rest[length+2:])
	}
	if expected := checksum(payload); sum != expected {
		return 0, nil, fmt.Errorf("checksum mismatch: expected %d, got %d", expected, sum)
	}
	return payload[0], payload[1:], nil
}

// readFull reads exactly n bytes, or fails when the deadline is exceeded.
func (c *Conn) readFull(n int, deadline time.Time) ([]byte, error) {
	for len(c.pending) < n {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout while reading; last bytes were '%s'", hexdump(c.pending))
		}
		k, err := c.rw.Read(c.buf)
		c.pending = append(c.pending, c.buf[:k]...)
		if err != nil && (!errors.Is(err, io.EOF) || k == 0) {
			return nil, err
		}
	}
	out := make([]byte, n)
	copy(out, c.pending)
	c.pending = c.pending[n:]
	return out, nil
}

func bytesEqual(a, b []byte) bool {
	return string(a) == string(b)
}

// hexdump formats data as hex bytes in groups of four.
// Long data is shortened to its first and last 6 bytes.
func hexdump(data []byte) string {
	if len(data) > 60 {
		return fmt.Sprintf("%s[%d more bytes] %s", hexdump(data[:6]), len(data)-12, hexdump(data[len(data)-6:]))
	}
	var sb strings.Builder
	for i, b := range data {
		sb.WriteString(hex.EncodeToString([]byte{b}))
		sb.WriteByte(' ')
		if i%4 == 3 {
			sb.WriteByte(' ')
		}
	}
	return sb.String()
}
