package main

import (
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/mlange-42/navilink/internal/gpx"
	"github.com/mlange-42/navilink/internal/navilink"
)

// gpxRoute is a route read from GPX, converted for the device.
type gpxRoute struct {
	name   string
	points []navilink.Waypoint
}

// readRoutes reads the input GPX and converts its routes for the device.
// Invalid routes are reported and skipped.
func (in *input) readRoutes() ([]gpxRoute, error) {
	g, err := in.readGPX()
	if err != nil {
		return nil, err
	}
	routes := make([]gpxRoute, 0, len(g.Routes))
	for i := range g.Routes {
		name, points, err := g.Routes[i].ToRoute(i + 1)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Skipping route: %v\n", err)
			continue
		}
		routes = append(routes, gpxRoute{name: name, points: points})
	}
	if len(routes) == 0 {
		return nil, errors.New("no routes found in GPX input")
	}
	return routes, nil
}

// readRouteNames reads the input GPX and returns the device names of its routes.
func (in *input) readRouteNames() ([]string, error) {
	g, err := in.readGPX()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(g.Routes))
	for i, r := range g.Routes {
		name := gpx.RouteName(r.Name)
		if name == "" {
			name = fmt.Sprintf("ROUTE %d", i+1)
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, errors.New("no routes found in GPX input")
	}
	return names, nil
}

// RoutesGetCmd downloads routes.
type RoutesGetCmd struct {
	output
}

func (c *RoutesGetCmd) Run(g *Globals) (err error) {
	s, err := g.open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Close()) }()

	routes, err := s.Routes()
	if err != nil {
		return err
	}
	if len(routes) == 0 {
		fmt.Fprintln(os.Stderr, "There are no routes on the device.")
	}
	waypoints, err := s.Waypoints()
	if err != nil {
		return err
	}
	out, err := gpx.FromRoutes(routes, waypoints)
	if err != nil {
		return err
	}
	return c.writeGPX(out)
}

// RoutesPutCmd uploads routes.
type RoutesPutCmd struct {
	input
	SkipExisting bool `short:"s" help:"Skip routes with names that already exist on the device."`
}

func (c *RoutesPutCmd) Run(g *Globals) (err error) {
	routes, err := c.readRoutes()
	if err != nil {
		return err
	}

	s, err := g.open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Close()) }()

	existing := map[string]bool{}
	if c.SkipExisting {
		onDevice, err := s.Routes()
		if err != nil {
			return err
		}
		for _, r := range onDevice {
			existing[r.Name] = true
		}
	}

	wps, err := newWaypointIndex(s)
	if err != nil {
		return err
	}

	bar := s.newBar("Uploading routes")
	bar.Update(0, len(routes))
	wps.logf = bar.Logf
	uploaded, failed := 0, 0
	for _, r := range routes {
		bar.Add(1)
		if existing[r.name] {
			bar.Logf("Skipped existing route %s\n", r.name)
			continue
		}
		ids, err := wps.ensure(r)
		if err == nil {
			err = s.AddRoute(navilink.Route{Name: r.name, WaypointIDs: ids})
		}
		if err != nil {
			bar.Logf("Upload of route %s failed: %v\n", r.name, err)
			failed++
			continue
		}
		if c.SkipExisting {
			existing[r.name] = true
		}
		uploaded++
	}
	bar.Finish()
	fmt.Fprintf(os.Stderr, "Uploaded %d routes and %d new waypoints.\n", uploaded, wps.added)
	if failed > 0 {
		return fmt.Errorf("upload of %d routes failed", failed)
	}
	return nil
}

// waypointIndex finds device waypoints by name, uploading missing ones.
type waypointIndex struct {
	s      *session
	byName map[string]navilink.Waypoint
	added  int
	logf   func(format string, args ...any)
}

func newWaypointIndex(s *session) (*waypointIndex, error) {
	idx := &waypointIndex{
		s:    s,
		logf: func(format string, args ...any) { fmt.Fprintf(os.Stderr, format, args...) },
	}
	return idx, idx.refresh()
}

func (idx *waypointIndex) refresh() error {
	wps, err := idx.s.Waypoints()
	if err != nil {
		return err
	}
	idx.byName = make(map[string]navilink.Waypoint, len(wps))
	for _, wp := range wps {
		if _, ok := idx.byName[wp.Name]; !ok {
			idx.byName[wp.Name] = wp
		}
	}
	return nil
}

// ensure uploads the route's waypoints that are not on the device yet,
// and returns the IDs of all of the route's waypoints.
func (idx *waypointIndex) ensure(r gpxRoute) ([]uint16, error) {
	pending := map[string]bool{}
	for _, p := range r.points {
		if wp, ok := idx.byName[p.Name]; ok {
			if math.Abs(wp.Lat-p.Lat) > 1e-4 || math.Abs(wp.Lon-p.Lon) > 1e-4 {
				idx.logf("Route %s: using existing waypoint %s at %.5f, %.5f instead of %.5f, %.5f\n",
					r.name, p.Name, wp.Lat, wp.Lon, p.Lat, p.Lon)
			}
			continue
		}
		if pending[p.Name] {
			continue
		}
		if err := idx.s.AddWaypoint(p); err != nil {
			return nil, err
		}
		pending[p.Name] = true
		idx.added++
	}

	// Re-read waypoints to learn the IDs assigned to the new ones.
	if len(pending) > 0 {
		if err := idx.refresh(); err != nil {
			return nil, err
		}
	}

	ids := make([]uint16, len(r.points))
	for i, p := range r.points {
		wp, ok := idx.byName[p.Name]
		if !ok {
			return nil, fmt.Errorf("waypoint %s not found on the device after upload", p.Name)
		}
		ids[i] = wp.ID
	}
	return ids, nil
}

// RoutesRemoveCmd removes the given routes.
type RoutesRemoveCmd struct {
	input
	confirmation
}

func (c *RoutesRemoveCmd) Run(g *Globals) (err error) {
	names, err := c.readRouteNames()
	if err != nil {
		return err
	}

	s, err := g.open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Close()) }()

	onDevice, err := s.Routes()
	if err != nil {
		return err
	}
	ids := map[string][]uint8{}
	for _, r := range onDevice {
		ids[r.Name] = append(ids[r.Name], r.ID)
	}

	var toDelete []uint8
	var deleteNames []string
	for _, name := range names {
		found, ok := ids[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "Route %s not found on the device\n", name)
			continue
		}
		delete(ids, name) // don't delete twice if the input contains duplicates
		for _, id := range found {
			toDelete = append(toDelete, id)
			deleteNames = append(deleteNames, name)
		}
	}
	if len(toDelete) == 0 {
		return errors.New("none of the routes were found on the device")
	}

	if err := c.confirm(fmt.Sprintf("%d routes will be deleted from the device!", len(toDelete))); err != nil {
		return err
	}

	deleted, failed := 0, 0
	for i, id := range toDelete {
		if err := s.DeleteRoute(id); err != nil {
			fmt.Fprintf(os.Stderr, "Removal of route %s failed: %v\n", deleteNames[i], err)
			failed++
			continue
		}
		deleted++
	}
	fmt.Fprintf(os.Stderr, "Deleted %d routes.\n", deleted)
	if failed > 0 {
		return fmt.Errorf("removal of %d routes failed", failed)
	}
	return nil
}

// RoutesDeleteAllCmd deletes all routes.
type RoutesDeleteAllCmd struct {
	confirmation
}

func (c *RoutesDeleteAllCmd) Run(g *Globals) (err error) {
	s, err := g.open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Close()) }()

	info, err := s.Info()
	if err != nil {
		return err
	}
	if info.Routes == 0 {
		fmt.Fprintln(os.Stderr, "There are no routes on the device.")
		return nil
	}
	if err := c.confirm(fmt.Sprintf("All %d routes will be deleted from the device!", info.Routes)); err != nil {
		return err
	}
	return s.DeleteAllRoutes()
}
