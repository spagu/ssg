package python

import (
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apisource"
)

// skipDirs are directory names never read: tests, docs, build output and
// environments. Dot-directories are skipped too.
var skipDirs = map[string]bool{
	"tests": true, "test": true, "docs": true, "build": true, "dist": true,
	"venv": true, "__pycache__": true, "node_modules": true, "site-packages": true,
}

// skipDir reports whether a directory is left out.
func skipDir(name string) bool { return skipDirs[name] || strings.HasPrefix(name, ".") }

// skipFile reports whether a file is not a module to document.
func skipFile(name string) bool {
	return !strings.HasSuffix(name, ".py") || strings.HasPrefix(name, ".") || name == "setup.py" ||
		name == "conftest.py" || strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.py")
}

// finder collects the source files of a package.
type finder struct {
	fsys   fs.FS
	filter apisource.Filter
	seen   map[string]bool
	files  []string
	diags  []apisource.Diagnostic
}

// findFiles lists the .py files to read, sorted: the configured entries,
// else the import packages under src/ or the root, else the top-level
// single modules.
func findFiles(fsys fs.FS, cfg apisource.Config) ([]string, []apisource.Diagnostic) {
	f := &finder{fsys: fsys, filter: cfg.Filter(), seen: map[string]bool{}}
	if len(cfg.Entries) > 0 {
		for _, e := range cfg.Entries {
			f.entry(e)
		}
	} else {
		f.discover()
	}
	sort.Strings(f.files)
	return f.files, f.diags
}

// entry adds a configured file, or the modules of a configured directory.
func (f *finder) entry(raw string) {
	p := path.Clean(strings.TrimPrefix(filepath.ToSlash(raw), "./"))
	info, err := fs.Stat(f.fsys, p)
	if err != nil || p == ".." || strings.HasPrefix(p, "../") || path.IsAbs(p) {
		f.diags = append(f.diags, apisource.Diagnostic{Severity: apisource.Error, File: raw, Message: "entry not found"})
		return
	}
	if info.IsDir() {
		f.walk(p)
		return
	}
	f.add(p)
}

// discover finds the import packages (directories with __init__.py) under
// src/, else at the root; with none, the single-module files there.
func (f *finder) discover() {
	for _, base := range []string{"src", "."} {
		if dirs := f.packageDirs(base); len(dirs) > 0 {
			for _, d := range dirs {
				f.walk(d)
			}
			return
		}
	}
	for _, base := range []string{"src", "."} {
		entries, _ := fs.ReadDir(f.fsys, base)
		for _, e := range entries {
			if !e.IsDir() && !skipFile(e.Name()) {
				f.add(path.Join(base, e.Name()))
			}
		}
		if len(f.files) > 0 {
			return
		}
	}
}

// packageDirs lists the package directories directly under base.
func (f *finder) packageDirs(base string) []string {
	var out []string
	entries, _ := fs.ReadDir(f.fsys, base)
	for _, e := range entries {
		d := path.Join(base, e.Name())
		if e.IsDir() && !skipDir(e.Name()) && exists(f.fsys, path.Join(d, "__init__.py")) {
			out = append(out, d)
		}
	}
	return out
}

// walk adds the modules under dir, skipping test and build directories.
func (f *finder) walk(dir string) {
	_ = fs.WalkDir(f.fsys, dir, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			f.diags = append(f.diags, apisource.Diagnostic{Severity: apisource.Error, File: p, Message: "cannot read: " + err.Error()})
		case d.IsDir() && p != dir && skipDir(d.Name()):
			return fs.SkipDir
		case !d.IsDir() && !skipFile(d.Name()):
			f.add(p)
		}
		return nil
	})
}

// add records a file once, when the globs allow it.
func (f *finder) add(p string) {
	if !f.seen[p] && f.filter.Allows(p) {
		f.seen[p] = true
		f.files = append(f.files, p)
	}
}

// exists reports whether a file exists in fsys.
func exists(fsys fs.FS, p string) bool {
	_, err := fs.Stat(fsys, p)
	return err == nil
}

// modulePath is the dotted path of a file as a slash path: the file's
// name (none for __init__.py) under every enclosing package directory.
// "src/textkit/lexer.py" → "textkit/lexer".
func modulePath(fsys fs.FS, file string) string {
	dir, name := path.Dir(file), strings.TrimSuffix(path.Base(file), ".py")
	var parts []string
	if name != "__init__" {
		parts = append(parts, name)
	}
	for dir != "." && exists(fsys, path.Join(dir, "__init__.py")) {
		parts = append([]string{path.Base(dir)}, parts...)
		dir = path.Dir(dir)
	}
	if len(parts) == 0 {
		return name
	}
	return strings.Join(parts, "/")
}

// packageOf is the package a module's relative imports start from: the
// module itself for a package's __init__.py, else its parent.
func packageOf(file, modPath string) string {
	if path.Base(file) == "__init__.py" {
		return modPath
	}
	if i := strings.LastIndex(modPath, "/"); i >= 0 {
		return modPath[:i]
	}
	return ""
}
