package main

import (
	"fmt"
	"testing"

	"golang.org/x/sys/windows"
)

func TestOpenErrorHint(t *testing.T) {
	if openErrorHint(fmt.Errorf("wrapped: %w", windows.ERROR_DEV_NOT_EXIST)) == "" {
		t.Error("expected hint for ERROR_DEV_NOT_EXIST")
	}
	if openErrorHint(windows.ERROR_ACCESS_DENIED) != "" {
		t.Error("expected no hint for other errors")
	}
}
