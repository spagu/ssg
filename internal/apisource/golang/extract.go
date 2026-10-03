package golang

import (
	"errors"
	"fmt"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// Language is the value of Package.Language for Go packages.
const Language = "go"

// Extract reads the Go module or package at cfg.Root into the model. It
// never panics on bad input: a file that cannot be read or parsed becomes a
// Diagnostic and the others are still read; err is only for a root that
// cannot be opened or holds no Go package to document.
func Extract(cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic, error) {
	if cfg.Root == "" {
		return nil, nil, errors.New("api docs: no root directory configured")
	}
	root, err := os.OpenRoot(cfg.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("api docs: %w", err)
	}
	defer func() { _ = root.Close() }()
	ld := &loader{fsys: root.FS(), fset: token.NewFileSet(),
		filter: cfg.Filter()}
	modPath := ld.modulePath()
	name := packageName(cfg, modPath)
	if modPath == "" {
		modPath = dirName(cfg.Root)
	}
	dirs := ld.packages(modPath, cfg.Entries)
	if len(dirs) == 0 {
		return nil, sortDiags(ld.diags), fmt.Errorf("api docs: no Go package found in %s", cfg.Root)
	}
	ix := newIndex(name, ld.fset, dirs)
	pkg := &apimodel.Package{Name: name, Language: Language, Readme: ld.readme(), Modules: []*apimodel.Module{}}
	for _, d := range dirs {
		pkg.Modules = append(pkg.Modules, ix.buildModule(d))
	}
	sort.Slice(pkg.Modules, func(i, j int) bool { return pkg.Modules[i].ID < pkg.Modules[j].ID })
	return pkg, sortDiags(ld.diags), nil
}

// Detect reports whether root holds Go code to document: a go.mod, or .go
// files directly in it.
func Detect(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
		return true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}

// packageName is the configured name, else the last element of the module
// path, else the name of the root directory.
func packageName(cfg apisource.Config, modPath string) string {
	switch {
	case cfg.Name != "":
		return cfg.Name
	case modPath != "":
		return path.Base(modPath)
	}
	return dirName(cfg.Root)
}

// dirName is the base name of a directory, made absolute first so "." has
// a name.
func dirName(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return filepath.Base(dir)
}

// modulePath reads the module directive of go.mod; "" without one.
func (ld *loader) modulePath() string {
	data, err := fs.ReadFile(ld.fsys, "go.mod")
	if err != nil {
		return ""
	}
	for line := range strings.Lines(string(data)) {
		line, _, _ = strings.Cut(line, "//")
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module"); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`+"`")
		}
	}
	return ""
}

// readme returns the root's README.md, or "".
func (ld *loader) readme() string {
	for _, name := range []string{"README.md", "readme.md"} {
		if data, err := fs.ReadFile(ld.fsys, name); err == nil {
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
