package dts

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// TestExtractGolden extracts the fixture package and compares the model with
// testdata/lib.golden.json; run with -update to rewrite it.
func TestExtractGolden(t *testing.T) {
	pkg, diags, err := Extract(apisource.Config{Root: filepath.Join("testdata", "lib")})
	if err != nil {
		t.Fatal(err)
	}
	api := &apimodel.API{Packages: []*apimodel.Package{pkg}}
	if probs := apimodel.Validate(api); len(probs) > 0 {
		t.Errorf("model problems: %v", probs)
	}
	got, err := apimodel.Encode(api)
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "lib.golden.json")
	if *update {
		if err := os.WriteFile(golden, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("model differs from %s; run go test -update and review the diff", golden)
	}
	wantDiags := []string{
		"broken.d.ts:0: line 2: unterminated comment",
		`index.d.ts:0: cannot find a declaration file for ./nowhere`,
		`index.d.ts:19: expected a name, found ":"`,
	}
	var gotDiags []string
	for _, d := range diags {
		gotDiags = append(gotDiags, d.String())
	}
	if strings.Join(gotDiags, "\n") != strings.Join(wantDiags, "\n") {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", strings.Join(gotDiags, "\n"), strings.Join(wantDiags, "\n"))
	}
}

// TestExtractDeterministic checks two runs write the same bytes.
func TestExtractDeterministic(t *testing.T) {
	var outs [][]byte
	for range 2 {
		pkg, _, err := Extract(apisource.Config{Root: filepath.Join("testdata", "lib"), Name: "renamed"})
		if err != nil {
			t.Fatal(err)
		}
		b, err := apimodel.Encode(&apimodel.API{Packages: []*apimodel.Package{pkg}})
		if err != nil {
			t.Fatal(err)
		}
		outs = append(outs, b)
	}
	if !bytes.Equal(outs[0], outs[1]) {
		t.Error("two runs differ")
	}
	if !bytes.Contains(outs[0], []byte(`"renamed/index#Lexer"`)) {
		t.Error("cfg.Name does not win over package.json")
	}
}
