package gpx

import (
	"bytes"
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
