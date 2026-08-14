package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/migrate"
)

// stubMigrate replaces the provider call and the live-mode park for one test.
func stubMigrate(t *testing.T, fetch func(opts migrate.Options) (*migrate.Report, error)) *int {
	t.Helper()
	blocked := 0
	oldFetch, oldBlock := migrateFetch, migrateBlock
	migrateFetch = func(_ migrate.Provider, _ string, opts migrate.Options) (*migrate.Report, error) {
		return fetch(opts)
	}
	migrateBlock = func() { blocked++ }
	t.Cleanup(func() { migrateFetch, migrateBlock = oldFetch, oldBlock })
	return &blocked
}

func TestRunMigrateUsagePaths(t *testing.T) {
	if code := runMigrate(nil); code != 2 {
		t.Fatalf("bare migrate = %d, want 2", code)
	}
	if code := runMigrate([]string{"--help"}); code != 0 {
		t.Fatalf("--help = %d, want 0", code)
	}
	if code := runMigrate([]string{"--list"}); code != 0 {
		t.Fatalf("--list = %d, want 0", code)
	}
	if code := runMigrate([]string{"drupal", "https://x.com"}); code != 2 {
		t.Fatalf("unknown provider = %d, want 2", code)
	}
	if code := runMigrate([]string{"wordpress"}); code != 2 {
		t.Fatalf("missing URL = %d, want 2", code)
	}
	if code := runMigrate([]string{"wordpress", "not-a-url"}); code != 2 {
		t.Fatalf("bad URL = %d, want 2", code)
	}
	if code := runMigrate([]string{"wordpress", "https://x.com", "--bogus"}); code != 2 {
		t.Fatalf("unknown flag = %d, want 2", code)
	}
}

// TestParseMigrateFlags_AllMedia: the flag that opts back into the whole media
// library (#130).
func TestParseMigrateFlags_AllMedia(t *testing.T) {
	f, code := parseMigrateFlags([]string{"--all-media"})
	if code != -1 || !f.allMedia {
		t.Errorf("--all-media not parsed: %+v code=%d", f, code)
	}
	if f, _ := parseMigrateFlags(nil); f.allMedia {
		t.Error("the default must stay lean")
	}
}

func TestParseMigrateFlags(t *testing.T) {
	f, code := parseMigrateFlags([]string{
		"--content", "pages,posts", "--watch", "--http", "--source=shop", "-q"})
	if code != -1 {
		t.Fatalf("code = %d", code)
	}
	if !f.watch || !f.http || !f.quiet || f.source != "shop" {
		t.Fatalf("flags = %+v", f)
	}
	if len(f.content) != 2 || f.content[0] != "pages" || f.content[1] != "posts" {
		t.Fatalf("content = %v", f.content)
	}
	// = form and space form are equivalent.
	f2, _ := parseMigrateFlags([]string{"--content=pages, posts ", "--source", "shop"})
	if len(f2.content) != 2 || f2.source != "shop" {
		t.Fatalf("= form: %+v", f2)
	}
	// Path-traversal source names are refused.
	if _, code := parseMigrateFlags([]string{"--source=../evil"}); code != 2 {
		t.Fatal("traversal source must be refused")
	}
	if _, code := parseMigrateFlags([]string{"--source=a/b"}); code != 2 {
		t.Fatal("slashed source must be refused")
	}
}

func TestMigrateScaffoldSkipsSamples(t *testing.T) {
	files := migrateScaffold("shop", "shop.example.com")
	for _, f := range files {
		if strings.HasSuffix(f.path, "index.md") || strings.HasSuffix(f.path, "hello-world.md") {
			t.Fatalf("sample content must not be scaffolded for a migration: %s", f.path)
		}
	}
	var hasConfig, hasMetadata bool
	for _, f := range files {
		hasConfig = hasConfig || f.path == ".ssg.yaml"
		hasMetadata = hasMetadata || strings.HasSuffix(f.path, "metadata.json")
	}
	if !hasConfig || !hasMetadata {
		t.Fatalf("config/metadata stub missing: %v", files)
	}
}

// TestMigrateBatchEndToEnd drives the whole command: scaffold into an empty
// dir, "fetch" via the stub (writing a post the way wpexporter would), build,
// report. The migrated project must build without errors — GO-100's core
// acceptance criterion.
func TestMigrateBatchEndToEnd(t *testing.T) {
	t.Chdir(t.TempDir())
	blocked := stubMigrate(t, func(opts migrate.Options) (*migrate.Report, error) {
		post := "---\ntitle: Hello\nslug: hello\nstatus: publish\ntype: post\ndate: 2024-01-01\n---\n\nMigrated.\n"
		path := filepath.Join(opts.Dest, "posts", "news", "hello.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(post), 0o644); err != nil {
			return nil, err
		}
		return &migrate.Report{Provider: "wordpress@test", Posts: 1}, nil
	})

	code := runMigrate([]string{"wordpress", "https://www.shop.example.com", "--quiet"})
	if code != 0 {
		t.Fatalf("batch migrate = %d", code)
	}
	// www. is stripped for the source name; the scaffolded config points there.
	if _, err := os.Stat(filepath.Join("content", "shop.example.com", "posts", "news", "hello.md")); err != nil {
		t.Fatalf("content not where the scaffold points: %v", err)
	}
	if _, err := os.Stat(".ssg.yaml"); err != nil {
		t.Fatalf("scaffold config missing: %v", err)
	}
	if _, err := os.Stat("output"); err != nil {
		t.Fatalf("batch mode must build the site: %v", err)
	}
	if *blocked != 0 {
		t.Fatal("batch mode must not park the process")
	}
}

// TestMigrateLiveWatch: live mode parks after the report and keeps serving;
// fetch errors also park (the partial site stays browsable) but exit 1.
func TestMigrateLiveWatch(t *testing.T) {
	t.Chdir(t.TempDir())
	blocked := stubMigrate(t, func(opts migrate.Options) (*migrate.Report, error) {
		return &migrate.Report{Provider: "wordpress@test"}, nil
	})
	code := runMigrate([]string{"wordpress", "https://live.example.com", "--watch", "--quiet"})
	if code != 0 || *blocked != 1 {
		t.Fatalf("live = %d, parked %d times", code, *blocked)
	}
}

func TestMigrateLiveFetchErrorKeepsServing(t *testing.T) {
	t.Chdir(t.TempDir())
	blocked := stubMigrate(t, func(opts migrate.Options) (*migrate.Report, error) {
		return nil, errors.New("engine exploded")
	})
	code := runMigrate([]string{"wordpress", "https://live.example.com", "--watch", "--quiet"})
	if code != 1 || *blocked != 1 {
		t.Fatalf("live error = %d, parked %d times", code, *blocked)
	}
}

// TestMigrateExistingProject: with a config already on disk nothing is
// scaffolded and the config's own source directory receives the export.
func TestMigrateExistingProject(t *testing.T) {
	t.Chdir(t.TempDir())
	yaml := "source: mysite\ntemplate: simple\ndomain: my.example.com\n"
	if err := os.WriteFile(".ssg.yaml", []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join("content", "mysite"), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := `{"categories":[],"users":[],"media":[]}`
	if err := os.WriteFile(filepath.Join("content", "mysite", "metadata.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	var gotDest string
	stubMigrate(t, func(opts migrate.Options) (*migrate.Report, error) {
		gotDest = opts.Dest
		return &migrate.Report{Provider: "wordpress@test"}, nil
	})
	if code := runMigrate([]string{"wordpress", "https://other.example.com", "--quiet"}); code != 0 {
		t.Fatalf("existing project = %d", code)
	}
	if gotDest != filepath.Join("content", "mysite") {
		t.Fatalf("export must land in the config's source, got %q", gotDest)
	}
}

func TestMigrateDispatch(t *testing.T) {
	// The verb is claimed even with no further args (usage, exit 2) — the old
	// positional fallthrough must never resurface.
	code, handled := dispatchSingleVerb([]string{"migrate"})
	if !handled || code != 2 {
		t.Fatalf("dispatch bare migrate = %d, %v", code, handled)
	}
	code, handled = dispatchSingleVerb([]string{"migrate", "--list"})
	if !handled || code != 0 {
		t.Fatalf("dispatch migrate --list = %d, %v", code, handled)
	}
}

// TestEngineFlagHint: wpexporter's own --no-<kind> flags are an easy thing to
// paste into `ssg migrate` — they sit one command apart — and a bare "unknown
// flag" left the operator guessing why their custom types never arrived (#134).
func TestEngineFlagHint(t *testing.T) {
	for flag, kind := range map[string]string{
		"--no-custom-types": "custom",
		"--no-comments":     "comments",
		"--no-media=true":   "media",
	} {
		hint := engineFlagHint(flag)
		if hint == "" {
			t.Fatalf("%s must be explained, not just rejected", flag)
		}
		if !strings.Contains(hint, "--content") || !strings.Contains(hint, kind) {
			t.Fatalf("%s hint must name --content and %s, got %q", flag, kind, hint)
		}
	}

	if hint := engineFlagHint("--nonsense"); hint != "" {
		t.Fatalf("an unrelated flag needs no engine hint, got %q", hint)
	}
}

// TestMigrateServerFlags: live mode is a server, so it accepts the server's
// address flags. `--watch --http --port 8889` used to die on "unknown flag"
// after the project had already been scaffolded (#135).
func TestMigrateServerFlags(t *testing.T) {
	f, code := parseMigrateFlags([]string{"--watch", "--http", "--port", "8889", "--host", "0.0.0.0"})
	if code >= 0 {
		t.Fatalf("server flags must be accepted, got exit %d", code)
	}
	if f.port != 8889 || f.host != "0.0.0.0" {
		t.Fatalf("parsed = %d/%q", f.port, f.host)
	}

	if f, code = parseMigrateFlags([]string{"--port=9000"}); code >= 0 || f.port != 9000 {
		t.Fatalf("--port=N form: %d, %d", f.port, code)
	}

	// A typo must be reported, never rounded down to the default port.
	if _, code = parseMigrateFlags([]string{"--port", "eight"}); code != 2 {
		t.Fatalf("invalid port = %d, want 2", code)
	}
	if _, code = parseMigrateFlags([]string{"--port", "70000"}); code != 2 {
		t.Fatalf("out-of-range port = %d, want 2", code)
	}
}
