package php

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apisource"
)

// symbolNames lists every top-level symbol ID of a package.
func symbolNames(t *testing.T, files map[string]string, cfg apisource.Config) (string, []apisource.Diagnostic) {
	t.Helper()
	pkg, diags := extractTree(t, files, cfg)
	var out []string
	for _, m := range pkg.Modules {
		out = append(out, ids(m.Symbols)...)
	}
	return strings.Join(out, " "), diags
}

func TestFileSelection(t *testing.T) {
	fn := func(name string) string { return php("function " + name + "() {}") }
	tests := []struct {
		name  string
		files map[string]string
		cfg   apisource.Config
		want  string
		diags string
	}{
		{"src when no composer", map[string]string{"src/a.php": fn("a"), "b.php": fn("b")}, apisource.Config{}, "p/global#a", ""},
		{"root when no src", map[string]string{"a.php": fn("a"), "lib/b.php": fn("b"), "test/c.php": fn("c"),
			".git/d.php": fn("d"), "node_modules/e.php": fn("e"), "x.txt": "x"}, apisource.Config{}, "p/global#a p/global#b", ""},
		{"composer psr-0 list and classmap", map[string]string{
			"composer.json": `{"autoload": {"psr-0": {"A": ["lib/", "more"]}, "psr-4": {"": ""}, "classmap": ["cm/x.php", "missing/"]}}`,
			"lib/a.php":     fn("a"), "more/b.php": fn("b"), "cm/x.php": fn("x"), "vendor/v.php": fn("v")},
			apisource.Config{}, "p/global#a p/global#b p/global#x", "missing: autoload path not found"},
		{"entries", map[string]string{"src/a.php": fn("a"), "other/b.php": fn("b"), "one.php": fn("one")},
			apisource.Config{Entries: []string{"./other", "one.php", "nope", "../up"}}, "p/global#b p/global#one",
			"../up: entry point not found; nope: entry point not found"},
		{"include and exclude", map[string]string{"src/a.php": fn("a"), "src/deep/b.php": fn("b"), "src/deep/c.php": fn("c")},
			apisource.Config{Include: []string{"src/**/*.php"}, Exclude: []string{"./src/deep/c.php", "["}}, "p/global#a p/global#b", ""},
		{"include filters named file", map[string]string{"src/a.php": fn("a"), "b.php": fn("b")},
			apisource.Config{Entries: []string{"b.php", "src"}, Include: []string{"src/*"}}, "p/global#a", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, diags := symbolNames(t, tt.files, tt.cfg)
			if got != tt.want {
				t.Errorf("symbols = %q, want %q", got, tt.want)
			}
			var ds []string
			for _, d := range diags {
				ds = append(ds, d.File+": "+d.Message)
			}
			if strings.Join(ds, "; ") != tt.diags {
				t.Errorf("diagnostics = %q, want %q", strings.Join(ds, "; "), tt.diags)
			}
		})
	}
}

func TestPackageNaming(t *testing.T) {
	files := map[string]string{"composer.json": `{"name": "acme/kit", "version": "1.0.0"}`, "a.php": php("")}
	pkg, _, err := Extract(apisource.Config{Root: writeTree(t, files)})
	if err != nil || pkg.Name != "kit" || pkg.Version != "1.0.0" {
		t.Fatalf("pkg = %+v, %v", pkg, err)
	}
	root := writeTree(t, map[string]string{"a.php": php(""), "composer.json": `{"name": `})
	pkg, diags, err := Extract(apisource.Config{Root: root})
	if err != nil || pkg.Name != filepath.Base(root) || len(diags) != 1 || !strings.Contains(diags[0].Message, "cannot read composer.json") {
		t.Fatalf("pkg = %+v, %v, %v", pkg, diags, err)
	}
	if got := packageName("", ".", &composerJSON{}); got == "" || got == "." {
		t.Errorf("name of . = %q", got)
	}
	var pl pathList
	if err := pl.UnmarshalJSON([]byte(`5`)); err == nil {
		t.Error("a number is not a path list")
	}
}

func TestExtractErrors(t *testing.T) {
	if _, _, err := Extract(apisource.Config{}); err == nil {
		t.Error("no root: want error")
	}
	if _, _, err := Extract(apisource.Config{Root: filepath.Join("testdata", "missing")}); err == nil {
		t.Error("missing root: want error")
	}
	root := writeTree(t, map[string]string{"x.txt": "x"})
	if _, _, err := Extract(apisource.Config{Root: root}); err == nil || !strings.Contains(err.Error(), "no PHP files") {
		t.Errorf("no PHP: err = %v", err)
	}
	root = writeTree(t, map[string]string{"a.php": php("function a() {}"), "b.php": php("")})
	if err := os.Chmod(filepath.Join(root, "b.php"), 0); err != nil {
		t.Fatal(err)
	}
	pkg, diags, err := Extract(apisource.Config{Root: root})
	if _, readable := os.ReadFile(filepath.Join(root, "b.php")); readable == nil { // #nosec G304 -- test path
		t.Skip("running with permission to read anything")
	}
	if err != nil || len(pkg.Modules) != 1 || len(diags) != 1 || diags[0].File != "b.php" {
		t.Errorf("unreadable file: %v %v", diags, err)
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"composer", map[string]string{"composer.json": "{}"}, true},
		{"php file", map[string]string{"lib/a.php": "<?php"}, true},
		{"vendor only", map[string]string{"vendor/a.php": "<?php", "package.json": "{}"}, false},
	}
	for _, tt := range tests {
		if got := Detect(writeTree(t, tt.files)); got != tt.want {
			t.Errorf("%s: %v", tt.name, got)
		}
	}
	if Detect(filepath.Join("testdata", "missing")) {
		t.Error("a missing directory is no package")
	}
}
