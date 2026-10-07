//go:build !windows

package main

// openErrorHint returns a how-to for known errors when opening a serial port.
func openErrorHint(err error) string {
	return ""
}
