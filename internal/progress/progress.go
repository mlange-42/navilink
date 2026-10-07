// Package progress draws a simple progress bar on a terminal.
package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	barWidth     = 30
	labelWidth   = 19 // length of the longest label, to align bars
	redrawPeriod = 100 * time.Millisecond
)

// Bar is a progress bar on a single terminal line.
// A disabled bar draws nothing; its Logf still prints messages.
type Bar struct {
	w       io.Writer
	enabled bool
	label   string
	total   int
	done    int
	start   time.Time
	drawn   time.Time
	lineLen int // length of the visible line, 0 if none
}

// New creates a progress bar on stderr.
// It is disabled if stderr is not a terminal, or if disable is true.
func New(label string, disable bool) *Bar {
	enabled := !disable && term.IsTerminal(int(os.Stderr.Fd()))
	return NewWriter(os.Stderr, label, enabled)
}

// NewWriter creates a progress bar on the given writer.
func NewWriter(w io.Writer, label string, enabled bool) *Bar {
	return &Bar{w: w, enabled: enabled, label: label, start: time.Now()}
}

// Update sets the progress. It redraws at most every 100ms, and always
// when the total is reached.
func (b *Bar) Update(done, total int) {
	b.done, b.total = done, total
	if b.enabled && (done >= total || time.Since(b.drawn) >= redrawPeriod) {
		b.draw()
	}
}

// Add increases the progress by n.
func (b *Bar) Add(n int) {
	b.Update(b.done+n, b.total)
}

// Logf prints a message on its own line, above the progress bar.
func (b *Bar) Logf(format string, args ...any) {
	b.clear()
	_, _ = fmt.Fprintf(b.w, format, args...)
	if b.enabled && b.total > 0 {
		b.draw()
	}
}

// Finish removes the progress bar from the terminal.
func (b *Bar) Finish() {
	b.clear()
}

func (b *Bar) draw() {
	b.drawn = time.Now()

	frac := 1.0
	if b.total > 0 {
		frac = min(1, float64(b.done)/float64(b.total))
	}
	filled := int(frac * barWidth)
	bar := strings.Repeat("=", filled)
	if filled < barWidth {
		bar += ">" + strings.Repeat(" ", barWidth-filled-1)
	}
	elapsed := time.Since(b.start).Truncate(time.Second)
	line := fmt.Sprintf("%-*s [%s] %3.0f%% %d/%d %s", labelWidth, b.label, bar, frac*100, b.done, b.total, elapsed)
	// Pad with spaces to overwrite a longer previous line.
	pad := max(0, b.lineLen-len(line))
	_, _ = fmt.Fprintf(b.w, "\r%s%s", line, strings.Repeat(" ", pad))
	b.lineLen = len(line)
}

func (b *Bar) clear() {
	if b.lineLen > 0 {
		_, _ = fmt.Fprintf(b.w, "\r%s\r", strings.Repeat(" ", b.lineLen))
		b.lineLen = 0
	}
}
