// Package dts extracts the API of a package from its TypeScript declaration
// files (.d.ts, .d.mts, .d.cts) into the API model (GO-105).
//
// A hand-written parser reads the declarations — functions and their
// overloads, classes, interfaces, type aliases, enums, variables,
// namespaces, `declare module` blocks and every import and export form —
// and hands the text of each type position to tstype. Names are resolved in
// a second pass, once every file's declarations are known, so a type links
// to the symbol it names wherever that symbol is defined. A symbol is listed
// under its public name in the entry module and documented where it is
// defined; a source map beside a declaration file points sources at the
// original .ts file.
package dts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// Extract reads a package's TypeScript declaration files (.d.ts, .d.mts, .d.cts)
// into the model. Bad files become Diagnostics; err only when nothing can work.
func Extract(cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic, error) {
	r, err := os.OpenRoot(cfg.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("api docs: %w", err)
	}
	defer func() { _ = r.Close() }()
	var diags []apisource.Diagnostic
	pj, err := readPackageJSON(r)
	if err != nil {
		diags = append(diags, apisource.Diagnostic{Severity: apisource.Error, File: "package.json", Message: "cannot read package.json: " + err.Error()})
	}
	entries, entryDiags := discoverEntries(r, cfg, pj)
	diags = append(diags, entryDiags...)
	if len(entries) == 0 {
		return nil, sortDiagnostics(diags), fmt.Errorf("api docs: no declaration files in %s: set \"types\" in package.json, add index.d.ts or list entries", cfg.Root)
	}
	l := newLoader(r, cfg)
	loaded := 0
	for _, e := range entries {
		if l.load(e) != nil {
			loaded++
		}
	}
	if loaded == 0 {
		return nil, sortDiagnostics(append(diags, l.diags...)), errors.New("api docs: no entry declaration file could be read")
	}
	b := newBuilder(firstNonEmpty(cfg.Name, pj.Name, rootName(cfg.Root)), l)
	pkg := b.build(entries)
	pkg.Version, pkg.Readme = pj.Version, readme(r)
	diags = append(append(diags, l.diags...), b.diags...) // the loader also reports while names resolve
	return pkg, sortDiagnostics(diags), nil
}

// build writes the planned modules: IDs first, so every type reference can
// link to any listed symbol, then the symbols themselves.
func (b *builder) build(entries []string) *apimodel.Package {
	plans := b.plan(entries)
	for _, mp := range plans {
		for _, l := range mp.listings {
			b.assign(l.n, l.id)
		}
	}
	pkg := &apimodel.Package{Name: b.pkg, Modules: []*apimodel.Module{}}
	for _, mp := range plans {
		mod := &apimodel.Module{ID: mp.id, Path: mp.path, Doc: moduleDoc(mp.doc), Symbols: []*apimodel.Symbol{}}
		for _, l := range mp.listings {
			s := b.symbol(l.n, l.name, l.id, nil)
			s.Flags.Default = l.isDefault
			mod.Symbols = append(mod.Symbols, s)
		}
		pkg.Modules = append(pkg.Modules, mod)
	}
	return pkg
}

// rootName is the name of the root directory, the package name of last
// resort.
func rootName(root string) string {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return filepath.Base(root)
}

// readme returns the package's README, or "".
func readme(r *os.Root) string {
	for _, name := range []string{"README.md", "readme.md", "Readme.md"} {
		if data, err := r.ReadFile(name); err == nil {
			return string(data)
		}
	}
	return ""
}

// sortDiagnostics orders diagnostics by file, line and message and drops
// repeats, so output does not depend on the order files were read in.
func sortDiagnostics(ds []apisource.Diagnostic) []apisource.Diagnostic {
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].File != ds[j].File {
			return ds[i].File < ds[j].File
		}
		if ds[i].Line != ds[j].Line {
			return ds[i].Line < ds[j].Line
		}
		return ds[i].Message < ds[j].Message
	})
	out := ds[:0]
	for i, d := range ds {
		if i == 0 || d != ds[i-1] {
			out = append(out, d)
		}
	}
	return out
}
