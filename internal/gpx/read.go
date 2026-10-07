package gpx

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/mlange-42/navilink/internal/navilink"
)

// MaxNameLength is the maximum length of waypoint names on the device.
const MaxNameLength = 6

var (
	invalidNameChars      = regexp.MustCompile(`[^A-Z0-9]+`)
	invalidRouteNameChars = regexp.MustCompile(`[^A-Z0-9 ]+`)
	multipleSpaces        = regexp.MustCompile(` +`)
)

// Read parses a GPX file. Any GPX version is accepted.
func Read(r io.Reader) (*GPX, error) {
	var g GPX
	if err := xml.NewDecoder(r).Decode(&g); err != nil {
		return nil, fmt.Errorf("parsing GPX: %w", err)
	}
	return &g, nil
}

// ToWaypoint converts a GPX point to a device waypoint.
//
// The name is converted to upper case, stripped of all characters except
// A-Z and 0-9 and shortened to MaxNameLength characters. Geocache names
// have their "GC" prefix removed. Points without time get the current time.
func (p *Point) ToWaypoint() (navilink.Waypoint, error) {
	name := WaypointName(p.Name, p.Symbol)
	if name == "" {
		return navilink.Waypoint{}, fmt.Errorf("waypoint at %f, %f has no valid name", p.Lat, p.Lon)
	}

	t := time.Now().UTC()
	if p.Time != "" {
		var err error
		if t, err = parseTime(p.Time); err != nil {
			return navilink.Waypoint{}, fmt.Errorf("waypoint %s: %w", name, err)
		}
	}

	wp := navilink.Waypoint{
		Name:   name,
		Lat:    p.Lat,
		Lon:    p.Lon,
		Time:   t,
		Symbol: navilink.SymbolID(p.Symbol),
	}
	if p.Elevation != nil {
		wp.Altitude = *p.Elevation
	}
	return wp, nil
}

// WaypointName converts a GPX waypoint name to a valid device waypoint name.
func WaypointName(name, symbol string) string {
	name = invalidNameChars.ReplaceAllString(strings.ToUpper(name), "")
	if strings.EqualFold(strings.TrimSpace(symbol), "Geocache") {
		name = strings.TrimPrefix(name, "GC")
	}
	if len(name) > MaxNameLength {
		name = name[:MaxNameLength]
	}
	return name
}

// parseTime parses a GPX time. Times without a time zone are assumed to be UTC.
func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q", s)
}

// RouteName converts a GPX route name to a valid device route name.
// Characters other than A-Z, 0-9 and space are removed,
// and the name is shortened to navilink.MaxRouteNameLength characters.
func RouteName(name string) string {
	name = invalidRouteNameChars.ReplaceAllString(strings.ToUpper(name), "")
	name = strings.TrimSpace(multipleSpaces.ReplaceAllString(name, " "))
	if len(name) > navilink.MaxRouteNameLength {
		name = strings.TrimSpace(name[:navilink.MaxRouteNameLength])
	}
	return name
}

// ToRoute converts a GPX route to a device route name and its waypoints.
//
// index is the 1-based position of the route in the GPX file. It is used for
// routes without a name ("ROUTE 1") and route points without a valid name
// ("R01001" for the first point of the first route).
func (r *Route) ToRoute(index int) (string, []navilink.Waypoint, error) {
	name := RouteName(r.Name)
	if name == "" {
		name = fmt.Sprintf("ROUTE %d", index)
	}
	if len(r.Points) == 0 {
		return "", nil, fmt.Errorf("route %s has no points", name)
	}
	if len(r.Points) > navilink.MaxRoutePoints {
		return "", nil, fmt.Errorf("route %s has %d points, the maximum is %d", name, len(r.Points), navilink.MaxRoutePoints)
	}

	points := make([]navilink.Waypoint, len(r.Points))
	for i := range r.Points {
		p := r.Points[i]
		if WaypointName(p.Name, p.Symbol) == "" {
			p.Name = fmt.Sprintf("R%02d%03d", index%100, i+1)
		}
		wp, err := p.ToWaypoint()
		if err != nil {
			return "", nil, fmt.Errorf("route %s: %w", name, err)
		}
		points[i] = wp
	}
	return name, points, nil
}
