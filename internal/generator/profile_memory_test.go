package generator

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestProfileReportsMemory: peak RSS and GC cycles, in the text and the JSON,
// so resources: can be tuned from a real build (#384).
func TestProfileReportsMemory(t *testing.T) {
	p := newProfileWithClock(time.Now)
	p.readMemory = func() ProfileMemory { return ProfileMemory{PeakRSS: 412 << 20, GCCycles: 37} }
	var buf bytes.Buffer
	p.WriteText(&buf, time.Now())
	if !strings.Contains(buf.String(), "Memory: peak RSS 412 MiB · 37 GC cycles") {
		t.Errorf("text:\n%s", buf.String())
	}
	if m := p.Report("dev", time.Now()).Memory; m == nil || m.PeakRSS != 412<<20 || m.GCCycles != 37 {
		t.Errorf("report memory = %+v", m)
	}
	// Where the system cannot say, the RSS is left out rather than shown as 0.
	if s := (ProfileMemory{GCCycles: 2}).String(); s != "2 GC cycles" {
		t.Errorf("no RSS: %q", s)
	}
	if m := readMemory(); m.PeakRSS < 0 {
		t.Errorf("readMemory = %+v", m)
	}
}
