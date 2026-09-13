package shared

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestDebugfIsSilentUntilEnabled(t *testing.T) {
	defer SetDebug(false)
	SetDebug(false)

	var buf bytes.Buffer
	originalWriter := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(originalWriter)

	Debugf("found card %d", 1)
	if buf.Len() != 0 {
		t.Errorf("Debugf wrote %q while debugging was off", buf.String())
	}

	SetDebug(true)
	Debugf("found card %d", 2)
	if !strings.Contains(buf.String(), "found card 2") {
		t.Errorf("Debugf wrote %q; want it to contain %q", buf.String(), "found card 2")
	}
}

func TestDebugEnabled(t *testing.T) {
	defer SetDebug(false)

	SetDebug(true)
	if !DebugEnabled() {
		t.Error("DebugEnabled = false after SetDebug(true)")
	}
	SetDebug(false)
	if DebugEnabled() {
		t.Error("DebugEnabled = true after SetDebug(false)")
	}
}
