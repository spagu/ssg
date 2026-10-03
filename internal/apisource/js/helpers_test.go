package js

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

// extractTree extracts a package written from files, named "p", with
// index.js as its default entry.
func extractTree(t *testing.T, files map[string]string, cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic) {
	t.Helper()
	cfg.Root = writeTree(t, files)
	if cfg.Name == "" {
		cfg.Name = "p"
	}
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

// extractIndex extracts a one-file package and returns its index module.
func extractIndex(t *testing.T, src string) (*apimodel.Module, []apisource.Diagnostic) {
	t.Helper()
	pkg, diags := extractTree(t, map[string]string{"index.js": src}, apisource.Config{})
	if len(pkg.Modules) != 1 {
		t.Fatalf("modules = %d, want 1", len(pkg.Modules))
	}
	return pkg.Modules[0], diags
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

// names lists symbol names, for failure messages.
func names(syms []*apimodel.Symbol) []string {
	out := make([]string, len(syms))
	for i, s := range syms {
		out[i] = s.Name
	}
	return out
}

// hasSymbol reports whether a module lists a top-level name.
func hasSymbol(m *apimodel.Module, name string) bool {
	for _, s := range m.Symbols {
		if s.Name == name {
			return true
		}
	}
	return false
}

// hasDiag reports whether a diagnostic message contains text.
func hasDiag(diags []apisource.Diagnostic, sev apisource.Severity, text string) bool {
	for _, d := range diags {
		if d.Severity == sev && strings.Contains(d.Message, text) {
			return true
		}
	}
	return false
}
