# Testing with a device

Unit tests (`go test ./...`) use a simulated device. This guide covers manual
tests against a real Locosys NaviGPS (or BGT-31/GT-31) connected via USB.

The steps go from read-only to destructive. Steps 1–3 never change data on the
device. Steps 4–7 only add and remove the test waypoints and routes from
[testdata/device/](testdata/device/), all named `ZZ…` (or `R02…`) to avoid
clashes with your own data. Step 8 deletes data and is optional.

Commands are run from the repository root with `go run .`. Outputs go to
`out/` (`.gpx` files are ignored by Git outside `testdata/`):

```shell
mkdir out
```

## 0. Setup

1. Connect the device and switch on NaviLink mode.
2. Find its serial port:

   ```shell
   go run . ports
   ```

   If only one port is listed, it is used automatically. Otherwise, pass it
   with `-d COM4` (or `-d /dev/ttyUSB0`) or set `NAVILINK_DEVICE`.

On Windows, if the port can't be opened ("A device which does not exist was
specified"), the PL2303 driver is too new. The error message explains how to
install an older driver.

If anything fails, re-run the command with `-v` to log every packet to stderr,
and include that output in bug reports.

## 1. Device info

```shell
go run . info
```

**Expected:** username, firmware, serial number and the number of waypoints,
routes and trackpoints. Note the counts; later steps refer to them.

## 2. Download track and log

```shell
go run . track get -o out/track.gpx
go run . log get -o out/log.gpx      # BGT-31/GT-31 only
```

**Expected:**

- `track.gpx` has as many points as `info` reports trackpoints.
- Times are in UTC (`…Z`) and match when the track was recorded.
- Elevations are plausible in meters.
- `<speed>` is in m/s (GPX standard): 5 m/s = 18 km/h. Track speed comes in
  steps of 0.56 m/s (the device stores 2 km/h steps).
- For a trip recorded in both, `log.gpx` covers the same time span and has
  similar elevations as `track.gpx`, usually with more points.

Load the files into a GPX viewer and check that the route looks right.

## 3. Back up waypoints and routes

```shell
go run . waypoints get -o out/backup-waypoints.gpx
go run . routes get -o out/backup-routes.gpx
```

**Expected:**

- `backup-waypoints.gpx` has one `<wpt>` per waypoint (as counted by `info`)
  with name, position, elevation, time and symbol.
- `backup-routes.gpx` has one `<rte>` per route (as counted by `info`), sorted
  by name, with its waypoints as `<rtept>` in the order shown on the device.

Keep these files to restore waypoints and routes.

## 4. Upload test waypoints

```shell
go run . waypoints put -i testdata/device/waypoints.gpx
go run . waypoints get -o out/after-put.gpx
```

**Expected:** `Uploaded 5 waypoints.` and these new waypoints in
`after-put.gpx` and in the device menu:

| Name     | Position              | Elevation | Time (UTC)             | Symbol   | Tests                               |
|----------|-----------------------|-----------|------------------------|----------|-------------------------------------|
| `ZZT1`   | 51.3462, 12.3846      | 113.08    | 2026-10-07 12:00:00    | Flag     | plain waypoint                      |
| `ZZT2BR` | 51.3401, 12.3712      | 98.45     | 2026-10-07 12:30:00    | Bridge   | name normalization, time zone       |
| `ZZT3`   | 51.3555, 12.4021      | 0         | time of upload         | Flag     | geocache `GC` prefix, missing values|
| `ZZT4SO` | -33.4489, -70.6693    | 519.99    | 2026-01-01 00:00:00    | Summit   | negative coordinates                |
| `ZZT5`   | 51.3333, 12.3333      | 0         | 2026-12-31 23:59:59    | Waypoint | unknown symbol, end of year         |

Elevations differ slightly from the input because the device stores feet.
The device may show times in local time.

## 5. Skip existing waypoints

```shell
go run . waypoints put -s -i testdata/device/waypoints.gpx
```

**Expected:** `Skipped existing waypoint …` for all five, then
`Uploaded 0 waypoints.` `info` shows the same waypoint count as after step 4.

## 6. Remove test waypoints

```shell
go run . waypoints remove -i testdata/device/waypoints-remove.gpx
```

**Expected:** `Waypoint ZZT9 not found on the device`, then a prompt to delete
2 waypoints. Answer `y`; the output is `Deleted 2 waypoints.` `ZZT1` and `ZZT2BR`
are gone, the other test waypoints are still there.

Clean up the remaining test waypoints:

```shell
go run . waypoints remove -i testdata/device/waypoints.gpx
```

**Expected:** `not found` for `ZZT1` and `ZZT2BR`, then 3 waypoints deleted.
`info` shows the waypoint count from step 1.

## 7. Routes

Upload the two test routes from [routes.gpx](testdata/device/routes.gpx):

```shell
go run . routes put -i testdata/device/routes.gpx
go run . routes get -o out/after-routes.gpx
```

**Expected:** `Uploaded 2 routes and 5 new waypoints.` and these routes on the
device and in `after-routes.gpx`:

| Route          | Waypoints                   | Tests                            |
|----------------|-----------------------------|----------------------------------|
| `ZZ ROUTE 2`   | `R02001`, `R02002`, `ZZR1A` | generated names, reused waypoint |
| `ZZ ROUTE ONE` | `ZZR1A`, `ZZR1B`, `ZZR1C`   | route name normalization         |

`ZZR1B` has symbol Bridge and elevation 110.03. The other new waypoints have
elevation 0 and the time of upload.

Upload again, skipping existing routes:

```shell
go run . routes put -s -i testdata/device/routes.gpx
```

**Expected:** `Skipped existing route …` for both, then
`Uploaded 0 routes and 0 new waypoints.`

Waypoints used by routes can't be deleted:

```shell
go run . waypoints remove -y -i testdata/device/routes-waypoints.gpx
```

**Expected:** removal fails for all 5 waypoints (`maybe it is in use?`), and
the command exits with an error.

Remove the test routes, then their waypoints:

```shell
go run . routes remove -i testdata/device/routes.gpx
go run . waypoints remove -i testdata/device/routes-waypoints.gpx
```

**Expected:** `Deleted 2 routes.`, then `Deleted 5 waypoints.` `info` shows the
counts from step 1.

## 8. Destructive tests (optional)

These delete your data. Do steps 2 and 3 first to have backups.

### Delete all routes and waypoints

Waypoints can only be deleted after all routes are deleted.

```shell
go run . waypoints delete-all                          # fails if there are routes
go run . routes delete-all
go run . waypoints delete-all
go run . info                                          # waypoints: 0, routes: 0
go run . waypoints put -i out/backup-waypoints.gpx     # restore waypoints first
go run . routes put -i out/backup-routes.gpx           # then routes
```

**Expected:** the first `waypoints delete-all` fails, asking to delete routes
first. After restoring, `waypoints get` and `routes get` match the backups.
Waypoint and route IDs may differ, which is fine as long as names and contents
match.

### Delete the track

There is no way to upload a track back to the device.

```shell
go run . track delete
go run . info                                          # trackpoints: 0
```

## Checklist

- [ ] `ports` lists the device
- [ ] `info` shows plausible values
- [ ] `track get` matches the recorded trip
- [ ] `log get` matches the recorded trip (BGT-31/GT-31)
- [ ] `waypoints get` lists all waypoints
- [ ] `waypoints put` uploads with the values in step 4
- [ ] `waypoints put -s` skips existing waypoints
- [ ] `waypoints remove` removes by name and reports missing ones
- [ ] `routes get` lists all routes with their waypoints
- [ ] `routes put` uploads routes and missing waypoints
- [ ] `routes put -s` skips existing routes
- [ ] waypoints used by routes can't be removed
- [ ] `routes remove` removes by name
- [ ] `routes delete-all` and `waypoints delete-all` (optional)
- [ ] `track delete` (optional)
- [ ] `-q` ends NaviLink mode on the device
