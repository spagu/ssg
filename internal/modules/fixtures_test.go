package modules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestSharedFixtures: the Go reader reports exactly what the TypeScript one
// does for every fixture in testdata/manifests (workers/module-fixtures.test.ts
// holds the TypeScript side to the same files).
func TestSharedFixtures(t *testing.T) {
	schema := mustSchema(t)
	files, err := filepath.Glob(filepath.Join("testdata", "manifests", "*.json"))
	if err != nil || len(files) < 20 {
		t.Fatalf("fixtures: %d (%v)", len(files), err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file) // #nosec G304 -- test fixture
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				Manifest json.RawMessage `json:"manifest"`
				Errors   []string        `json:"errors"`
			}
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatal(err)
			}
			m, errs := schema.ReadManifest(fixture.Manifest)
			if !slices.Equal(errs, fixture.Errors) {
				t.Errorf("errors differ from the TypeScript reader's\n got: %q\nwant: %q", errs, fixture.Errors)
			}
			valid := strings.HasPrefix(filepath.Base(file), "valid-")
			if valid != (m != nil) {
				t.Errorf("manifest returned = %v, want %v", m != nil, valid)
			}
		})
	}
}

// TestEveryShippedTemplateIsValid: the manifests the binary embeds pass the
// Go reader, as workers/modules.test.ts checks they pass the TypeScript one.
func TestEveryShippedTemplateIsValid(t *testing.T) {
	schema := mustSchema(t)
	dirs, _ := filepath.Glob(filepath.Join("..", "..", "workers", "*", "module.json"))
	if len(dirs) < 12 {
		t.Fatalf("only %d template manifests", len(dirs))
	}
	for _, file := range dirs {
		data, err := os.ReadFile(file) // #nosec G304 -- repository file
		if err != nil {
			t.Fatal(err)
		}
		if m, errs := schema.ReadManifest(data); m == nil {
			t.Errorf("%s: %q", file, errs)
		} else if m.ID != filepath.Base(filepath.Dir(file)) {
			t.Errorf("%s: id %q", file, m.ID)
		}
	}
}

func mustSchema(t *testing.T) *Schema {
	t.Helper()
	s, err := EmbeddedSchema()
	if err != nil {
		t.Fatal(err)
	}
	return s
}
