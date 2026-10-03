package dts

import (
	"os"
	"path"
	"strings"

	"github.com/spagu/ssg/internal/apisource"
)

// loader reads and parses declaration files under the package root, each
// once, following the relative imports and re-exports between them.
type loader struct {
	root  *os.Root
	cfg   apisource.Config
	files map[string]*file // nil value: unreadable, or still loading
	diags []apisource.Diagnostic
}

// newLoader returns a loader reading from r.
func newLoader(r *os.Root, cfg apisource.Config) *loader {
	return &loader{root: r, cfg: cfg, files: map[string]*file{}}
}

// load parses the file at p (relative to the root) and every file it
// refers to. It returns nil for a file that cannot be read or tokenized, or
// that the configuration leaves out; the problem is a diagnostic.
func (l *loader) load(p string) *file {
	if f, seen := l.files[p]; seen {
		return f
	}
	l.files[p] = nil
	if !l.included(p) {
		return nil
	}
	data, err := l.root.ReadFile(p)
	if err != nil {
		l.report(apisource.Error, p, 0, "cannot read the declaration file: "+trimRootError(err))
		return nil
	}
	f, diags, err := parseFile(p, string(data))
	l.diags = append(l.diags, diags...)
	if err != nil {
		l.report(apisource.Error, p, 0, err.Error())
		return nil
	}
	f.srcMap = l.sourceMap(p)
	l.files[p] = f
	for _, spec := range f.specifiers() {
		l.resolve(f, spec)
	}
	return f
}

// resolve returns the file a module specifier in f refers to, loading it;
// nil for another package or a file outside the root.
func (l *loader) resolve(f *file, spec string) *file {
	target := relativeTarget(f.path, spec)
	if target == "" {
		return nil
	}
	found := findFile(l.root, declarationCandidates(target))
	if found == "" {
		l.report(apisource.Warning, f.path, 0, "cannot find a declaration file for "+spec)
		return nil
	}
	return l.load(found)
}

// sourceMap reads the source map beside a declaration file, if any.
func (l *loader) sourceMap(p string) *sourceMap {
	data, err := l.root.ReadFile(p + ".map")
	if err != nil {
		return nil
	}
	m, err := parseSourceMap(data, p+".map")
	if err != nil {
		l.report(apisource.Warning, p+".map", 0, err.Error())
		return nil
	}
	return m
}

// included applies the Include and Exclude globs of the configuration.
func (l *loader) included(p string) bool {
	if len(l.cfg.Include) > 0 && !matchAny(l.cfg.Include, p) {
		return false
	}
	return !matchAny(l.cfg.Exclude, p)
}

// report adds a diagnostic.
func (l *loader) report(sev apisource.Severity, file string, line int, msg string) {
	l.diags = append(l.diags, apisource.Diagnostic{Severity: sev, File: file, Line: line, Message: msg})
}

// specifiers lists every module specifier a file refers to, in a stable
// order: imports, re-exports and star exports, in every scope.
func (f *file) specifiers() []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, k := range sortedKeys(f.imports) {
		add(f.imports[k].from)
	}
	scopes := []*scope{f.top}
	for _, k := range sortedKeys(f.ambients) {
		scopes = append(scopes, f.ambients[k])
	}
	for _, sc := range scopes {
		for _, k := range sortedKeys(sc.exports) {
			add(sc.exports[k].from)
		}
		for _, s := range sc.stars {
			add(s)
		}
	}
	return out
}

// trimRootError drops the "openat" noise from an os.Root error.
func trimRootError(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		return msg[i+2:]
	}
	return msg
}

// matchAny reports whether p matches one of the globs; "**" matches any
// number of path segments.
func matchAny(globs []string, p string) bool {
	for _, g := range globs {
		if matchSegments(strings.Split(cleanGlob(g), "/"), strings.Split(p, "/")) {
			return true
		}
	}
	return false
}

// cleanGlob normalises a glob like a path: "/" separators, no "./".
func cleanGlob(g string) string {
	return strings.TrimPrefix(path.Clean(strings.ReplaceAll(g, "\\", "/")), "./")
}

// matchSegments matches glob segments against path segments.
func matchSegments(glob, segs []string) bool {
	if len(glob) == 0 {
		return len(segs) == 0
	}
	if glob[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if matchSegments(glob[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	ok, err := path.Match(glob[0], segs[0])
	return err == nil && ok && matchSegments(glob[1:], segs[1:])
}
