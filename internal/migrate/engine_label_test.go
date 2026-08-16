package migrate

// Naming the engine that actually ran (#140). A snap carries its own copy, so
// upgrading the host's wpexporter changes nothing — and until this line
// existed, nothing in the run said which binary produced the export.

import (
	"strings"
	"testing"
)

func TestEngineLabel(t *testing.T) {
	t.Setenv("SNAP", "")
	got := engineLabel("/usr/local/bin/wpexporter", "wpexporter version v1.8.11 (abc)")
	if !strings.Contains(got, "1.8.11") || !strings.Contains(got, "/usr/local/bin/wpexporter") {
		t.Fatalf("a host binary names its version and path: %q", got)
	}

	// Inside a snap, the bundled copy is named as such — the one line that
	// explains why `snap refresh wpexporter` changed nothing.
	t.Setenv("SNAP", "/snap/static-site-generator/current")
	got = engineLabel("/snap/static-site-generator/current/bin/wpexporter", "wpexporter version 1.8.4")
	if !strings.Contains(got, "1.8.4") || !strings.Contains(got, "bundled with the snap") {
		t.Fatalf("the bundled engine must say so: %q", got)
	}

	// A binary outside the snap directory is still the host's, even in a snap.
	got = engineLabel("/usr/bin/wpexporter", "wpexporter version 1.8.6")
	if strings.Contains(got, "bundled") {
		t.Fatalf("a host path must not be reported as bundled: %q", got)
	}

	// An unreadable banner says so rather than inventing a number.
	t.Setenv("SNAP", "")
	if got = engineLabel("/usr/bin/wpexporter", "not a version"); !strings.Contains(got, "unknown version") {
		t.Fatalf("an unreadable banner: %q", got)
	}
}

// TestFetchReportsEngine: the label reaches the report the operator reads.
func TestFetchReportsEngine(t *testing.T) {
	p, _ := Lookup("wordpress")
	rep, err := p.Fetch("https://e.com", Options{
		Dest:     t.TempDir(),
		LookPath: func(string) (string, error) { return "/opt/wpexporter", nil },
		Version:  func(string) string { return "wpexporter version v1.8.11" },
		Run:      func(string, []string, bool) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rep.Engine, "1.8.11") {
		t.Fatalf("the report must name the engine that ran: %q", rep.Engine)
	}
}
