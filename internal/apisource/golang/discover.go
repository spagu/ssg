package golang

import (
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apisource"
)

// loader walks the root, parses the files the filter allows and collects
// diagnostics on the way.
type loader struct {
	fsys   fs.FS
	fset   *token.FileSet
	filter apisource.Filter
	diags  []apisource.Diagnostic
}

// diag records a diagnostic.
func (ld *loader) diag(sev apisource.Severity, file string, line int, msg string) {
	ld.diags = append(ld.diags, apisource.Diagnostic{Severity: sev, File: file, Line: line, Message: msg})
}

// packages finds the package directories under the root, in path order,
// limited to entries when there are any ("." is the root package).
// A directory that cannot be read is a diagnostic.
func (ld *loader) packages(modPath string, entries []string) []*pkgDir {
	byDir := map[string][]string{}
	// The callback never returns an error, so neither does the walk.
	_ = fs.WalkDir(ld.fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			ld.diag(apisource.Warning, p, 0, "cannot read: "+err.Error())
			return nil
		}
		if d.IsDir() {
			if p != "." && ld.skipDir(p, d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") && ld.filter.Allows(p) {
			byDir[path.Dir(p)] = append(byDir[path.Dir(p)], p)
		}
		return nil
	})
	wanted := entrySet(entries)
	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		if wanted == nil || wanted[dir] {
			dirs = append(dirs, dir)
			delete(wanted, dir)
		}
	}
	sort.Strings(dirs)
	for _, missing := range sortedKeys(wanted) {
		ld.diag(apisource.Error, missing, 0, "entry package not found")
	}
	var out []*pkgDir
	for _, dir := range dirs {
		if d := ld.loadDir(dir, modPath, byDir[dir]); d != nil {
			out = append(out, d)
		}
	}
	return out
}

// skipDir reports whether a directory holds no package of this module:
// testdata, vendor, dot- and underscore-directories, nested modules.
func (ld *loader) skipDir(p, name string) bool {
	if name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}
	_, err := fs.Stat(ld.fsys, path.Join(p, "go.mod"))
	return err == nil
}

// entrySet turns configured entry directories into slash paths relative to
// the root; nil when there are none.
func entrySet(entries []string) map[string]bool {
	if len(entries) == 0 {
		return nil
	}
	set := map[string]bool{}
	for _, e := range entries {
		set[path.Clean(strings.TrimPrefix(filepath.ToSlash(e), "./"))] = true
	}
	return set
}

// sortedKeys returns the keys of a set in order.
func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
