package python

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// Language is the apimodel.Package language this extractor produces.
const Language = "python"

// Extract reads a Python package into the model. Files that cannot be
// scanned cleanly become Diagnostics and the rest is still read; err is
// only for a root that cannot be read or holds no Python module.
func Extract(cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic, error) {
	if cfg.Root == "" {
		return nil, nil, errors.New("api docs: no root directory configured")
	}
	root, err := os.OpenRoot(cfg.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("api docs: %w", err)
	}
	defer func() { _ = root.Close() }()
	fsys := root.FS()
	files, diags := findFiles(fsys, cfg)
	x := &extraction{fsys: fsys, modules: map[string]*module{}, resolvers: map[*module]resolver{}, diags: diags}
	for _, f := range files {
		x.load(f)
	}
	if len(x.modules) == 0 {
		return nil, sortDiags(x.diags), fmt.Errorf("api docs: no Python module found in %s", cfg.Root)
	}
	name, version := projectMeta(fsys)
	pkg := &apimodel.Package{
		Name:     firstNonEmpty(cfg.Name, name, x.topPackage(), rootName(cfg.Root)),
		Version:  version,
		Language: Language,
		Readme:   readme(fsys),
		Modules:  []*apimodel.Module{},
	}
	x.plan(pkg.Name)
	for _, m := range x.ordered {
		if privateModule(m) {
			continue // read for its definitions, shown only through re-exports
		}
		pkg.Modules = append(pkg.Modules, x.buildModule(m))
	}
	return pkg, sortDiags(x.diags), nil
}

// topPackage is the name of the single top-level package read, or "".
func (x *extraction) topPackage() string {
	tops := map[string]bool{}
	for p := range x.modules {
		top, _, _ := strings.Cut(p, "/")
		tops[top] = true
	}
	if len(tops) != 1 {
		return ""
	}
	for t := range tops {
		return t
	}
	return ""
}

// rootName is the base name of the root directory.
func rootName(root string) string {
	if abs, err := filepath.Abs(root); err == nil {
		return filepath.Base(abs)
	}
	return filepath.Base(root)
}

// sortDiags orders diagnostics by file, line and message, dropping repeats.
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
	out := ds[:0]
	for _, d := range ds {
		if len(out) == 0 || d != out[len(out)-1] {
			out = append(out, d)
		}
	}
	return out
}
