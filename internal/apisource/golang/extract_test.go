package golang

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// update rewrites the golden files: go test ./internal/apisource/golang -update
var update = flag.Bool("update", false, "rewrite golden files")

// fixture is the module most tests read.
var fixture = filepath.Join("testdata", "textkit")

// extract runs Extract over a root, failing the test on an error.
func extract(t *testing.T, cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic) {
	t.Helper()
	pkg, diags, err := Extract(cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return pkg, diags
}

// encode renders a package as api.json.
func encode(t *testing.T, pkg *apimodel.Package) []byte {
	t.Helper()
	out, err := apimodel.Encode(&apimodel.API{Packages: []*apimodel.Package{pkg}})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return out
}

func TestExtractGolden(t *testing.T) {
	pkg, _ := extract(t, apisource.Config{Root: fixture})
	got := encode(t, pkg)
	golden := filepath.Join("testdata", "textkit.golden.json")
	if *update {
		if err := os.WriteFile(golden, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden) // #nosec G304 -- fixed test path
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output differs from %s; run with -update and review the diff", golden)
	}
}

func TestExtractValidatesAndRoundTrips(t *testing.T) {
	pkg, _ := extract(t, apisource.Config{Root: fixture})
	api := &apimodel.API{Schema: apimodel.Schema, Packages: []*apimodel.Package{pkg}}
	if probs := apimodel.Validate(api); len(probs) > 0 {
		t.Errorf("Validate: %v", probs)
	}
	first := encode(t, pkg)
	back, err := apimodel.Decode(first)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	again, err := apimodel.Encode(back)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.Equal(first, again) {
		t.Error("Encode → Decode → Encode changed the document")
	}
}

func TestExtractDeterministic(t *testing.T) {
	first, d1 := extract(t, apisource.Config{Root: fixture})
	for range 3 {
		next, d2 := extract(t, apisource.Config{Root: fixture})
		if !bytes.Equal(encode(t, first), encode(t, next)) {
			t.Fatal("two runs gave different models")
		}
		if len(d1) != len(d2) {
			t.Fatal("two runs gave different diagnostics")
		}
	}
}

func TestExtractPackage(t *testing.T) {
	pkg, diags := extract(t, apisource.Config{Root: fixture})
	if pkg.Name != "textkit" || pkg.Language != "go" || pkg.Version != "" || pkg.Readme == "" {
		t.Errorf("package = %q %q %q readme %d bytes", pkg.Name, pkg.Language, pkg.Version, len(pkg.Readme))
	}
	var ids []string
	for _, m := range pkg.Modules {
		ids = append(ids, m.ID+"="+m.Path)
	}
	want := []string{"textkit/internal/wrap=internal/wrap", "textkit/lexer=lexer", "textkit/textkit=textkit"}
	if !slices.Equal(ids, want) {
		t.Errorf("modules = %v, want %v", ids, want)
	}
	if len(diags) != 1 || diags[0].File != "lexer/stray.go" || diags[0].Severity != apisource.Warning {
		t.Errorf("diagnostics = %v, want the stray package warning", diags)
	}
}

func TestExtractInternal(t *testing.T) {
	pkg, _ := extract(t, apisource.Config{Root: fixture})
	ix := apimodel.NewIndex(&apimodel.API{Packages: []*apimodel.Package{pkg}})
	for id, internal := range map[string]bool{
		"textkit/internal/wrap#Wrap":  true,
		"textkit/internal/wrap#Width": true,
		"textkit/lexer#Token":         false,
	} {
		s, ok := ix.Symbol(id)
		if !ok || s.Flags.Internal != internal {
			t.Errorf("%s: found %v, internal = %v, want %v", id, ok, s != nil && s.Flags.Internal, internal)
		}
	}
}

func TestExtractConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     apisource.Config
		modules []string
	}{
		{"name", apisource.Config{Root: fixture, Name: "tk", Entries: []string{"./lexer"}}, []string{"tk/lexer"}},
		{"root entry", apisource.Config{Root: fixture, Entries: []string{"."}}, []string{"textkit/textkit"}},
		{"exclude", apisource.Config{Root: fixture, Exclude: []string{"internal/**", "lexer/*"}}, []string{"textkit/textkit"}},
		{"include", apisource.Config{Root: fixture, Include: []string{"lexer/*.go"}}, []string{"textkit/lexer"}},
		{"no go.mod", apisource.Config{Root: filepath.Join("testdata", "nomod")}, []string{"nomod/nomod"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, _ := extract(t, tt.cfg)
			var ids []string
			for _, m := range pkg.Modules {
				ids = append(ids, m.ID)
			}
			if !slices.Equal(ids, tt.modules) {
				t.Errorf("modules = %v, want %v", ids, tt.modules)
			}
		})
	}
}

func TestExtractBroken(t *testing.T) {
	pkg, diags := extract(t, apisource.Config{Root: filepath.Join("testdata", "broken")})
	if len(pkg.Modules) != 1 || len(pkg.Modules[0].Symbols) != 1 {
		t.Fatalf("modules = %+v, want broken with OK", pkg.Modules)
	}
	if len(diags) != 1 || diags[0].File != "bad.go" || diags[0].Line != 3 || diags[0].Severity != apisource.Error {
		t.Errorf("diagnostics = %v, want bad.go:3 error", diags)
	}
}

func TestExtractErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  apisource.Config
	}{
		{"no root", apisource.Config{}},
		{"missing root", apisource.Config{Root: filepath.Join("testdata", "missing")}},
		{"no package", apisource.Config{Root: filepath.Join(fixture, "cmd")}},
		{"missing entry", apisource.Config{Root: fixture, Entries: []string{"nowhere"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := Extract(tt.cfg); err == nil {
				t.Error("Extract succeeded, want an error")
			}
		})
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		root string
		want bool
	}{
		{fixture, true},
		{filepath.Join("testdata", "nomod"), true},
		{filepath.Join(fixture, "cmd"), false},
		{filepath.Join("testdata", "missing"), false},
	}
	for _, tt := range tests {
		if got := Detect(tt.root); got != tt.want {
			t.Errorf("Detect(%s) = %v, want %v", tt.root, got, tt.want)
		}
	}
}
