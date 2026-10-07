// Command navilink downloads and uploads data from and to Locosys NaviGPS
// devices using the NaviLink protocol.
package main

import (
	"github.com/alecthomas/kong"
)

// CLI is the command line interface of navilink.
type CLI struct {
	Globals

	Ports PortsCmd `cmd:"" help:"List available serial ports."`
	Info  InfoCmd  `cmd:"" help:"Print device information and the number of waypoints, routes and trackpoints."`

	Track struct {
		Get    TrackGetCmd    `cmd:"" help:"Download track data as GPX."`
		Delete TrackDeleteCmd `cmd:"" help:"Delete all track data from the device."`
	} `cmd:"" aliases:"tp" help:"Manage track data."`

	Waypoints struct {
		Get       WaypointsGetCmd       `cmd:"" help:"Download waypoints as GPX."`
		Put       WaypointsPutCmd       `cmd:"" help:"Upload waypoints from GPX. Names are shortened to 6 characters (A-Z, 0-9)."`
		Remove    WaypointsRemoveCmd    `cmd:"" help:"Delete the waypoints given as GPX from the device, matched by name."`
		DeleteAll WaypointsDeleteAllCmd `cmd:"" help:"Delete all waypoints from the device."`
	} `cmd:"" aliases:"wp" help:"Manage waypoints."`

	Log struct {
		Get LogGetCmd `cmd:"" help:"Download internal log data as GPX (BGT-31/GT-31 only)."`
	} `cmd:"" help:"Access the internal data logger."`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("navilink"),
		kong.Description("Download or upload data to a Locosys NaviGPS device via the NaviLink protocol."),
		kong.UsageOnError(),
		kong.Bind(&cli.Globals),
	)
	ctx.FatalIfErrorf(ctx.Run())
}
