# navilink

A command line tool for Locosys NaviGPS devices (GT-11/BGT-11, GT-31/BGT-31)
connected via USB. It talks to the device using the NaviLink protocol to:

- download tracks, the data log (GT-31/BGT-31), waypoints and routes as GPX
- upload waypoints and routes from GPX
- delete tracks, waypoints and routes

## Installation

With [Go](https://go.dev/) installed:

```shell
go install github.com/mlange-42/navilink@latest
```

Pre-built binaries for Windows, Linux and macOS will be available on the
[Releases](https://github.com/mlange-42/navilink/releases) page (TODO).

> [!NOTE]
> On Windows, current Prolific drivers refuse to work with the PL2303HXA
> USB-serial chip used by NaviGPS devices. If the port can't be opened,
> navilink prints instructions for installing an older driver.

## Usage

Switch the device to NaviLink mode and connect it. The serial port is detected
automatically if it is the only one; otherwise select it with `-d` (e.g.
`-d COM4` or `-d /dev/ttyUSB0`).

Download the track and the data log:

```shell
navilink track get -o track.gpx
navilink log get -o log.gpx
```

Back up all waypoints and routes:

```shell
navilink waypoints get -o waypoints.gpx
navilink routes get -o routes.gpx
```

Upload routes, skipping routes that already exist on the device.
Route points that are not on the device yet are uploaded as waypoints:

```shell
navilink routes put -s -i tour.gpx
```

Run `navilink --help` for all commands, and `navilink <command> --help` for
their options.

## Documentation

- [docs/navilink-protocol.md](docs/navilink-protocol.md): the NaviLink protocol specification
- [TESTING.md](TESTING.md): how to test against a real device

## Credits

This is an AI-assisted port to Go of
[navilink.pl](https://www.splitbrain.org/projects/navilink)
([GitHub](https://github.com/splitbrain/navilink)) by Andreas Gohr and
contributors, Copyright (c) 2007-2009, released under the BSD 3-Clause License.

## License

This project is distributed under the [BSD 3-Clause License](LICENSE),
which includes the copyright notice of the original navilink.pl.
The protocol specification in [docs/navilink-protocol.md](docs/navilink-protocol.md)
is a copy licensed under [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/).
