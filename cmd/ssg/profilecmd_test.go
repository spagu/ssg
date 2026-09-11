package main

// The build profile on the command line (GO-097).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// writeReport puts a profile report in the working directory, as a profiled
// build would.
func writeReport(t *testing.T) {
	t.Helper()
	p := generator.NewProfile(generator.ProfileJSON)
	p.Add("Generating site", 400*time.Millisecond)
	p.Page("/configuration/", 25*time.Millisecond)
	p.Page("/about/", 4*time.Millisecond)
	p.Finish()
	if _, err := p.WriteJSON(".", "1.8.60", time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
}

// TestProfilePageReportsOnePage: the URL a person types finds the page the
// report wrote, with or without the slashes.
func TestProfilePageReportsOnePage(t *testing.T) {
	t.Chdir(t.TempDir())
	writeReport(t)
	for _, arg := range []string{"/configuration/", "configuration", "/configuration/index.html"} {
		if code := runProfilePage([]string{arg}); code != 0 {
			t.Errorf("%q: exit %d", arg, code)
		}
	}
}

// TestProfilePageWithoutAReport: each way of having nothing to answer from is
// reported as itself, and none of them is an exit 0.
func TestProfilePageWithoutAReport(t *testing.T) {
	t.Chdir(t.TempDir())
	if code := runProfilePage(nil); code != 2 {
		t.Errorf("no argument: exit %d, want 2", code)
	}
	if code := runProfilePage([]string{"/x/"}); code != 1 {
		t.Errorf("no report: exit %d, want 1", code)
	}
	if err := os.WriteFile(generator.ProfileFileName, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runProfilePage([]string{"/x/"}); code != 1 {
		t.Errorf("unreadable report: exit %d, want 1", code)
	}
	writeReport(t)
	if code := runProfilePage([]string{"/never-built/"}); code != 1 {
		t.Errorf("unknown page: exit %d, want 1", code)
	}
}

// TestProfileSubcommandDispatch: `ssg profile page` is a subcommand, while a
// source directory called "profile" still builds — the verb+noun rule.
func TestProfileSubcommandDispatch(t *testing.T) {
	if !isProfileSubcommand("page") {
		t.Error("`profile page` must dispatch")
	}
	for _, noun := range []string{"stats", "", "--help"} {
		if isProfileSubcommand(noun) {
			t.Errorf("`profile %s` must not dispatch", noun)
		}
	}
	if _, handled := dispatchSubcommand([]string{"profile", "build"}); handled {
		t.Error("an unknown noun after profile must fall through to the build")
	}
}

// TestNormalizeProfileURL: the report writes URLs; a person types paths.
func TestNormalizeProfileURL(t *testing.T) {
	cases := map[string]string{
		"/a/":            "/a/",
		"a":              "/a/",
		"/a/index.html":  "/a/",
		"  /a/b/  ":      "/a/b/",
		"/feed.xml":      "/feed.xml",
		"/a/b/index.htm": "/a/b/index.htm",
	}
	for in, want := range cases {
		if got := normalizeProfileURL(in); got != want {
			t.Errorf("normalizeProfileURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestEmitProfileWritesTheArtifact: text mode prints and writes nothing; json
// mode writes the file beside the project, never into the output tree.
func TestEmitProfileWritesTheArtifact(t *testing.T) {
	t.Chdir(t.TempDir())
	out := t.TempDir()

	p := generator.NewProfile(generator.ProfileText)
	p.Add("Generating site", time.Millisecond)
	emitProfile(p, &config.Config{Profile: generator.ProfileText, OutputDir: out, Quiet: true})
	if _, err := os.Stat(generator.ProfileFileName); err == nil {
		t.Error("text mode must not write the artifact")
	}

	j := generator.NewProfile(generator.ProfileJSON)
	j.Add("Generating site", time.Millisecond)
	emitProfile(j, &config.Config{Profile: generator.ProfileJSON, OutputDir: out, Quiet: true})
	if _, err := os.Stat(generator.ProfileFileName); err != nil {
		t.Errorf("json mode did not write the artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, generator.ProfileFileName)); err == nil {
		t.Error("the report must not be published into the site's output")
	}
	// A nil profile is the off switch, and emitting it does nothing at all.
	emitProfile(nil, &config.Config{Profile: generator.ProfileJSON})
}

// TestEmitProfileSurvivesAnUnwritableTarget: a report that cannot be written
// warns; it does not panic and does not fail the build that already succeeded.
func TestEmitProfileSurvivesAnUnwritableTarget(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir(generator.ProfileFileName, 0o755); err != nil {
		t.Fatal(err)
	}
	p := generator.NewProfile(generator.ProfileJSON)
	p.Add("x", time.Millisecond)
	emitProfile(p, &config.Config{Profile: generator.ProfileJSON, Quiet: true})
}

// TestProfileFlagsParse: the bare flag, the json form, an unknown mode, and the
// pprof directory.
func TestProfileFlagsParse(t *testing.T) {
	cfg := &config.Config{}
	parseFlags([]string{"--profile"}, cfg)
	if cfg.Profile != generator.ProfileText {
		t.Errorf("--profile = %q", cfg.Profile)
	}
	cfg = &config.Config{}
	parseFlags([]string{"--profile=json"}, cfg)
	if cfg.Profile != generator.ProfileJSON {
		t.Errorf("--profile=json = %q", cfg.Profile)
	}
	cfg = &config.Config{}
	parseFlags([]string{"--profile=nonsense"}, cfg)
	if cfg.Profile != "" {
		t.Errorf("an unknown mode must be refused, got %q", cfg.Profile)
	}
	cfg = &config.Config{}
	parseFlags([]string{"--profile-pprof=/tmp/x"}, cfg)
	if cfg.ProfilePprof != "/tmp/x" {
		t.Errorf("--profile-pprof = %q", cfg.ProfilePprof)
	}
	if known := knownFlagNames(&config.Config{}); !known["--profile"] || !known["--profile-pprof"] {
		t.Error("the profile flags must be known to the unknown-flag check")
	}
}

// TestStartPprofWritesBothProfiles: the CPU profile runs for the build and the
// heap snapshot is taken after it.
func TestStartPprofWritesBothProfiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "prof")
	stop := startPprof(&config.Config{ProfilePprof: dir, Quiet: true})
	stop()
	for _, name := range []string{"cpu.prof", "heap.prof"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Size() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	// No directory asked for, nothing written, and the returned stop is safe.
	startPprof(&config.Config{Quiet: true})()
}

// TestStartPprofRefusesABadDirectory: an unusable target warns and leaves the
// build alone rather than stopping it.
func TestStartPprofRefusesABadDirectory(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "notadir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	startPprof(&config.Config{ProfilePprof: filepath.Join(file, "sub"), Quiet: true})()
}

// TestPprofFilesThatCannotBeWritten: a directory sitting where cpu.prof or
// heap.prof should be is a warning, and the build carries on.
func TestPprofFilesThatCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "cpu.prof"), 0o755); err != nil {
		t.Fatal(err)
	}
	startPprof(&config.Config{ProfilePprof: dir, Quiet: true})()

	heapOnly := t.TempDir()
	if err := os.Mkdir(filepath.Join(heapOnly, "heap.prof"), 0o755); err != nil {
		t.Fatal(err)
	}
	stop := startPprof(&config.Config{ProfilePprof: heapOnly, Quiet: true})
	stop()
	if _, err := os.Stat(filepath.Join(heapOnly, "cpu.prof")); err != nil {
		t.Errorf("the CPU profile should still have been written: %v", err)
	}
}

// TestEmitProfilePrintsTheTable: without --quiet the report reaches the person
// who asked for it.
func TestEmitProfilePrintsTheTable(t *testing.T) {
	t.Chdir(t.TempDir())
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	p := generator.NewProfile(generator.ProfileJSON)
	p.Add("Generating site", 5*time.Millisecond)
	p.Page("/a/", time.Millisecond)
	emitProfile(p, &config.Config{Profile: generator.ProfileJSON})
	_ = w.Close()
	os.Stdout = old
	out := make([]byte, 4096)
	n, _ := r.Read(out)
	if got := string(out[:n]); !strings.Contains(got, "Build profile") || !strings.Contains(got, "Generating site") {
		t.Errorf("report not printed: %q", got)
	}
}
