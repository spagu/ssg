package python

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// writeTree writes files (slash path → content) under a temporary root.
func writeTree(t testing.TB, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// extractTree extracts a package written from files and checks that the
// model validates.
func extractTree(t *testing.T, files map[string]string, cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic) {
	t.Helper()
	cfg.Root = writeTree(t, files)
	pkg, diags, err := Extract(cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	api := &apimodel.API{Schema: apimodel.Schema, Packages: []*apimodel.Package{pkg}}
	if probs := apimodel.Validate(api); len(probs) > 0 {
		t.Errorf("Validate: %v", probs)
	}
	return pkg, diags
}

// extractModule extracts a one-module package (p/__init__.py holding src)
// and returns that module.
func extractModule(t *testing.T, src string) *apimodel.Module {
	t.Helper()
	pkg, _ := extractTree(t, map[string]string{"p/__init__.py": src}, apisource.Config{})
	return pkg.Modules[0]
}

// moduleNamed finds a module by path.
func moduleNamed(t *testing.T, pkg *apimodel.Package, path string) *apimodel.Module {
	t.Helper()
	for _, m := range pkg.Modules {
		if m.Path == path {
			return m
		}
	}
	t.Fatalf("no module %q", path)
	return nil
}

// symbolNamed finds a symbol (or member, by dotted path) of a module.
func symbolNamed(t *testing.T, m *apimodel.Module, path string) *apimodel.Symbol {
	t.Helper()
	syms := m.Symbols
	var found *apimodel.Symbol
	for _, part := range strings.Split(path, ".") {
		found = nil
		for _, s := range syms {
			if s.Name == part {
				found = s
				break
			}
		}
		if found == nil {
			t.Fatalf("no symbol %q in %s (have %v)", path, m.ID, names(syms))
		}
		syms = found.Members
	}
	return found
}

// names lists symbol names, for failure messages and order checks.
func names(syms []*apimodel.Symbol) []string {
	out := make([]string, len(syms))
	for i, s := range syms {
		out[i] = s.Name
	}
	return out
}

// hasDiag reports whether a diagnostic of a file contains text.
func hasDiag(diags []apisource.Diagnostic, sev apisource.Severity, file, text string) bool {
	for _, d := range diags {
		if d.Severity == sev && d.File == file && strings.Contains(d.Message, text) {
			return true
		}
	}
	return false
}
