package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlange-42/navilink/internal/gpx"
	"github.com/mlange-42/navilink/internal/navilink/navilinktest"
)

// runWith runs the CLI with the given arguments against a simulated device.
func runWith(t *testing.T, dev *navilinktest.Device, args ...string) error {
	t.Helper()
	orig := openPort
	openPort = func(string) (io.ReadWriteCloser, error) { return dev, nil }
	defer func() { openPort = orig }()

	var cli CLI
	parser := newParser(&cli)
	ctx, err := parser.Parse(append([]string{"-d", "sim"}, args...))
	if err != nil {
		t.Fatal(err)
	}
	return ctx.Run()
}

func readGPXFile(t *testing.T, path string) *gpx.GPX {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	g, err := gpx.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

const (
	waypointsFile       = "testdata/device/waypoints.gpx"
	waypointsRemoveFile = "testdata/device/waypoints-remove.gpx"
	routesFile          = "testdata/device/routes.gpx"
	routesWaypointsFile = "testdata/device/routes-waypoints.gpx"
)

func TestWaypointsPutGetRemove(t *testing.T) {
	dev := navilinktest.New()
	if err := runWith(t, dev, "wp", "put", "-i", waypointsFile); err != nil {
		t.Fatal(err)
	}
	want := []string{"ZZT1", "ZZT2BR", "ZZT3", "ZZT4SO", "ZZT5"}
	if got := dev.WaypointNames(); !slices.Equal(got, want) {
		t.Fatalf("after put: got %v, want %v", got, want)
	}

	if err := runWith(t, dev, "wp", "put", "-s", "-i", waypointsFile); err != nil {
		t.Fatal(err)
	}
	if got := dev.WaypointNames(); len(got) != 5 {
		t.Errorf("put --skip-existing uploaded duplicates: %v", got)
	}

	out := filepath.Join(t.TempDir(), "out.gpx")
	if err := runWith(t, dev, "wp", "get", "-o", out); err != nil {
		t.Fatal(err)
	}
	g := readGPXFile(t, out)
	if len(g.Waypoints) != 5 {
		t.Fatalf("get: got %d waypoints", len(g.Waypoints))
	}
	p := g.Waypoints[3]
	if p.Name != "ZZT4SO" || p.Lat != -33.4489 || p.Lon != -70.6693 || *p.Elevation != 519.99 ||
		p.Time != "2026-01-01T00:00:00Z" || p.Symbol != "Summit" {
		t.Errorf("get: got %+v", p)
	}

	if err := runWith(t, dev, "wp", "remove", "-y", "-i", waypointsRemoveFile); err != nil {
		t.Fatal(err)
	}
	want = []string{"ZZT3", "ZZT4SO", "ZZT5"}
	if got := dev.WaypointNames(); !slices.Equal(got, want) {
		t.Errorf("after remove: got %v, want %v", got, want)
	}

	if err := runWith(t, dev, "wp", "delete-all", "-y"); err != nil {
		t.Fatal(err)
	}
	if got := dev.WaypointNames(); len(got) != 0 {
		t.Errorf("after delete-all: got %v", got)
	}
}

func TestRoutesPutGet(t *testing.T) {
	dev := navilinktest.New()
	dev.AddWaypoint("ZZR1C", 51.3512, 12.3899) // existing waypoint is reused

	if err := runWith(t, dev, "rt", "put", "-i", routesFile); err != nil {
		t.Fatal(err)
	}
	wantRoutes := map[string][]string{
		"ZZ ROUTE ONE": {"ZZR1A", "ZZR1B", "ZZR1C"},
		"ZZ ROUTE 2":   {"R02001", "R02002", "ZZR1A"},
	}
	got := dev.Routes()
	if len(got) != len(wantRoutes) {
		t.Fatalf("got routes %v", got)
	}
	for name, points := range wantRoutes {
		if !slices.Equal(got[name], points) {
			t.Errorf("route %q: got %v, want %v", name, got[name], points)
		}
	}
	wantWaypoints := []string{"ZZR1C", "ZZR1A", "ZZR1B", "R02001", "R02002"}
	if names := dev.WaypointNames(); !slices.Equal(names, wantWaypoints) {
		t.Errorf("waypoints: got %v, want %v", names, wantWaypoints)
	}

	if err := runWith(t, dev, "rt", "put", "-s", "-i", routesFile); err != nil {
		t.Fatal(err)
	}
	if got := dev.Routes(); len(got) != 2 {
		t.Errorf("put --skip-existing uploaded duplicates: %v", got)
	}

	out := filepath.Join(t.TempDir(), "out.gpx")
	if err := runWith(t, dev, "rt", "get", "-o", out); err != nil {
		t.Fatal(err)
	}
	g := readGPXFile(t, out)
	if len(g.Routes) != 2 {
		t.Fatalf("get: got %d routes", len(g.Routes))
	}
	// Routes are sorted by name on the device.
	r := g.Routes[0]
	if r.Name != "ZZ ROUTE 2" || len(r.Points) != 3 {
		t.Fatalf("get: got route %+v", r)
	}
	if p := r.Points[0]; p.Name != "R02001" || p.Lat != 51.36 || p.Lon != 12.4 {
		t.Errorf("get: got point %+v", p)
	}
	if p := g.Routes[1].Points[1]; p.Name != "ZZR1B" || p.Symbol != "Bridge" || *p.Elevation != 110.03 {
		t.Errorf("get: got point %+v", p)
	}
}

func TestRoutesRemove(t *testing.T) {
	dev := navilinktest.New()
	if err := runWith(t, dev, "rt", "put", "-i", routesFile); err != nil {
		t.Fatal(err)
	}

	// Waypoints used by routes can't be deleted.
	err := runWith(t, dev, "wp", "remove", "-y", "-i", routesWaypointsFile)
	if err == nil || !strings.Contains(err.Error(), "5 waypoints failed") {
		t.Errorf("expected removal of waypoints in routes to fail, got %v", err)
	}

	if err := runWith(t, dev, "rt", "remove", "-y", "-i", routesFile); err != nil {
		t.Fatal(err)
	}
	if got := dev.Routes(); len(got) != 0 {
		t.Errorf("after remove: got routes %v", got)
	}

	if err := runWith(t, dev, "wp", "remove", "-y", "-i", routesWaypointsFile); err != nil {
		t.Fatal(err)
	}
	if got := dev.WaypointNames(); len(got) != 0 {
		t.Errorf("after cleanup: got waypoints %v", got)
	}
}

func TestRoutesDeleteAll(t *testing.T) {
	dev := navilinktest.New()
	if err := runWith(t, dev, "rt", "put", "-i", routesFile); err != nil {
		t.Fatal(err)
	}
	// Waypoints can only be deleted after all routes.
	if err := runWith(t, dev, "wp", "delete-all", "-y"); err == nil {
		t.Error("expected error deleting all waypoints while there are routes")
	}

	if err := runWith(t, dev, "rt", "delete-all", "-y"); err != nil {
		t.Fatal(err)
	}
	if got := dev.Routes(); len(got) != 0 {
		t.Errorf("got routes %v", got)
	}
	if got := dev.WaypointNames(); len(got) != 5 {
		t.Errorf("waypoints should be kept, got %v", got)
	}

	if err := runWith(t, dev, "wp", "delete-all", "-y"); err != nil {
		t.Fatal(err)
	}
	if got := dev.WaypointNames(); len(got) != 0 {
		t.Errorf("got waypoints %v", got)
	}
}

func TestRoutesGetEmpty(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.gpx")
	if err := runWith(t, navilinktest.New(), "rt", "get", "-o", out); err != nil {
		t.Fatal(err)
	}
	if g := readGPXFile(t, out); len(g.Routes) != 0 {
		t.Errorf("got %d routes", len(g.Routes))
	}
}
