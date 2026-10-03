package php

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// Extract reads a PHP package into the model. Files the scanner cannot
// read (an unterminated string, comment or heredoc, unbalanced brackets)
// become Error diagnostics and are left out; err is only for a root that
// cannot be read or holds no PHP file to read.
func Extract(cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic, error) {
	if cfg.Root == "" {
		return nil, nil, errors.New("api docs: no root directory configured")
	}
	root, err := os.OpenRoot(cfg.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("api docs: %w", err)
	}
	defer func() { _ = root.Close() }()
	cj, diags := readComposer(root)
	files, more := listFiles(root, cfg, cj)
	diags = append(diags, more...)
	if len(files) == 0 {
		return nil, sortDiags(diags), fmt.Errorf("api docs: no PHP files found in %s", cfg.Root)
	}
	pkg := &apimodel.Package{Name: packageName(cfg.Name, cfg.Root, cj), Version: cj.Version, Language: "php",
		Readme: readme(root)}
	var decls []*decl
	for _, f := range files {
		ds, err := readFile(root, f)
		if err != nil {
			line, msg := 0, err.Error()
			var le *lexError
			if errors.As(err, &le) {
				line, msg = le.line, le.msg
			}
			diags = append(diags, apisource.Diagnostic{Severity: apisource.Error, File: f, Line: line, Message: msg})
			continue
		}
		decls = append(decls, ds...)
	}
	mods, more := buildModules(pkg.Name, decls)
	pkg.Modules = mods
	return pkg, sortDiags(append(diags, more...)), nil
}

// readFile reads and parses one source file.
func readFile(root *os.Root, file string) ([]*decl, error) {
	data, err := root.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return parseFile(file, string(data))
}

// Detect reports whether root holds a PHP package: a composer.json, or a
// .php file outside vendor, tests and dot-directories.
func Detect(root string) bool {
	r, err := os.OpenRoot(root)
	if err != nil {
		return false
	}
	defer func() { _ = r.Close() }()
	if _, err := r.Stat("composer.json"); err == nil {
		return true
	}
	found := false
	walkPHP(r, ".", func(string) { found = true })
	return found
}

// readme returns the package's README.md, or "".
func readme(root *os.Root) string {
	for _, name := range []string{"README.md", "readme.md", "Readme.md"} {
		if data, err := root.ReadFile(name); err == nil {
			return string(data)
		}
	}
	return ""
}

// sortDiags orders diagnostics by file, line and message.
func sortDiags(ds []apisource.Diagnostic) []apisource.Diagnostic {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i], ds[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Message < b.Message
	})
	return ds
}
