package progress

import (
	"bytes"
	"strings"
	"testing"
)

func TestBar(t *testing.T) {
	var buf bytes.Buffer
	b := NewWriter(&buf, "Test", true)
	b.Update(0, 4)
	if !strings.Contains(buf.String(), "[>                             ]   0% 0/4") {
		t.Errorf("start: got %q", buf.String())
	}

	buf.Reset()
	b.Update(4, 4) // always drawn when complete
	if !strings.Contains(buf.String(), "[==============================] 100% 4/4") {
		t.Errorf("complete: got %q", buf.String())
	}

	buf.Reset()
	b.Logf("message %d\n", 1)
	out := buf.String()
	clearLine := "\r" + strings.Repeat(" ", b.lineLen) + "\r"
	if !strings.HasPrefix(out, clearLine+"message 1\n\rTest ") {
		t.Errorf("log: got %q", out)
	}

	buf.Reset()
	b.Finish()
	if buf.String() != clearLine {
		t.Errorf("finish: got %q", buf.String())
	}
	buf.Reset()
	b.Finish()
	if buf.Len() != 0 {
		t.Errorf("second finish: got %q", buf.String())
	}
}

func TestBarDisabled(t *testing.T) {
	var buf bytes.Buffer
	b := NewWriter(&buf, "Test", false)
	b.Update(1, 2)
	b.Add(1)
	b.Logf("message\n")
	b.Finish()
	if buf.String() != "message\n" {
		t.Errorf("got %q", buf.String())
	}
}
