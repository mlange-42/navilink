//go:build !windows

// Package hint provides how-tos for known errors.
package hint

// OpenError returns a how-to for known errors when opening a serial port.
func OpenError(err error) string {
	return ""
}
