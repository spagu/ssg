package migrate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLookupAndProviders(t *testing.T) {
	p, ok := Lookup("wordpress")
	if !ok || p.Name() != "wordpress" {
		t.Fatalf("Lookup(wordpress) = %v, %v", p, ok)
	}
	// Case- and whitespace-insensitive: CLI input is human input.
	if _, ok := Lookup("  WordPress "); !ok {
		t.Fatal("Lookup must normalise case and spaces")
	}
	if _, ok := Lookup("drupal"); ok {
		t.Fatal("drupal is not built yet")
	}
	all := Providers()
	if len(all) != 1 || all[0].Name() != "wordpress" {
		t.Fatalf("Providers() = %v", all)
	}
	if all[0].Version() == "" || all[0].Description() == "" {
		t.Fatal("provider must declare version and description")
	}
}

func TestValidateURL(t *testing.T) {
	for _, ok := range []string{"https://example.com", "http://127.0.0.1:8080/blog"} {
		if _, err := ValidateURL(ok); err != nil {
			t.Errorf("ValidateURL(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"example.com", "ftp://x.com", "https://", "ht tp://x"} {
		if _, err := ValidateURL(bad); err == nil {
			t.Errorf("ValidateURL(%q) must fail", bad)
		}
	}
}

func TestWpexporterArgs(t *testing.T) {
	base := Options{Dest: "content/x"}

	// No selection → full default export, no --no-* flags.
	args, skipped, err := wpexporterArgs("https://e.com", base)
	if err != nil || len(skipped) != 0 {
		t.Fatalf("default: %v %v", skipped, err)
	}
	joined := strings.Join(args, " ")
	if !strings.HasPrefix(joined, "export -u https://e.com -f markdown -o content/x --link-style root") {
		t.Fatalf("base args wrong: %q", joined)
	}
	if strings.Contains(joined, "--no-") || strings.Contains(joined, "-q") {
		t.Fatalf("default must not disable anything: %q", joined)
	}

	// Quiet adds -q.
	quiet := base
	quiet.Quiet = true
	args, _, _ = wpexporterArgs("https://e.com", quiet)
	if !strings.Contains(strings.Join(args, " "), " -q") {
		t.Fatal("quiet must pass -q")
	}

	// comments: recognised but unsupported → skipped, not an error.
	com := base
	com.Content = []string{"comments", "pages"}
	_, skipped, err = wpexporterArgs("https://e.com", com)
	if err != nil || len(skipped) != 1 || skipped[0] != "comments" {
		t.Fatalf("comments: skipped=%v err=%v", skipped, err)
	}

	// Unknown kind: hard error naming the valid set (a typo must not export
	// the whole site).
	typo := base
	typo.Content = []string{"postz"}
	if _, _, err = wpexporterArgs("https://e.com", typo); err == nil ||
		!strings.Contains(err.Error(), "comments") || !strings.Contains(err.Error(), "pages") {
		t.Fatalf("unknown kind error must list valid kinds, got %v", err)
	}

	// Blank entries are ignored.
	blank := base
	blank.Content = []string{"", " "}
	if _, _, err = wpexporterArgs("https://e.com", blank); err != nil {
		t.Fatalf("blank kinds: %v", err)
	}
}

// TestWpexporterMetadataAlwaysShips: a --content list selects CONTENT; the
// site's metadata (tags, users, menus) rides along regardless, because a
// migration that silently drops the navigation, the category names and the
// authors is not a migration (1.8.30). Metadata leaves only when named
// explicitly as no-<kind>, and content kinds cannot be excluded that way.
func TestWpexporterMetadataAlwaysShips(t *testing.T) {
	base := Options{Dest: "content/x"}

	sel := base
	sel.Content = []string{"pages", " POSTS ", "media"}
	args, skipped, err := wpexporterArgs("https://e.com", sel)
	if err != nil || len(skipped) != 0 {
		t.Fatalf("selection: %v %v", skipped, err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--no-products") {
		t.Errorf("unrequested content kind must be disabled: %q", joined)
	}
	for _, on := range []string{"--no-pages", "--no-posts", "--no-media",
		"--no-tags", "--no-users", "--no-menus"} {
		if strings.Contains(joined, on) {
			t.Errorf("must not disable %s: %q", on, joined)
		}
	}

	excl := base
	excl.Content = []string{"pages", "no-menus"}
	if args, _, err = wpexporterArgs("https://e.com", excl); err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(args, " ")
	if !strings.Contains(joined, "--no-menus") || strings.Contains(joined, "--no-tags") {
		t.Errorf("explicit exclusion wrong: %q", joined)
	}

	bad := base
	bad.Content = []string{"no-pages"}
	if _, _, err = wpexporterArgs("https://e.com", bad); err == nil ||
		!strings.Contains(err.Error(), "menus") {
		t.Fatalf("only metadata may be excluded; error must list it: %v", err)
	}
}

func TestWordpressFetch(t *testing.T) {
	dest := t.TempDir()
	writeFile := func(rel, body string) {
		path := filepath.Join(dest, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	p, _ := Lookup("wordpress")

	// Missing binary → install instructions, never a bare exec error.
	t.Setenv("SNAP", "") // plain install: the tool is the user's to install
	_, err := p.Fetch("https://e.com", Options{
		Dest:     dest,
		LookPath: func(string) (string, error) { return "", errors.New("nope") },
	})
	if err == nil || !strings.Contains(err.Error(), "cmd/wpexporter@latest") {
		t.Fatalf("missing binary must give the correct install path, got %v", err)
	}

	// Engine failure is wrapped with context.
	_, err = p.Fetch("https://e.com", Options{
		Dest:     dest,
		LookPath: func(string) (string, error) { return "/bin/wpexporter", nil },
		Run:      func(string, []string, bool) error { return errors.New("boom") },
	})
	if err == nil || !strings.Contains(err.Error(), "wpexporter failed") {
		t.Fatalf("run error must be wrapped, got %v", err)
	}

	// Invalid URL fails before any exec.
	if _, err = p.Fetch("not-a-url", Options{Dest: dest}); err == nil {
		t.Fatal("invalid URL must fail")
	}

	// Bad content kind fails before any exec.
	_, err = p.Fetch("https://e.com", Options{
		Dest:     dest,
		Content:  []string{"bogus"},
		LookPath: func(string) (string, error) { return "/bin/wpexporter", nil },
		Run: func(string, []string, bool) error {
			t.Fatal("must not run on invalid kinds")
			return nil
		},
	})
	if err == nil {
		t.Fatal("unknown kind must fail")
	}

	// Success: the stubbed engine "exports" files; the report counts them and
	// carries the comments warning.
	var gotBin string
	report, err := p.Fetch("https://e.com", Options{
		Dest:    dest,
		Content: []string{"pages", "posts", "media", "comments"},
		LookPath: func(name string) (string, error) {
			return "/resolved/" + name, nil
		},
		Run: func(bin string, args []string, quiet bool) error {
			gotBin = bin
			writeFile("pages/about.md", "# a")
			writeFile("pages/contact.md", "# c")
			writeFile("posts/news/hello.md", "# h")
			writeFile("posts/news/notes.txt", "not markdown") // must not count
			writeFile("media/logo.png", "png")
			writeFile("metadata.json", "{}")
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotBin != "/resolved/wpexporter" {
		t.Fatalf("must exec the resolved path, got %q", gotBin)
	}
	if report.Pages != 2 || report.Posts != 1 || report.Media != 1 {
		t.Fatalf("counts = %d/%d/%d", report.Pages, report.Posts, report.Media)
	}
	if report.Provider != "wordpress@"+wordpressVersion {
		t.Fatalf("provider stamp = %q", report.Provider)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "comments") {
		t.Fatalf("comments warning missing: %v", report.Warnings)
	}
}

// TestMissingEngineMessage: the advice must match how ssg was installed. A
// snap cannot use the host's wpexporter (confinement forbids running another
// snap), so telling a snap user to "go install" it would be a lie — the snap
// bundles the engine and a missing one means the snap is stale (#114).
func TestMissingEngineMessage(t *testing.T) {
	snap := missingEngineMessage("/snap/static-site-generator/current")
	if !strings.Contains(snap, "snap refresh") || strings.Contains(snap, "go install") {
		t.Fatalf("snap advice wrong: %s", snap)
	}
	plain := missingEngineMessage("")
	if !strings.Contains(plain, "go install github.com/tradik/wpexporter/cmd/wpexporter@latest") {
		t.Fatalf("plain advice wrong: %s", plain)
	}
}

func TestCountExportMissingDirs(t *testing.T) {
	pages, posts, media := countExport(filepath.Join(t.TempDir(), "nope"))
	if pages != 0 || posts != 0 || media != 0 {
		t.Fatalf("missing dest must count zero, got %d/%d/%d", pages, posts, media)
	}
}

func TestOptionsDefaultSeams(t *testing.T) {
	// The default LookPath is the real one — resolve a binary every Linux CI
	// has, then run it through the default runner (both quiet and loud).
	var o Options
	path, err := o.lookPath("sh")
	if err != nil {
		t.Skipf("no sh in PATH: %v", err)
	}
	if err := o.run(path, []string{"-c", "exit 0"}); err != nil {
		t.Fatalf("default run: %v", err)
	}
	o.Quiet = true
	if err := o.run(path, []string{"-c", "exit 3"}); err == nil {
		t.Fatal("exit 3 must surface as an error")
	}
}
