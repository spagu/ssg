package main

// Wiring the MCP server to a project: which directories it may read media
// from, and which git binary it is allowed to run.
//
// These are small functions with security consequences. A media root that is
// wrong points the upload tools at somebody else's directory; a git binary
// resolved from a relative PATH entry is whatever happens to be in the working
// directory when the server starts.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/models"
)

// TestMediaRootsCoverEveryDirectoryTheBuildPublishes.
//
// A migrated site keeps its pictures in the content source's media/ and serves
// them at /media/, which is why the media tools were blind to them when they
// only knew about static/ (#218). Extra passthrough roots are served at the
// destination they are copied to, not at their path on disk.
func TestMediaRootsCoverEveryDirectoryTheBuildPublishes(t *testing.T) {
	cfg := &config.Config{
		StaticDir: "static", Source: "site", ContentDir: "content",
	}
	cfg.StaticSources = []models.StaticSource{
		{Path: "assets/brand", Dest: "/brand/"},
		{Path: "", Dest: "/ignored/"}, // a row with no path is not a root
	}
	roots := mediaRootsOf(cfg)

	byURL := map[string]string{}
	for _, r := range roots {
		byURL[r.URL] = r.Dir
	}
	if byURL["/"] != "static" {
		t.Errorf("static root = %q", byURL["/"])
	}
	if byURL["/media/"] != filepath.Join("content", "site", "media") {
		t.Errorf("content media root = %q", byURL["/media/"])
	}
	if byURL["/brand"] != "assets/brand" {
		t.Errorf("passthrough root = %q", byURL["/brand"])
	}
	for _, r := range roots {
		if r.Dir == "" {
			t.Errorf("a root with no directory was published: %+v", r)
		}
	}
}

// TestMediaRootsSkipTheContentSourceWhenThereIsNone: an MDDB-only project has
// no source directory, and joining an empty one would hand the tools the
// content root itself.
func TestMediaRootsSkipTheContentSourceWhenThereIsNone(t *testing.T) {
	roots := mediaRootsOf(&config.Config{StaticDir: "static", ContentDir: "content"})
	if len(roots) != 1 || roots[0].URL != "/" {
		t.Errorf("roots = %+v", roots)
	}
}

// TestGitRunnerRefusesARelativeGit.
//
// PATH can contain a relative entry, and `git` resolved through one is
// whatever sits in the directory the server happened to start in. The runner
// has to refuse rather than execute it, and the refusal has to say why.
func TestGitRunnerRefusesARelativeGit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	fake := filepath.Join(dir, "git")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho hello\n"), 0o700); err != nil { // #nosec G306 -- a test's fake binary
		t.Fatal(err)
	}
	t.Setenv("PATH", ".")

	run := gitRunner()
	if run == nil {
		t.Fatal("gitRunner must always return a runner")
	}
	out, err := run("status")
	if err == nil {
		t.Fatalf("a relative git must be refused, got %q", out)
	}
	if !strings.Contains(err.Error(), "git") {
		t.Errorf("error = %v", err)
	}
}

// TestGitRunnerRefusesAMissingGit the same way, so a project without git gets
// an explanation rather than an exec error from three layers down.
func TestGitRunnerRefusesAMissingGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := gitRunner()("status"); err == nil {
		t.Error("no git on PATH must be refused")
	}
}

// TestTheConfigValidatorRejectsAnEditThatWouldNotLoad.
//
// designer_config_set writes through this: an edit that leaves the file
// unparseable is rolled back, and the only thing that knows whether it would
// load is a real load.
func TestTheConfigValidatorRejectsAnEditThatWouldNotLoad(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.yaml")
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(good, []byte("domain: example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("domain: [unclosed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	validate := func(path string) error {
		_, err := loadConfigFile(path)
		return err
	}
	if err := validate(good); err != nil {
		t.Errorf("a valid config was rejected: %v", err)
	}
	if err := validate(bad); err == nil {
		t.Error("a config that does not parse must be rejected")
	}
}
