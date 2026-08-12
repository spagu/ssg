package main

// Branch tests for ssg migrate (1.8.28): the default fetch seam, batch error
// paths, the live --http flow with an ephemeral port, and scaffold reporting.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spagu/ssg/internal/migrate"
)

// fakeProvider exercises the DEFAULT migrateFetch seam (which must delegate to
// the provider) without any real engine.
type fakeProvider struct{ err error }

func (fakeProvider) Name() string        { return "fake" }
func (fakeProvider) Version() string     { return "0.0.1" }
func (fakeProvider) Description() string { return "test double" }
func (f fakeProvider) Fetch(string, migrate.Options) (*migrate.Report, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &migrate.Report{Provider: "fake@0.0.1"}, nil
}

func TestMigrateFetchDefaultDelegates(t *testing.T) {
	rep, err := migrateFetch(fakeProvider{}, "https://e.com", migrate.Options{})
	if err != nil || rep.Provider != "fake@0.0.1" {
		t.Fatalf("default seam must call the provider: %v %v", rep, err)
	}
}

func TestMigrateBatchFetchError(t *testing.T) {
	t.Chdir(t.TempDir())
	stubMigrate(t, func(migrate.Options) (*migrate.Report, error) {
		return nil, errors.New("engine down")
	})
	if code := runMigrate([]string{"wordpress", "https://e.example.com", "--quiet"}); code != 1 {
		t.Fatalf("batch fetch error = %d, want 1", code)
	}
}

// TestMigrateBatchBuildError: content arrives but the project cannot build
// (broken template setting) — the failure is reported, not swallowed.
func TestMigrateBatchBuildError(t *testing.T) {
	t.Chdir(t.TempDir())
	yaml := "source: s\ntemplate: no-such-template\ndomain: e.example.com\n"
	if err := os.WriteFile(".ssg.yaml", []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	stubMigrate(t, func(migrate.Options) (*migrate.Report, error) {
		return &migrate.Report{Provider: "wordpress@test"}, nil
	})
	if code := runMigrate([]string{"wordpress", "https://e.example.com", "--quiet"}); code != 1 {
		t.Fatalf("unbuildable project = %d, want 1", code)
	}
}

// TestMigrateLiveHTTPOnly: --http without --watch serves on an ephemeral port,
// prints the live banner and builds once after the fetch (nothing else would).
func TestMigrateLiveHTTPOnly(t *testing.T) {
	t.Chdir(t.TempDir())
	yaml := "source: s\ntemplate: simple\ndomain: live.example.com\nport: 0\n"
	if err := os.WriteFile(".ssg.yaml", []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join("content", "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := `{"categories":[],"users":[],"media":[]}`
	if err := os.WriteFile(filepath.Join("content", "s", "metadata.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	blocked := stubMigrate(t, func(opts migrate.Options) (*migrate.Report, error) {
		post := "---\ntitle: T\nslug: t\nstatus: publish\ntype: post\ndate: 2024-01-01\n---\n\nX.\n"
		path := filepath.Join(opts.Dest, "posts", "t.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		return &migrate.Report{Provider: "wordpress@test", Posts: 1},
			os.WriteFile(path, []byte(post), 0o644)
	})
	if code := runMigrate([]string{"wordpress", "https://live.example.com", "--http"}); code != 0 {
		t.Fatalf("live http = %d", code)
	}
	if *blocked != 1 {
		t.Fatal("live mode must park after the report")
	}
	if _, err := os.Stat(filepath.Join("output", "index.html")); err != nil {
		t.Fatalf("http-only live mode must build once after the fetch: %v", err)
	}
}

// TestMigrateSourcelessConfig: a content_sources-only project has no primary
// source; migrate then derives the destination from the site host.
func TestMigrateSourcelessConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	yaml := "content_sources:\n  - path: docs\ntemplate: simple\ndomain: e.example.com\n"
	if err := os.WriteFile(".ssg.yaml", []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	var dest string
	stubMigrate(t, func(opts migrate.Options) (*migrate.Report, error) {
		dest = opts.Dest
		return &migrate.Report{Provider: "wordpress@test"}, nil
	})
	runMigrate([]string{"wordpress", "https://www.host.example.com", "--quiet"})
	if dest != filepath.Join("content", "host.example.com") {
		t.Fatalf("sourceless config must fall back to the host, got %q", dest)
	}
}

func TestReportScaffoldSkipped(t *testing.T) {
	reportScaffold([]string{"a"}, []string{"b"}) // both branches print, no panic
}

// TestMigrateScaffoldWriteError: an unwritable working directory fails the
// scaffold step with exit 1 instead of half-creating a project.
func TestMigrateScaffoldWriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	stubMigrate(t, func(migrate.Options) (*migrate.Report, error) {
		t.Fatal("fetch must not run when scaffolding failed")
		return nil, nil
	})
	if code := runMigrate([]string{"wordpress", "https://e.example.com", "--quiet"}); code != 1 {
		t.Fatalf("scaffold failure = %d, want 1", code)
	}
}
