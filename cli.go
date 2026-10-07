package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mlange-42/navilink/internal/gpx"
	"github.com/mlange-42/navilink/internal/hint"
	"github.com/mlange-42/navilink/internal/navilink"
	"go.bug.st/serial"
)

// Globals are flags shared by all commands.
type Globals struct {
	Device  string `short:"d" env:"NAVILINK_DEVICE" help:"Serial port of the device (e.g. COM3 or /dev/ttyUSB0). Auto-detected if only one port is available."`
	Verbose bool   `short:"v" help:"Print packet hex dumps to stderr."`
	Quit    bool   `short:"q" help:"Quit NaviLink mode on the device after finishing."`
}

// session is an open connection to a device.
type session struct {
	*navilink.Client
	port serial.Port
	quit bool
}

// open opens the serial port and starts the communication with the device.
func (g *Globals) open() (*session, error) {
	device := g.Device
	if device == "" {
		var err error
		if device, err = detectPort(); err != nil {
			return nil, err
		}
	}

	port, err := serial.Open(device, &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		return nil, fmt.Errorf("can't open %s: %w%s", device, err, hint.OpenError(err))
	}
	if err := port.SetReadTimeout(100 * time.Millisecond); err != nil {
		return nil, errors.Join(err, port.Close())
	}

	conn := navilink.NewConn(port)
	if g.Verbose {
		conn.Debug = os.Stderr
	}
	client := navilink.NewClient(conn)
	if err := client.Sync(); err != nil {
		return nil, errors.Join(err, port.Close())
	}
	return &session{Client: client, port: port, quit: g.Quit}, nil
}

// Close quits NaviLink mode if requested and closes the port.
func (s *session) Close() error {
	var err error
	if s.quit {
		err = s.Quit()
	}
	return errors.Join(err, s.port.Close())
}

func detectPort() (string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return "", fmt.Errorf("listing serial ports: %w", err)
	}
	switch len(ports) {
	case 0:
		return "", errors.New("no serial ports found; is the device connected?")
	case 1:
		return ports[0], nil
	default:
		return "", fmt.Errorf("multiple serial ports found (%s); select one with --device", strings.Join(ports, ", "))
	}
}

// output is a flag for commands that write output.
type output struct {
	Output string `short:"o" type:"path" placeholder:"FILE" help:"Write output to this file instead of stdout."`
}

// create opens the output file, or returns stdout.
func (o *output) create() (io.WriteCloser, error) {
	if o.Output == "" || o.Output == "-" {
		return nopCloser{os.Stdout}, nil
	}
	return os.Create(o.Output)
}

// writeGPX writes the GPX to the output.
func (o *output) writeGPX(g *gpx.GPX) (err error) {
	w, err := o.create()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, w.Close()) }()
	return g.Write(w)
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// PortsCmd lists the available serial ports.
type PortsCmd struct{}

func (c *PortsCmd) Run() error {
	ports, err := serial.GetPortsList()
	if err != nil {
		return err
	}
	for _, p := range ports {
		fmt.Println(p)
	}
	return nil
}

// InfoCmd prints device information.
type InfoCmd struct {
	output
}

func (c *InfoCmd) Run(g *Globals) (err error) {
	s, err := g.open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Close()) }()

	info, err := s.Info()
	if err != nil {
		return err
	}
	fw, err := s.FirmwareVersion()
	if err != nil {
		return err
	}

	w, err := c.create()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, w.Close()) }()

	_, err = fmt.Fprintf(w, `username    : %s
firmware    : %s
serial      : %d
protocol    : %d
waypoints   : %d
routes      : %d
trackpoints : %d
`, info.Username, fw, info.Serial, info.Protocol, info.Waypoints, info.Routes, info.Trackpoints)
	return err
}

// TrackGetCmd downloads track data.
type TrackGetCmd struct {
	output
}

func (c *TrackGetCmd) Run(g *Globals) (err error) {
	s, err := g.open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Close()) }()

	points, err := s.Trackpoints()
	if err != nil {
		return err
	}
	if len(points) == 0 {
		fmt.Fprintln(os.Stderr, "There are no trackpoints on the device.")
	}
	return c.writeGPX(gpx.FromTrackpoints(points))
}

// WaypointsGetCmd downloads waypoints.
type WaypointsGetCmd struct {
	output
}

func (c *WaypointsGetCmd) Run(g *Globals) (err error) {
	s, err := g.open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Close()) }()

	points, err := s.Waypoints()
	if err != nil {
		return err
	}
	if len(points) == 0 {
		fmt.Fprintln(os.Stderr, "There are no waypoints on the device.")
	}
	return c.writeGPX(gpx.FromWaypoints(points))
}

// LogGetCmd downloads data from the internal data logger.
type LogGetCmd struct {
	output
}

func (c *LogGetCmd) Run(g *Globals) (err error) {
	s, err := g.open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Close()) }()

	points, err := s.LogPoints()
	if err != nil {
		return err
	}
	return c.writeGPX(gpx.FromTrackpoints(points))
}

// input is a flag for commands that read GPX input.
type input struct {
	Input string `short:"i" type:"path" placeholder:"FILE" help:"Read GPX from this file instead of stdin."`
}

// readWaypoints reads the input GPX and converts its waypoints for the device.
// Invalid waypoints are reported and skipped.
func (in *input) readWaypoints() ([]navilink.Waypoint, error) {
	r := io.Reader(os.Stdin)
	if in.Input != "" && in.Input != "-" {
		f, err := os.Open(in.Input)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	g, err := gpx.Read(r)
	if err != nil {
		return nil, err
	}

	points := make([]navilink.Waypoint, 0, len(g.Waypoints))
	for i := range g.Waypoints {
		wp, err := g.Waypoints[i].ToWaypoint()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Skipping waypoint: %v\n", err)
			continue
		}
		points = append(points, wp)
	}
	if len(points) == 0 {
		return nil, errors.New("no waypoints found in GPX input")
	}
	return points, nil
}

// confirmation is a flag for commands that delete data from the device.
type confirmation struct {
	Yes bool `short:"y" help:"Do not ask for confirmation."`
}

// confirm asks the user to confirm a destructive action.
func (c *confirmation) confirm(msg string) error {
	if c.Yes {
		return nil
	}
	fmt.Fprintf(os.Stderr, "%s Continue? [y/N] ", msg)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if errors.Is(err, io.EOF) && answer == "" {
		fmt.Fprintln(os.Stderr)
		return errors.New("no confirmation received; use --yes to skip it")
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	}
	return errors.New("aborted")
}

// TrackDeleteCmd deletes all track data.
type TrackDeleteCmd struct {
	confirmation
}

func (c *TrackDeleteCmd) Run(g *Globals) (err error) {
	s, err := g.open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Close()) }()

	info, err := s.Info()
	if err != nil {
		return err
	}
	if info.Trackpoints == 0 {
		fmt.Fprintln(os.Stderr, "There are no trackpoints on the device.")
		return nil
	}
	if err := c.confirm(fmt.Sprintf("All %d trackpoints will be deleted from the device!", info.Trackpoints)); err != nil {
		return err
	}
	return s.DeleteTrack()
}

// WaypointsPutCmd uploads waypoints.
type WaypointsPutCmd struct {
	input
	SkipExisting bool `short:"s" help:"Skip waypoints with names that already exist on the device."`
}

func (c *WaypointsPutCmd) Run(g *Globals) (err error) {
	points, err := c.readWaypoints()
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
		onDevice, err := s.Waypoints()
		if err != nil {
			return err
		}
		for _, wp := range onDevice {
			existing[wp.Name] = true
		}
	}

	uploaded, failed := 0, 0
	for _, wp := range points {
		if existing[wp.Name] {
			fmt.Fprintf(os.Stderr, "Skipped existing waypoint %s\n", wp.Name)
			continue
		}
		if err := s.AddWaypoint(wp); err != nil {
			fmt.Fprintf(os.Stderr, "Upload failed: %v\n", err)
			failed++
			continue
		}
		if c.SkipExisting {
			existing[wp.Name] = true
		}
		uploaded++
	}
	fmt.Fprintf(os.Stderr, "Uploaded %d waypoints.\n", uploaded)
	if failed > 0 {
		return fmt.Errorf("upload of %d waypoints failed", failed)
	}
	return nil
}

// WaypointsRemoveCmd removes the given waypoints.
type WaypointsRemoveCmd struct {
	input
	confirmation
}

func (c *WaypointsRemoveCmd) Run(g *Globals) (err error) {
	points, err := c.readWaypoints()
	if err != nil {
		return err
	}

	s, err := g.open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Close()) }()

	onDevice, err := s.Waypoints()
	if err != nil {
		return err
	}
	ids := make(map[string][]uint16, len(onDevice))
	for _, wp := range onDevice {
		ids[wp.Name] = append(ids[wp.Name], wp.ID)
	}

	var toDelete []uint16
	var names []string
	for _, wp := range points {
		found, ok := ids[wp.Name]
		if !ok {
			fmt.Fprintf(os.Stderr, "Waypoint %s not found on the device\n", wp.Name)
			continue
		}
		delete(ids, wp.Name) // don't delete twice if the input contains duplicates
		for _, id := range found {
			toDelete = append(toDelete, id)
			names = append(names, wp.Name)
		}
	}
	if len(toDelete) == 0 {
		return errors.New("none of the waypoints were found on the device")
	}

	if err := c.confirm(fmt.Sprintf("%d waypoints will be deleted from the device!", len(toDelete))); err != nil {
		return err
	}

	deleted, failed := 0, 0
	for i, id := range toDelete {
		if err := s.DeleteWaypoint(id); err != nil {
			fmt.Fprintf(os.Stderr, "Removal of waypoint %s failed, maybe it is in use? %v\n", names[i], err)
			failed++
			continue
		}
		deleted++
	}
	fmt.Fprintf(os.Stderr, "Deleted %d waypoints.\n", deleted)
	if failed > 0 {
		return fmt.Errorf("removal of %d waypoints failed", failed)
	}
	return nil
}

// WaypointsDeleteAllCmd deletes all waypoints.
type WaypointsDeleteAllCmd struct {
	confirmation
}

func (c *WaypointsDeleteAllCmd) Run(g *Globals) (err error) {
	s, err := g.open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Close()) }()

	info, err := s.Info()
	if err != nil {
		return err
	}
	if info.Waypoints == 0 {
		fmt.Fprintln(os.Stderr, "There are no waypoints on the device.")
		return nil
	}
	if err := c.confirm(fmt.Sprintf("All %d waypoints will be deleted from the device!", info.Waypoints)); err != nil {
		return err
	}
	return s.DeleteAllWaypoints()
}
