// Package hint provides how-tos for known errors.
package hint

import (
	"errors"

	"golang.org/x/sys/windows"
)

// pl2303Hint explains how to fix opening a port that the Prolific driver refuses.
const pl2303Hint = `
The serial port exists but its driver refuses to open it. NaviGPS devices use
a Prolific PL2303HXA USB-serial chip, which is blocked by Prolific drivers newer
than 3.4 (Device Manager shows "PL2303HXA PHASED OUT SINCE 2012").

To fix this, install the older Prolific driver 3.3.2.102:
  1. Download the driver package (search for "PL2303 driver 3.3.2.102").
  2. Install it, or extract it and run in an admin shell:
       pnputil /add-driver <path>\ser2pl.inf
  3. Connect the GPS. In Device Manager, under "Ports (COM & LPT)", right-click
     the PL2303 device > Update driver > Browse my computer >
     Let me pick from a list, and select version 3.3.2.102.
  4. Reconnect the GPS and check the port with "navilink ports".`

// OpenError returns a how-to for known errors when opening a serial port.
func OpenError(err error) string {
	if errors.Is(err, windows.ERROR_DEV_NOT_EXIST) {
		return pl2303Hint
	}
	return ""
}
