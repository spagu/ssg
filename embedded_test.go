package ssg

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmbeddedWorkersCoverEveryWorker fails when a worker is added under
// workers/ and not added to the go:embed list in embedded.go.
//
// The list is enumerated rather than written as `all:workers` because a worker
// with a test suite has node_modules beside its source, and that swept 550 MB
// of test tooling into the binary. The cost of enumerating is that a new worker
// can be forgotten, which is what this test is for.
func TestEmbeddedWorkersCoverEveryWorker(t *testing.T) {
	entries, err := os.ReadDir("workers")
	if err != nil {
		t.Fatalf("reading workers/: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		embedded, err := EmbeddedWorkers.ReadDir("workers/" + name)
		if err != nil || len(embedded) == 0 {
			t.Errorf("worker %q is on disk but not embedded — add it to the go:embed list in embedded.go", name)
		}
	}
}

// TestEmbeddedWorkersExcludeTooling keeps the binary from growing by half a
// gigabyte the next time someone runs `npm install` in a worker directory.
func TestEmbeddedWorkersExcludeTooling(t *testing.T) {
	unwanted := []string{"node_modules", "coverage", ".wrangler"}
	err := fs.WalkDir(EmbeddedWorkers, "workers", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		for _, bad := range unwanted {
			if d.Name() == bad {
				t.Errorf("%s is embedded in the binary; it is tooling, not a worker's source", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the embedded workers: %v", err)
	}
}

// TestEmbeddedWorkersCarryTheirSource spot-checks that the files a scaffolded
// worker cannot run without are present — in particular the underscore-prefixed
// modules go:embed drops without `all:`.
func TestEmbeddedWorkersCarryTheirSource(t *testing.T) {
	for _, path := range []string{
		"workers/comments/functions/api/comments/_lib.ts",
		"workers/ecommerce/functions/api/shop/_schema.ts",
		"workers/ecommerce/functions/api/shop/schema.sql",
		"workers/ecommerce/public/ecommerce-admin/index.html",
		"workers/ecommerce/wrangler.snippet.toml",
		"workers/rate-limit/functions/_middleware.ts",
	} {
		if _, err := EmbeddedWorkers.ReadFile(path); err != nil {
			t.Errorf("%s is missing from the binary: %v", path, err)
		}
	}
}

// TestEmbeddedThemesCarryTheirTemplates does the same for the starter themes,
// which a first run scaffolds when the requested theme is not on disk.
func TestEmbeddedThemesCarryTheirTemplates(t *testing.T) {
	for _, theme := range []string{"simple", "krowy"} {
		entries, err := EmbeddedThemes.ReadDir(filepath.Join("templates", theme))
		if err != nil || len(entries) == 0 {
			t.Fatalf("theme %q is not embedded: %v", theme, err)
		}
		var hasIndex bool
		for _, e := range entries {
			if strings.EqualFold(e.Name(), "index.html") {
				hasIndex = true
			}
		}
		if !hasIndex {
			t.Errorf("theme %q has no index.html in the binary", theme)
		}
	}
}
