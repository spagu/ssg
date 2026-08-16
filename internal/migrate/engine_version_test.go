package migrate

// Gating a flag on the engine's version (#137). Sending `--no-comments` to a
// wpexporter that predates it kills the run outright — after the project has
// been scaffolded and, in live mode, after the server is already up. The cost
// of skipping the flag is one line in the report; the cost of sending it is
// the whole migration.

import (
	"strings"
	"testing"
)

func TestParseSemverAndAtLeast(t *testing.T) {
	cases := []struct {
		banner              string
		major, minor, patch int
	}{
		{"wpexporter version v1.8.4 (8c1aacb, built 2026-08-14T14:18:43Z)", 1, 8, 4},
		{"1.8.5", 1, 8, 5},
		{"v2.0.0-rc1", 2, 0, 0},
		{"", 0, 0, 0},
		{"no version here", 0, 0, 0},
	}
	for _, c := range cases {
		major, minor, patch := parseSemver(c.banner)
		if major != c.major || minor != c.minor || patch != c.patch {
			t.Errorf("parseSemver(%q) = %d.%d.%d, want %d.%d.%d",
				c.banner, major, minor, patch, c.major, c.minor, c.patch)
		}
	}

	for _, c := range []struct {
		banner string
		want   bool
	}{
		{"v1.8.5", true}, {"v1.8.6", true}, {"v1.9.0", true}, {"v2.0.0", true},
		{"v1.8.4", false}, {"v1.7.9", false}, {"v0.9.9", false}, {"", false},
	} {
		if got := atLeast(c.banner, 1, 8, 5); got != c.want {
			t.Errorf("atLeast(%q, 1.8.5) = %v, want %v", c.banner, got, c.want)
		}
	}
}

// TestEngineMinimumRefusesOldEngines: ssg migrate is built against the flags
// and fixes of 1.8.11; an older engine produces an export that looks complete
// and is not, so it is refused BEFORE anything is written — not after the
// project is scaffolded and, in live mode, the server is up (#137's lesson).
func TestEngineMinimumRefusesOldEngines(t *testing.T) {
	for _, banner := range []string{
		"wpexporter version v1.8.4 (8c1aacb)",
		"wpexporter version v1.8.10",
		"1.7.99",
		"0.9.0",
	} {
		err := checkEngineVersion(banner, true)
		if err == nil {
			t.Errorf("%q must be refused", banner)
			continue
		}
		if !strings.Contains(err.Error(), "1.8.11") || !strings.Contains(err.Error(), "snap refresh") {
			t.Errorf("the refusal must name the minimum and a way out: %v", err)
		}
	}
}

func TestEngineMinimumAcceptsCurrent(t *testing.T) {
	for _, banner := range []string{
		"wpexporter version v1.8.11",
		"wpexporter version v1.8.12 (abc, built …)",
		"1.9.0",
		"2.0.0",
	} {
		if err := checkEngineVersion(banner, true); err != nil {
			t.Errorf("%q must be accepted: %v", banner, err)
		}
	}
}

// TestEngineMinimumToleratesAnUnreadableBanner: a wrapper or a fork that
// prints something else is not proof of an old engine, and blocking a working
// setup over a formatting difference would be worse than the risk. It is
// reported, not refused.
func TestEngineMinimumToleratesAnUnreadableBanner(t *testing.T) {
	if err := checkEngineVersion("", true); err != nil {
		t.Fatalf("an unreadable banner must not block the run: %v", err)
	}
	if err := checkEngineVersion("no version here", true); err != nil {
		t.Fatalf("an unreadable banner must not block the run: %v", err)
	}
}

// TestFetchRefusesOldEngineBeforeWriting: the refusal happens before the
// engine is ever run, which is what keeps a half-scaffolded project off disk.
func TestFetchRefusesOldEngineBeforeWriting(t *testing.T) {
	p, _ := Lookup("wordpress")
	ran := false
	_, err := p.Fetch("https://e.com", Options{
		Dest:     t.TempDir(),
		LookPath: func(string) (string, error) { return "/bin/wpexporter", nil },
		Version:  func(string) string { return "wpexporter version v1.8.4" },
		Run: func(string, []string, bool) error {
			ran = true
			return nil
		},
	})
	if err == nil {
		t.Fatal("an old engine must fail the fetch")
	}
	if ran {
		t.Fatal("the engine must not be run at all")
	}
}

// TestVersionOutputDefaultSeam: without a stub the real binary is asked, and a
// tool that cannot be run yields an empty banner rather than a panic.
func TestVersionOutputDefaultSeam(t *testing.T) {
	if got := (Options{}).versionOutput("/nonexistent/wpexporter"); got != "" {
		t.Fatalf("an unrunnable tool must yield no banner, got %q", got)
	}
	// /bin/echo answers --version with its own banner: proof the seam really
	// executes the tool and returns what it printed.
	if got := (Options{}).versionOutput("/bin/echo"); !strings.Contains(got, "echo") {
		t.Fatalf("the default seam must exec the tool, got %q", got)
	}
}
