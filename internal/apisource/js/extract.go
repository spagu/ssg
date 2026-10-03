package js

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// Extract reads a JavaScript package into the model. It never panics on bad
// input: unreadable or unparsable files become Diagnostics; err is only for
// a configuration that cannot work (no Root, no entry point found).
//
// Entry files are always read; Include and Exclude decide which of the
// files they import are read, the rest being external.
func Extract(cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic, error) {
	if cfg.Root == "" {
		return nil, nil, errors.New("api docs: no root directory configured")
	}
	root, err := os.OpenRoot(cfg.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("api docs: %w", err)
	}
	defer func() { _ = root.Close() }()
	ld := &loader{root: root, filter: filter{include: cfg.Include, exclude: cfg.Exclude}, files: map[string]*file{},
		specs: map[[2]string]string{}}
	pj := ld.packageJSON()
	pkg := &apimodel.Package{Name: packageName(cfg, pj), Version: pj.Version, Readme: ld.readme(), Modules: []*apimodel.Module{}}
	entries := ld.entryFiles(entryCandidates(cfg.Entries, pj))
	if len(entries) == 0 {
		return nil, sortDiags(ld.diags), fmt.Errorf("api docs: no entry point found in %s", cfg.Root)
	}
	var plans []*entryModule
	for _, p := range entries {
		if f := ld.load(p); f != nil {
			plans = append(plans, ld.planModule(pkg.Name, f))
		}
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].id < plans[j].id })
	ix := typeIndex(plans)
	b := &builder{ld: ld, resolve: func(name string) string { return ix[name] }}
	for _, m := range plans {
		pkg.Modules = append(pkg.Modules, b.buildModule(m))
	}
	return pkg, sortDiags(ld.diags), nil
}

// packageName is the configured name, else package.json's, else the name
// of the root directory.
func packageName(cfg apisource.Config, pj *packageJSON) string {
	switch {
	case cfg.Name != "":
		return cfg.Name
	case pj.Name != "":
		return pj.Name
	}
	abs, err := filepath.Abs(cfg.Root)
	if err != nil {
		return filepath.Base(cfg.Root)
	}
	return filepath.Base(abs)
}

// packageJSON reads package.json; a missing file is an empty one, a
// malformed one a diagnostic.
func (ld *loader) packageJSON() *packageJSON {
	pj := &packageJSON{}
	data, err := ld.root.ReadFile("package.json")
	if err != nil {
		return pj
	}
	if err := json.Unmarshal(data, pj); err != nil {
		ld.diag(apisource.Error, "package.json", 0, "cannot read package.json: "+err.Error())
		return &packageJSON{}
	}
	return pj
}

// readme returns the package's README.md, or "".
func (ld *loader) readme() string {
	for _, name := range []string{"README.md", "readme.md", "Readme.md"} {
		if data, err := ld.root.ReadFile(name); err == nil {
			return string(data)
		}
	}
	return ""
}

// entryFiles resolves entry candidates to existing files, without
// repeats. A candidate that names no file is a diagnostic.
func (ld *loader) entryFiles(list []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range list {
		p := path.Clean(strings.TrimPrefix(filepath.ToSlash(raw), "./"))
		found := ""
		if p != ".." && !strings.HasPrefix(p, "../") && !path.IsAbs(p) {
			for _, c := range candidates(p) {
				if ld.exists(c) {
					found = c
					break
				}
			}
		}
		if found == "" {
			ld.diag(apisource.Error, raw, 0, "entry point not found")
			continue
		}
		if !seen[found] {
			seen[found] = true
			out = append(out, found)
		}
	}
	return out
}

// sortDiags orders diagnostics by file, line and message, dropping repeats
// (a symbol listed in two places is built twice).
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
