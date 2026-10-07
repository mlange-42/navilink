package navilink

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
	"time"
)

// fakeDevice records written packets and answers with a fixed sequence of packets.
type fakeDevice struct {
	written bytes.Buffer
	answers bytes.Buffer
}

func (d *fakeDevice) Write(p []byte) (int, error) { return d.written.Write(p) }
func (d *fakeDevice) Read(p []byte) (int, error) {
	n, err := d.answers.Read(p)
	if err == io.EOF {
		return 0, nil // behave like a serial port timeout
	}
	return n, err
}

func (d *fakeDevice) answer(t *testing.T, typ byte, data []byte) {
	t.Helper()
	p, err := encodePacket(typ, data)
	if err != nil {
		t.Fatal(err)
	}
	d.answers.Write(p)
}

func newTestClient(dev *fakeDevice) *Client {
	conn := NewConn(dev)
	conn.Timeout = 100 * time.Millisecond
	return NewClient(conn)
}

func TestEncodePacket(t *testing.T) {
	p, err := encodePacket(PidSync, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0xa0, 0xa2, 0x01, 0x00, 0xd6, 0xd6, 0x00, 0xb0, 0xb3}
	if !bytes.Equal(p, want) {
		t.Errorf("got % x, want % x", p, want)
	}
}

func TestChecksumMask(t *testing.T) {
	payload := bytes.Repeat([]byte{0xff}, 200) // 51000 > 0x7fff
	if got, want := checksum(payload), uint16(51000&0x7fff); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

func TestReceiveSkipsGarbage(t *testing.T) {
	dev := &fakeDevice{}
	dev.answers.Write([]byte{0x12, 0xa0, 0x34})
	dev.answer(t, PidData, []byte{1, 2, 3})

	typ, data, err := NewConn(dev).Receive()
	if err != nil {
		t.Fatal(err)
	}
	if typ != PidData || !bytes.Equal(data, []byte{1, 2, 3}) {
		t.Errorf("got type 0x%02x data % x", typ, data)
	}
}

func TestReceiveBadChecksum(t *testing.T) {
	dev := &fakeDevice{}
	p, _ := encodePacket(PidData, []byte{1, 2, 3})
	p[len(p)-4]++
	dev.answers.Write(p)

	if _, _, err := NewConn(dev).Receive(); err == nil {
		t.Error("expected checksum error")
	}
}

func TestReceiveTimeout(t *testing.T) {
	conn := NewConn(&fakeDevice{})
	conn.Timeout = 50 * time.Millisecond
	if _, _, err := conn.Receive(); err == nil {
		t.Error("expected timeout error")
	}
}

func TestInfo(t *testing.T) {
	data := make([]byte, 32)
	le := binary.LittleEndian
	le.PutUint16(data[0:], 12)
	data[2] = 3
	data[3] = 1
	le.PutUint32(data[4:], 0x1000)
	le.PutUint32(data[8:], 123456)
	le.PutUint16(data[12:], 4097)
	le.PutUint16(data[14:], 2)
	copy(data[16:], "John Doe")

	dev := &fakeDevice{}
	dev.answer(t, PidData, data)
	info, err := newTestClient(dev).Info()
	if err != nil {
		t.Fatal(err)
	}
	want := Info{Waypoints: 12, Routes: 3, Tracks: 1, TrackBuffer: 0x1000,
		Serial: 123456, Trackpoints: 4097, Protocol: 2, Username: "John Doe"}
	if info != want {
		t.Errorf("got %+v, want %+v", info, want)
	}
}

func TestFirmwareVersion(t *testing.T) {
	dev := &fakeDevice{}
	dev.answer(t, PidData, []byte("1.23,foo\x00"))
	v, err := newTestClient(dev).FirmwareVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v != "1.23" {
		t.Errorf("got %q", v)
	}
}

func trackRecord(lat, lon int32, altFt uint16, speed byte) []byte {
	rec := make([]byte, recordSize)
	le := binary.LittleEndian
	le.PutUint32(rec[12:], uint32(lat))
	le.PutUint32(rec[16:], uint32(lon))
	le.PutUint16(rec[20:], altFt)
	copy(rec[22:], []byte{9, 6, 15, 12, 30, 45})
	rec[29] = speed
	return rec
}

func TestDecodeTrackpoint(t *testing.T) {
	p := decodeTrackpoint(trackRecord(-335000000, 1512345678, 100, 10))
	want := Trackpoint{
		Lat:      -33.5,
		Lon:      151.2345678,
		Altitude: 30.48,
		Time:     time.Date(2009, 6, 15, 12, 30, 45, 0, time.UTC),
		Speed:    20,
	}
	if p != want {
		t.Errorf("got %+v, want %+v", p, want)
	}
}

func TestTrackpointsChunked(t *testing.T) {
	const n = 600 // more than one chunk of 512
	info := make([]byte, 32)
	binary.LittleEndian.PutUint32(info[4:], 0x2000)
	binary.LittleEndian.PutUint16(info[12:], n)

	dev := &fakeDevice{}
	dev.answer(t, PidData, info)
	dev.answer(t, PidData, bytes.Repeat(trackRecord(1, 2, 3, 4), 512))
	dev.answer(t, PidData, bytes.Repeat(trackRecord(1, 2, 3, 4), n-512))

	points, err := newTestClient(dev).Trackpoints()
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != n {
		t.Errorf("got %d points, want %d", len(points), n)
	}

	// Check the second read request: address and size.
	conn := NewConn(bytes.NewBuffer(dev.written.Bytes()))
	conn.Timeout = 100 * time.Millisecond
	var reqs [][]byte
	for {
		typ, data, err := conn.Receive()
		if err != nil {
			break
		}
		if typ == PidReadTrackpoints {
			reqs = append(reqs, data)
		}
	}
	if len(reqs) != 2 {
		t.Fatalf("got %d read requests, want 2", len(reqs))
	}
	if addr := binary.LittleEndian.Uint32(reqs[1]); addr != 0x2000+512*32 {
		t.Errorf("second request address 0x%x", addr)
	}
	if size := binary.LittleEndian.Uint16(reqs[1][4:]); size != (n-512)*32 {
		t.Errorf("second request size %d", size)
	}
}

func TestWaypoints(t *testing.T) {
	info := make([]byte, 32)
	binary.LittleEndian.PutUint16(info[0:], 1)

	rec := trackRecord(525000000, 133000000, 0, 0)
	binary.LittleEndian.PutUint16(rec[2:], 7)
	copy(rec[4:], "HOME\x00")
	rec[28] = 2

	dev := &fakeDevice{}
	dev.answer(t, PidData, info)
	dev.answer(t, PidData, rec)

	points, err := newTestClient(dev).Waypoints()
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 {
		t.Fatalf("got %d points", len(points))
	}
	p := points[0]
	if p.ID != 7 || p.Name != "HOME" || p.Lat != 52.5 || p.Lon != 13.3 || SymbolName(p.Symbol) != "Residence" {
		t.Errorf("got %+v", p)
	}
}

func TestSymbols(t *testing.T) {
	if got := SymbolID("parking area"); got != 44 {
		t.Errorf("last symbol: got %d, want 44", got)
	}
	if got := SymbolID("Geocache"); got != 1 {
		t.Errorf("geocache: got %d, want 1", got)
	}
	if got := SymbolID("unknown"); got != 0 {
		t.Errorf("unknown: got %d, want 0", got)
	}
	if got := SymbolName(200); got != "Waypoint" {
		t.Errorf("out of range: got %q", got)
	}
}
