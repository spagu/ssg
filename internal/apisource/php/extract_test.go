package php

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

// update rewrites the golden file: go test ./internal/apisource/php -update
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
	out, err := apimodel.Encode(&apimodel.API{Schema: apimodel.Schema, Packages: []*apimodel.Package{pkg}})
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

func TestExtractValidatesAndIsDeterministic(t *testing.T) {
	pkg, diags := extractFixture(t)
	api := &apimodel.API{Schema: apimodel.Schema, Packages: []*apimodel.Package{pkg}}
	if probs := apimodel.Validate(api); len(probs) > 0 {
		t.Errorf("Validate: %v", probs)
	}
	if len(diags) > 0 {
		t.Errorf("diagnostics: %v", diags)
	}
	first := encode(t, pkg)
	for range 3 {
		again, _ := extractFixture(t)
		if !bytes.Equal(encode(t, again), first) {
			t.Fatal("two extractions differ")
		}
	}
}

func TestExtractPackage(t *testing.T) {
	pkg, _ := extractFixture(t)
	if pkg.Name != "textkit" || pkg.Version != "2.1.0" || pkg.Language != "php" || !strings.HasPrefix(pkg.Readme, "# textkit") {
		t.Errorf("package = %q %q %q %q", pkg.Name, pkg.Version, pkg.Language, pkg.Readme)
	}
	var paths []string
	for _, m := range pkg.Modules {
		paths = append(paths, m.Path)
	}
	want := "Acme/Textkit Acme/Textkit/Lexer Acme/Textkit/Token global"
	if got := strings.Join(paths, " "); got != want {
		t.Errorf("modules = %q, want %q", got, want)
	}
}

func TestExtractBroken(t *testing.T) {
	pkg, diags, err := Extract(apisource.Config{Root: filepath.Join("testdata", "broken"), Name: "broken"})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	var got []string
	for _, d := range diags {
		if d.Severity != apisource.Error {
			t.Errorf("severity of %v = %s", d, d.Severity)
		}
		got = append(got, d.String())
	}
	want := []string{
		"a_string.php:6: unterminated string '",
		"b_braces.php:5: unbalanced {: never closed",
		"d_heredoc.php:2: unterminated heredoc EOT",
		"e_comment.php:2: unterminated comment",
		"f_closer.php:2: unbalanced ): { opened on line 2",
		"g_extra.php:2: unbalanced }: nothing to close",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(pkg.Modules) != 1 || len(pkg.Modules[0].Symbols) != 1 || pkg.Modules[0].Symbols[0].Name != "good" {
		t.Errorf("modules = %+v", pkg.Modules)
	}
}
