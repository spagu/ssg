package python

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// update rewrites the golden files: go test ./internal/apisource/python -update
var update = flag.Bool("update", false, "rewrite golden files")

// extractFixture runs Extract over testdata/textkit.
func extractFixture(t *testing.T) (*apimodel.Package, []apisource.Diagnostic) {
	t.Helper()
	pkg, diags, err := Extract(apisource.Config{Root: filepath.Join("testdata", "textkit")})
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
	pkg, _ := extractFixture(t)
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

func TestExtractValidates(t *testing.T) {
	pkg, _ := extractFixture(t)
	api := &apimodel.API{Schema: apimodel.Schema, Packages: []*apimodel.Package{pkg}}
	if probs := apimodel.Validate(api); len(probs) > 0 {
		t.Errorf("Validate: %v", probs)
	}
}

func TestExtractDeterministic(t *testing.T) {
	first, d1 := extractFixture(t)
	for range 5 {
		next, d2 := extractFixture(t)
		if !bytes.Equal(encode(t, first), encode(t, next)) {
			t.Fatal("two runs gave different models")
		}
		if len(d1) != len(d2) {
			t.Fatal("two runs gave different diagnostics")
		}
	}
}
