package dts

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apisource"
)

// TestExtractErrors checks the cases where nothing can be extracted.
func TestExtractErrors(t *testing.T) {
	if _, _, err := Extract(apisource.Config{Root: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Error("a missing root extracted")
	}
	if _, _, err := Extract(apisource.Config{Root: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "no declaration files") {
		t.Errorf("an empty root: %v", err)
	}
	root := writeTree(t, map[string]string{"package.json": `{"types": "gone.d.ts"}`})
	_, diags, err := Extract(apisource.Config{Root: root})
	if err == nil || len(diags) != 1 || !strings.Contains(diags[0].Message, "cannot read") {
		t.Errorf("an unreadable entry: %v %v", err, diags)
	}
}

// TestExtractWarnings checks a malformed manifest and source map, a
// missing README and the root's base name as the package name.
func TestExtractWarnings(t *testing.T) {
	root := writeTree(t, map[string]string{
		"package.json":   "{",
		"index.d.ts":     "export declare const a: number;\nexport * from \"other-package\";",
		"index.d.ts.map": "{",
	})
	pkg, diags, err := Extract(apisource.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Name != filepath.Base(root) || pkg.Readme != "" || pkg.Version != "" {
		t.Errorf("package = %+v", pkg)
	}
	if len(diags) != 2 || diags[0].File != "index.d.ts.map" || diags[1].File != "package.json" {
		t.Errorf("diagnostics: %v", diags)
	}
	if src := pkg.Modules[0].Symbols[0].Source; src.File != "index.d.ts" || src.Line != 1 {
		t.Errorf("source = %+v", src)
	}
}

// TestIncludeExclude checks the configured globs decide which files are
// read; a re-export from a file left out lists nothing.
func TestIncludeExclude(t *testing.T) {
	files := map[string]string{
		"index.d.ts":          "export * from \"./src/a\";\nexport * from \"./src/internal/b\";",
		"src/a.d.ts":          "export declare const a: number;",
		"src/internal/b.d.ts": "export declare const b: number;",
	}
	x := extractFiles(t, files, apisource.Config{Exclude: []string{"**/internal/**"}})
	x.sym("p/index#a")
	x.missing("p/index#b")
	x = extractFiles(t, files, apisource.Config{Include: []string{"index.d.ts", "src/*.d.ts"}})
	x.sym("p/index#a")
	x.missing("p/index#b")
	x = extractFiles(t, files, apisource.Config{Include: []string{"./**/*.d.ts"}})
	x.sym("p/index#b")
}

// TestMatchAny checks the glob forms.
func TestMatchAny(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"**", "a/b.d.ts", true},
		{"a/**/c.d.ts", "a/c.d.ts", true},
		{"a/**/c.d.ts", "a/x/y/c.d.ts", true},
		{"a/*.d.ts", "a/x/c.d.ts", false},
		{"a/b", "a", false},
		{`a\[`, "a/[", false},
	}
	for _, c := range cases {
		if got := matchAny([]string{c.glob}, c.path); got != c.want {
			t.Errorf("matchAny(%q, %q) = %v", c.glob, c.path, got)
		}
	}
}

// TestCycles checks star exports and file namespaces that refer back to
// themselves end.
func TestCycles(t *testing.T) {
	x := extractFiles(t, map[string]string{
		"index.d.ts": "export * from \"./a\";\nexport * as self from \"./index\";\nexport declare const top: number;",
		"a.d.ts":     "export * from \"./index\";\nexport declare const a: number;\nexport * as default from \"./a\";",
	}, apisource.Config{Entries: []string{"index.d.ts", "a.d.ts"}})
	x.sym("p/index#a")
	x.sym("p/index#self.top")
	if s := x.sym("p/a#default"); s.Name != "default" || !s.Flags.Default {
		t.Errorf("namespace default = %+v", s)
	}
}
