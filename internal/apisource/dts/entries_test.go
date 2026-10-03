package dts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apisource"
)

// writeTree writes files (path → content) under a new temporary directory.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// openRoot opens dir as an os.Root closed with the test.
func openRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// TestExportTypes checks the shapes an exports field can take.
func TestExportTypes(t *testing.T) {
	cases := map[string][]string{
		`"./index.js"`: nil,
		`{"types": "./a.d.ts", "default": "./a.js"}`:                                                                            {"./a.d.ts"},
		`{"import": {"types": "./a.d.mts"}, "require": {"types": "./a.d.cts"}}`:                                                 {"./a.d.mts"},
		`{"./b": {"types": "./b.d.ts"}, ".": [{"node": {"types": "./m.d.ts"}}], "./*": {"types": "./*.d.ts"}, "./c": "./c.js"}`: {"./m.d.ts", "./b.d.ts"},
		`[{"types": "./x.d.ts"}]`: {"./x.d.ts"},
		`{"browser": "./x.js"}`:   nil,
		`{`:                       nil,
	}
	for raw, want := range cases {
		if got := exportTypes(json.RawMessage(raw)); !reflect.DeepEqual(got, want) {
			t.Errorf("exportTypes(%s) = %v, want %v", raw, got, want)
		}
	}
	if exportTypes(nil) != nil {
		t.Error("no exports field gave entries")
	}
}

// TestManifestEntries checks types, typings and exports join without
// duplicates or paths outside the root.
func TestManifestEntries(t *testing.T) {
	pj := packageJSON{Typings: "./index.d.ts", Exports: json.RawMessage(`{".": {"types": "index.d.ts"}, "./x": {"types": "../x.d.ts"}}`)}
	if got := manifestEntries(pj); !reflect.DeepEqual(got, []string{"index.d.ts"}) {
		t.Errorf("manifestEntries = %v", got)
	}
	if cleanRel(".") != "" || cleanRel("/abs") != "" || cleanRel(`a\b.d.ts`) != "a/b.d.ts" {
		t.Error("cleanRel")
	}
}

// TestDiscoverEntries checks configured entries, the manifest and the
// fallbacks, in that order.
func TestDiscoverEntries(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"lib/main.d.ts": "", "index.d.ts": "", "src/a.d.mts": "",
	})
	r := openRoot(t, dir)
	cfg := apisource.Config{Root: dir, Entries: []string{"src/a.mjs", "lib/main", "nope.ts", "../out.d.ts"}}
	got, diags := discoverEntries(r, cfg, packageJSON{})
	if !reflect.DeepEqual(got, []string{"src/a.d.mts", "lib/main.d.ts"}) || len(diags) != 2 {
		t.Errorf("configured: %v %v", got, diags)
	}
	got, _ = discoverEntries(r, apisource.Config{}, packageJSON{Types: "lib/main.js"})
	if !reflect.DeepEqual(got, []string{"lib/main.d.ts"}) {
		t.Errorf("types: %v", got)
	}
	got, _ = discoverEntries(r, apisource.Config{}, packageJSON{Types: "gone.d.ts"})
	if !reflect.DeepEqual(got, []string{"gone.d.ts"}) {
		t.Errorf("missing types file kept for its diagnostic: %v", got)
	}
	got, _ = discoverEntries(r, apisource.Config{}, packageJSON{Main: "lib/main.js"})
	if !reflect.DeepEqual(got, []string{"lib/main.d.ts"}) {
		t.Errorf("main: %v", got)
	}
	got, _ = discoverEntries(r, apisource.Config{}, packageJSON{})
	if !reflect.DeepEqual(got, []string{"index.d.ts"}) {
		t.Errorf("index: %v", got)
	}
	empty := openRoot(t, t.TempDir())
	if got, _ := discoverEntries(empty, apisource.Config{}, packageJSON{}); got != nil {
		t.Errorf("empty: %v", got)
	}
}

// TestDeclarationCandidates checks how a path maps to declaration files.
func TestDeclarationCandidates(t *testing.T) {
	cases := map[string][]string{
		"a.d.cts": {"a.d.cts"},
		"a.cjs":   {"a.d.cts"},
		"a.tsx":   {"a.d.ts"},
		"a":       {"a.d.ts", "a.d.mts", "a.d.cts", "a/index.d.ts"},
	}
	for in, want := range cases {
		if got := declarationCandidates(in); !reflect.DeepEqual(got, want) {
			t.Errorf("declarationCandidates(%q) = %v", in, got)
		}
	}
	if relativeTarget("a/b.d.ts", "pkg") != "" || relativeTarget("a.d.ts", "../x") != "" || relativeTarget("a/b.d.ts", "../c") != "c" {
		t.Error("relativeTarget")
	}
}

// TestHasDeclarations checks each way a package can ship declarations.
func TestHasDeclarations(t *testing.T) {
	cases := []struct {
		files map[string]string
		want  bool
	}{
		{map[string]string{"package.json": `{"types": "x.d.ts"}`, "x.d.ts": ""}, true},
		{map[string]string{"package.json": `{"exports": {"types": "./x.d.ts"}}`, "x.d.ts": ""}, true},
		// Named but not built yet: the JavaScript is what can be read.
		{map[string]string{"package.json": `{"types": "dist/x.d.ts"}`}, false},
		{map[string]string{"package.json": `{"exports": {"types": "./x.d.ts"}}`}, false},
		{map[string]string{"package.json": `{"main": "dist/x.js"}`, "dist/x.d.ts": ""}, true},
		{map[string]string{"index.d.ts": ""}, true},
		{map[string]string{"package.json": `{"main": "x.js"}`, "x.js": ""}, false},
		{map[string]string{"package.json": `{`}, false},
	}
	for i, c := range cases {
		if got := HasDeclarations(writeTree(t, c.files)); got != c.want {
			t.Errorf("case %d: HasDeclarations = %v, want %v", i, got, c.want)
		}
	}
	if HasDeclarations(filepath.Join(t.TempDir(), "missing")) {
		t.Error("a missing directory has declarations")
	}
}

// TestReadPackageJSON checks a missing, malformed and unreadable manifest.
func TestReadPackageJSON(t *testing.T) {
	if pj, err := readPackageJSON(openRoot(t, t.TempDir())); err != nil || pj.Name != "" {
		t.Errorf("missing: %+v %v", pj, err)
	}
	if _, err := readPackageJSON(openRoot(t, writeTree(t, map[string]string{"package.json": "{"}))); err == nil {
		t.Error("malformed manifest read")
	}
	dir := writeTree(t, map[string]string{"package.json/x": ""})
	if _, err := readPackageJSON(openRoot(t, dir)); err == nil || strings.Contains(err.Error(), "no such") {
		t.Errorf("a directory named package.json: %v", err)
	}
}
