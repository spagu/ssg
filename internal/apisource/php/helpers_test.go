package php

import (
	"os"
	"path/filepath"
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

// extractTree extracts a package named "p" written from files.
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

// find returns the symbol or member with the given ID, or fails the test.
func find(t *testing.T, pkg *apimodel.Package, id string) *apimodel.Symbol {
	t.Helper()
	var found *apimodel.Symbol
	for _, m := range pkg.Modules {
		apimodel.Walk(m.Symbols, func(s, _ *apimodel.Symbol) {
			if s.ID == id {
				found = s
			}
		})
	}
	if found == nil {
		t.Fatalf("no symbol %s", id)
	}
	return found
}

// ids lists the IDs of symbols in order.
func ids(syms []*apimodel.Symbol) []string {
	out := make([]string, len(syms))
	for i, s := range syms {
		out[i] = s.ID
	}
	return out
}

// php wraps declarations in an opening tag.
func php(src string) string { return "<?php\n" + src }
