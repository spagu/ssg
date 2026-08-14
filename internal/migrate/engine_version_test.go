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

// TestCommentsFlagGatedOnEngineVersion: the exact failure #137 reports — a
// --content list without comments used to emit a flag the installed engine
// rejects.
func TestCommentsFlagGatedOnEngineVersion(t *testing.T) {
	p, _ := Lookup("wordpress")
	dest := t.TempDir()

	run := func(banner string) (args []string, rep *Report) {
		var got []string
		r, err := p.Fetch("https://e.com", Options{
			Dest:     dest,
			Content:  []string{"pages", "media"},
			LookPath: func(string) (string, error) { return "/bin/wpexporter", nil },
			Version:  func(string) string { return banner },
			Run: func(_ string, a []string, _ bool) error {
				got = a
				return nil
			},
		})
		if err != nil {
			t.Fatalf("fetch on %q: %v", banner, err)
		}
		return got, r
	}

	// An engine too old for the flag: it is not sent, and the report says the
	// comments came along anyway.
	args, rep := run("wpexporter version v1.8.4 (8c1aacb)")
	if strings.Contains(strings.Join(args, " "), "--no-comments") {
		t.Fatalf("an old engine must not be sent the flag: %v", args)
	}
	if !strings.Contains(strings.Join(rep.Warnings, " | "), "cannot be asked to skip them") {
		t.Fatalf("the operator must be told: %v", rep.Warnings)
	}

	// A current engine: the exclusion is honoured and nothing is explained.
	args, rep = run("wpexporter version v1.8.5")
	if !strings.Contains(strings.Join(args, " "), "--no-comments") {
		t.Fatalf("a current engine must honour the exclusion: %v", args)
	}
	for _, w := range rep.Warnings {
		if strings.Contains(w, "cannot be asked to skip") {
			t.Fatalf("nothing to explain when the flag was sent: %v", rep.Warnings)
		}
	}

	// An unreadable banner is treated as old — skipping a flag is recoverable,
	// sending an unknown one is not.
	args, _ = run("")
	if strings.Contains(strings.Join(args, " "), "--no-comments") {
		t.Fatalf("an unknown engine version must be treated as old: %v", args)
	}
}

// TestCommentsAskedForNeedsNoExplanation: a run that wants comments has
// nothing to report about them, whatever the engine's age.
func TestCommentsAskedForNeedsNoExplanation(t *testing.T) {
	if w := commentsExclusionWarning(Options{Content: []string{"pages", " Comments "}}); w != "" {
		t.Fatalf("comments were requested: %q", w)
	}
	if w := commentsExclusionWarning(Options{}); w != "" {
		t.Fatalf("no --content list means no exclusion: %q", w)
	}
	if w := commentsExclusionWarning(Options{Content: []string{"pages"}}); w == "" {
		t.Fatal("an unlisted kind on an old engine must be explained")
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
