package php

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apisource"
)

// skippedDirs are directories never read: dependencies and tests.
var skippedDirs = map[string]bool{"vendor": true, "tests": true, "test": true, "node_modules": true}

// skipDir reports whether a directory below a starting point is left out:
// a skipped name or a dot-directory.
func skipDir(name string) bool {
	return skippedDirs[name] || (strings.HasPrefix(name, ".") && name != ".")
}

// starts returns where to look for sources, and whether they were named
// in the configuration: the configured entries, else composer.json's
// autoload paths, else src/ when it exists, else the whole root.
func starts(root *os.Root, entries []string, cj *composerJSON) ([]string, bool) {
	if len(entries) > 0 {
		return entries, true
	}
	if paths := cj.autoloadPaths(); len(paths) > 0 {
		return paths, false
	}
	if info, err := root.Stat("src"); err == nil && info.IsDir() {
		return []string{"src"}, false
	}
	return []string{"."}, false
}

// listFiles returns the .php files to read, as sorted slash paths relative
// to the root, filtered by the Include and Exclude globs. A starting point
// that does not exist is a diagnostic.
func listFiles(root *os.Root, cfg apisource.Config, cj *composerJSON) ([]string, []apisource.Diagnostic) {
	var diags []apisource.Diagnostic
	f := cfg.Filter()
	set := map[string]bool{}
	list, configured := starts(root, cfg.Entries, cj)
	for _, raw := range list {
		p := path.Clean(strings.TrimPrefix(filepath.ToSlash(raw), "./"))
		info, err := root.Stat(p)
		if err != nil || p == ".." || strings.HasPrefix(p, "../") || path.IsAbs(p) {
			sev, msg := apisource.Warning, "autoload path not found"
			if configured {
				sev, msg = apisource.Error, "entry point not found"
			}
			diags = append(diags, apisource.Diagnostic{Severity: sev, File: raw, Message: msg})
			continue
		}
		if !info.IsDir() {
			if f.Allows(p) {
				set[p] = true
			}
			continue
		}
		walkPHP(root, p, func(file string) {
			if f.Allows(file) {
				set[file] = true
			}
		})
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, diags
}

// walkPHP calls visit for every .php file under dir, skipping vendor,
// tests and dot-directories below it. Unreadable directories are skipped.
func walkPHP(root *os.Root, dir string, visit func(file string)) {
	_ = fs.WalkDir(root.FS(), dir, func(p string, e fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil
		case e.IsDir() && p != dir && skipDir(e.Name()):
			return fs.SkipDir
		case !e.IsDir() && strings.HasSuffix(p, ".php"):
			visit(p)
		}
		return nil
	})
}

// dirName is the name of a directory, made absolute first so "." has one.
func dirName(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return filepath.Base(dir)
}
