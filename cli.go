package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mlange-42/navilink/internal/gpx"
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
		return nil, fmt.Errorf("can't open %s: %w%s", device, err, openErrorHint(err))
	}
	if err := port.SetReadTimeout(100 * time.Millisecond); err != nil {
		port.Close()
		return nil, err
	}

	conn := navilink.NewConn(port)
	if g.Verbose {
		conn.Debug = os.Stderr
	}
	client := navilink.NewClient(conn)
	if err := client.Sync(); err != nil {
		port.Close()
		return nil, err
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
