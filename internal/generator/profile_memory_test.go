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

// TestProfileMemoryFixedAtFinish: a finished profile reports the memory Finish
// took, however much the process allocates or collects afterwards, so two
// writes of one report are byte-identical.
func TestProfileMemoryFixedAtFinish(t *testing.T) {
	p := newProfileWithClock(time.Now)
	calls := uint32(0)
	p.readMemory = func() ProfileMemory { calls++; return ProfileMemory{GCCycles: calls} }
	if m := p.memory(); m.GCCycles != 1 {
		t.Errorf("a running build reads live: %+v", m)
	}
	p.Finish()
	p.Finish()
	for i := 0; i < 2; i++ {
		if m := p.memory(); m.GCCycles != 2 {
			t.Errorf("after Finish, read %d = %+v", i, m)
		}
	}
}
