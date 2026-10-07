package navilink

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"
)

func TestEncodeRoute(t *testing.T) {
	msg, err := encodeRoute(Route{ID: 5, Name: "MY ROUTE", WaypointIDs: []uint16{3, 1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(msg) != 2*recordSize {
		t.Fatalf("got %d bytes, want %d", len(msg), 2*recordSize)
	}
	header := []byte{0x00, 0x20, 0x00, 0x20, 'M', 'Y', ' ', 'R', 'O', 'U', 'T', 'E', 0, 0, 0, 0, 0, 0}
	header = append(header, make([]byte, 12)...)
	header = append(header, 0x7b, 0x77)
	if !bytes.Equal(msg[:recordSize], header) {
		t.Errorf("header:\ngot  % x\nwant % x", msg[:recordSize], header)
	}
	sub := []byte{0x10, 0x20, 3, 0, 1, 0, 2, 0}
	sub = append(sub, bytes.Repeat([]byte{0xff}, 22)...)
	sub = append(sub, 0x7f, 0x77)
	if !bytes.Equal(msg[recordSize:], sub) {
		t.Errorf("subroute:\ngot  % x\nwant % x", msg[recordSize:], sub)
	}
}

func TestEncodeRouteSubroutes(t *testing.T) {
	tests := []struct{ points, subroutes int }{
		{1, 1}, {13, 1}, {14, 2}, {27, 2}, {28, 3}, {MaxRoutePoints, 9},
	}
	for _, tt := range tests {
		ids := make([]uint16, tt.points)
		for i := range ids {
			ids[i] = uint16(i)
		}
		msg, err := encodeRoute(Route{Name: "R", WaypointIDs: ids})
		if err != nil {
			t.Fatal(err)
		}
		if got := len(msg)/recordSize - 1; got != tt.subroutes {
			t.Errorf("%d points: got %d subroutes, want %d", tt.points, got, tt.subroutes)
		}
		for s := range tt.subroutes {
			tag := msg[recordSize*(s+2)-2]
			if last := s == tt.subroutes-1; last != (tag == lastSubroute) {
				t.Errorf("%d points: subroute %d has tag 0x%02x", tt.points, s, tag)
			}
		}

		r, err := decodeRoute(msg)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(r.WaypointIDs, ids) {
			t.Errorf("%d points: round trip got %v", tt.points, r.WaypointIDs)
		}
	}
}

func TestEncodeRouteInvalid(t *testing.T) {
	for _, r := range []Route{
		{Name: "EMPTY"},
		{Name: "LONG", WaypointIDs: make([]uint16, MaxRoutePoints+1)},
		{Name: "NAME TOO LONG1", WaypointIDs: []uint16{1}},
	} {
		if _, err := encodeRoute(r); err == nil {
			t.Errorf("expected error for route %s with %d points", r.Name, len(r.WaypointIDs))
		}
	}
}

func TestDecodeRouteFull(t *testing.T) {
	// A route of 14 waypoints with all 9 subroutes sent, as the device may do.
	msg, err := encodeRoute(Route{Name: "FULL", WaypointIDs: make([]uint16, 14)})
	if err != nil {
		t.Fatal(err)
	}
	for len(msg) < 10*recordSize {
		msg = append(msg, make([]byte, recordSize)...)
	}
	msg[2] = 7
	r, err := decodeRoute(msg)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != 7 || r.Name != "FULL" || len(r.WaypointIDs) != 14 {
		t.Errorf("got %+v", r)
	}
}

func TestRoutes(t *testing.T) {
	info := make([]byte, 32)
	info[2] = 2 // routes
	r1, _ := encodeRoute(Route{Name: "A", WaypointIDs: []uint16{1}})
	r2, _ := encodeRoute(Route{Name: "B", WaypointIDs: []uint16{2, 3}})

	dev := &fakeDevice{}
	dev.answer(t, PidData, info)
	dev.answer(t, PidData, r1)
	dev.answer(t, PidData, r2)
	routes, err := newTestClient(dev).Routes()
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 || routes[1].Name != "B" || !slices.Equal(routes[1].WaypointIDs, []uint16{2, 3}) {
		t.Errorf("got %+v", routes)
	}

	types, data := sentPackets(t, dev)
	if types[2] != PidQryRoute || !bytes.Equal(data[2], []byte{1, 0, 0, 0, 0, 0, 1}) {
		t.Errorf("second query: type 0x%02x data % x", types[2], data[2])
	}
}

func TestDeleteRoute(t *testing.T) {
	dev := &fakeDevice{}
	dev.answer(t, PidAck, nil)
	dev.answer(t, PidAck, nil)
	c := newTestClient(dev)
	if err := c.DeleteRoute(3); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteAllRoutes(); err != nil {
		t.Fatal(err)
	}
	types, data := sentPackets(t, dev)
	if types[0] != PidDelRoute || binary.LittleEndian.Uint32(data[0]) != 3<<16 {
		t.Errorf("delete: type 0x%02x data % x", types[0], data[0])
	}
	if types[1] != PidDelAllRoute || !bytes.Equal(data[1], []byte{0x00, 0xf0, 0x00, 0x00}) {
		t.Errorf("delete all: type 0x%02x data % x", types[1], data[1])
	}
}
