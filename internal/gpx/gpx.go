// Package gpx reads and writes GPX 1.0 files.
package gpx

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/mlange-42/navilink/internal/navilink"
)

const (
	namespace = "http://www.topografix.com/GPX/1/0"
	creator   = "navilink"
)

// GPX is the root element of a GPX file.
type GPX struct {
	XMLName   xml.Name `xml:"gpx"`
	Xmlns     string   `xml:"xmlns,attr,omitempty"`
	Version   string   `xml:"version,attr"`
	Creator   string   `xml:"creator,attr"`
	Waypoints []Point  `xml:"wpt"`
	Routes    []Route  `xml:"rte"`
	Tracks    []Track  `xml:"trk"`
}

// Route is a GPX route.
type Route struct {
	Name   string  `xml:"name,omitempty"`
	Points []Point `xml:"rtept"`
}

// Track is a GPX track.
type Track struct {
	Segments []Segment `xml:"trkseg"`
}

// Segment is a GPX track segment.
type Segment struct {
	Points []Point `xml:"trkpt"`
}

// Point is a GPX waypoint or track point.
// Elements are in the order required by the GPX 1.0 schema.
type Point struct {
	Lat       float64  `xml:"lat,attr"`
	Lon       float64  `xml:"lon,attr"`
	Elevation *float64 `xml:"ele,omitempty"`
	Time      string   `xml:"time,omitempty"`
	Speed     *float64 `xml:"speed,omitempty"` // m/s
	Name      string   `xml:"name,omitempty"`
	Symbol    string   `xml:"sym,omitempty"`
}

func newGPX() *GPX {
	return &GPX{Xmlns: namespace, Version: "1.0", Creator: creator}
}

// FromTrackpoints creates a GPX with a single track from the given points.
func FromTrackpoints(points []navilink.Trackpoint) *GPX {
	seg := Segment{Points: make([]Point, len(points))}
	for i, p := range points {
		seg.Points[i] = Point{
			Lat:       p.Lat,
			Lon:       p.Lon,
			Elevation: ptr(round(p.Altitude, 2)),
			Time:      formatTime(p.Time),
			Speed:     ptr(round(p.Speed/3.6, 2)),
		}
	}
	g := newGPX()
	g.Tracks = []Track{{Segments: []Segment{seg}}}
	return g
}

// FromWaypoints creates a GPX from the given waypoints.
func FromWaypoints(points []navilink.Waypoint) *GPX {
	g := newGPX()
	g.Waypoints = make([]Point, len(points))
	for i, p := range points {
		g.Waypoints[i] = Point{
			Lat:       p.Lat,
			Lon:       p.Lon,
			Elevation: ptr(round(p.Altitude, 2)),
			Time:      formatTime(p.Time),
			Name:      p.Name,
			Symbol:    navilink.SymbolName(p.Symbol),
		}
	}
	return g
}

// FromRoutes creates a GPX from the given routes.
// The waypoints must contain all waypoints referenced by the routes.
func FromRoutes(routes []navilink.Route, waypoints []navilink.Waypoint) (*GPX, error) {
	byID := make(map[uint16]navilink.Waypoint, len(waypoints))
	for _, wp := range waypoints {
		byID[wp.ID] = wp
	}

	g := newGPX()
	g.Routes = make([]Route, len(routes))
	for i, r := range routes {
		points := make([]navilink.Waypoint, len(r.WaypointIDs))
		for j, id := range r.WaypointIDs {
			wp, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("route %s references unknown waypoint ID %d", r.Name, id)
			}
			points[j] = wp
		}
		g.Routes[i] = Route{Name: r.Name, Points: FromWaypoints(points).Waypoints}
	}
	return g, nil
}

// Write writes the GPX as indented XML.
func (g *GPX) Write(w io.Writer) error {
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(g); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func round(v float64, digits int) float64 {
	f := math.Pow10(digits)
	return math.Round(v*f) / f
}

func ptr[T any](v T) *T {
	return &v
}
