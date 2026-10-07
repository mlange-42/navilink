package gpx

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mlange-42/navilink/internal/navilink"
)

func TestWriteTrack(t *testing.T) {
	g := FromTrackpoints([]navilink.Trackpoint{{
		Lat:      52.5,
		Lon:      -13.25,
		Altitude: 30.480000000000004,
		Time:     time.Date(2009, 6, 15, 12, 30, 45, 0, time.UTC),
		Speed:    36,
	}})
	var buf bytes.Buffer
	if err := g.Write(&buf); err != nil {
		t.Fatal(err)
	}
	want := `<?xml version="1.0" encoding="UTF-8"?>
<gpx xmlns="http://www.topografix.com/GPX/1/0" version="1.0" creator="navilink">
  <trk>
    <trkseg>
      <trkpt lat="52.5" lon="-13.25">
        <ele>30.48</ele>
        <time>2009-06-15T12:30:45Z</time>
        <speed>10</speed>
      </trkpt>
    </trkseg>
  </trk>
</gpx>
`
	if buf.String() != want {
		t.Errorf("got\n%s\nwant\n%s", buf.String(), want)
	}
}

func TestWriteWaypoints(t *testing.T) {
	g := FromWaypoints([]navilink.Waypoint{{
		Name:   "HOME",
		Lat:    1,
		Lon:    2,
		Time:   time.Date(2009, 6, 15, 12, 30, 45, 0, time.UTC),
		Symbol: 2,
	}})
	var buf bytes.Buffer
	if err := g.Write(&buf); err != nil {
		t.Fatal(err)
	}
	want := `<?xml version="1.0" encoding="UTF-8"?>
<gpx xmlns="http://www.topografix.com/GPX/1/0" version="1.0" creator="navilink">
  <wpt lat="1" lon="2">
    <ele>0</ele>
    <time>2009-06-15T12:30:45Z</time>
    <name>HOME</name>
    <sym>Residence</sym>
  </wpt>
</gpx>
`
	if buf.String() != want {
		t.Errorf("got\n%s\nwant\n%s", buf.String(), want)
	}
}

func TestReadWaypoints(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" xmlns="http://www.topografix.com/GPX/1/1">
  <wpt lat="52.5" lon="-13.25">
    <ele>30.48</ele>
    <time>2009-06-15T14:30:45+02:00</time>
    <name>My Home!</name>
    <sym>Residence</sym>
  </wpt>
  <wpt lat="1" lon="2">
    <time>2009-06-15T12:30:45</time>
    <name>GC1A2B3C</name>
    <sym>Geocache</sym>
  </wpt>
  <wpt lat="3" lon="4"><name>!!!</name></wpt>
</gpx>`
	g, err := Read(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Waypoints) != 3 {
		t.Fatalf("got %d waypoints", len(g.Waypoints))
	}

	wp, err := g.Waypoints[0].ToWaypoint()
	if err != nil {
		t.Fatal(err)
	}
	want := navilink.Waypoint{Name: "MYHOME", Lat: 52.5, Lon: -13.25, Altitude: 30.48,
		Time: time.Date(2009, 6, 15, 12, 30, 45, 0, time.UTC), Symbol: 2}
	if wp != want {
		t.Errorf("got %+v, want %+v", wp, want)
	}

	wp, err = g.Waypoints[1].ToWaypoint()
	if err != nil {
		t.Fatal(err)
	}
	if wp.Name != "1A2B3C" || wp.Symbol != 1 || wp.Time.Hour() != 12 {
		t.Errorf("geocache: got %+v", wp)
	}

	if _, err = g.Waypoints[2].ToWaypoint(); err == nil {
		t.Error("expected error for invalid name")
	}
}

func TestReadWrittenGPX(t *testing.T) {
	var buf bytes.Buffer
	in := navilink.Waypoint{Name: "HOME", Lat: 1, Lon: 2, Altitude: 3,
		Time: time.Date(2009, 6, 15, 12, 30, 45, 0, time.UTC), Symbol: 9}
	if err := FromWaypoints([]navilink.Waypoint{in}).Write(&buf); err != nil {
		t.Fatal(err)
	}
	g, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	out, err := g.Waypoints[0].ToWaypoint()
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("got %+v, want %+v", out, in)
	}
}

func TestWaypointName(t *testing.T) {
	tests := []struct{ name, sym, want string }{
		{"abc def-1", "", "ABCDEF"},
		{"GC12345", "Flag", "GC1234"},
		{"GC12345", "geocache", "12345"},
		{"Zürich", "", "ZRICH"},
	}
	for _, tt := range tests {
		if got := WaypointName(tt.name, tt.sym); got != tt.want {
			t.Errorf("WaypointName(%q, %q) = %q, want %q", tt.name, tt.sym, got, tt.want)
		}
	}
}

// TestDeviceTestdata checks the conversions documented in TESTING.md.
func TestDeviceTestdata(t *testing.T) {
	read := func(file string) []navilink.Waypoint {
		f, err := os.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		g, err := Read(f)
		if err != nil {
			t.Fatal(err)
		}
		var out []navilink.Waypoint
		for i := range g.Waypoints {
			wp, err := g.Waypoints[i].ToWaypoint()
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, wp)
		}
		return out
	}

	points := read("../../testdata/device/waypoints.gpx")
	want := []struct {
		name   string
		symbol string
		time   string // empty: current time
	}{
		{"ZZT1", "Flag", "2026-10-07T12:00:00Z"},
		{"ZZT2BR", "Bridge", "2026-10-07T12:30:00Z"},
		{"ZZT3", "Flag", ""},
		{"ZZT4SO", "Summit", "2026-01-01T00:00:00Z"},
		{"ZZT5", "Waypoint", "2026-12-31T23:59:59Z"},
	}
	if len(points) != len(want) {
		t.Fatalf("got %d waypoints, want %d", len(points), len(want))
	}
	for i, w := range want {
		p := points[i]
		if p.Name != w.name || navilink.SymbolName(p.Symbol) != w.symbol {
			t.Errorf("waypoint %d: got %s/%s, want %s/%s", i, p.Name, navilink.SymbolName(p.Symbol), w.name, w.symbol)
		}
		if w.time != "" && formatTime(p.Time) != w.time {
			t.Errorf("waypoint %s: got time %s, want %s", p.Name, formatTime(p.Time), w.time)
		}
	}

	var names []string
	for _, p := range read("../../testdata/device/waypoints-remove.gpx") {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "ZZT1,ZZT2BR,ZZT9" {
		t.Errorf("remove names: got %v", names)
	}
}
