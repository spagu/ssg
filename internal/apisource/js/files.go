package js

import (
	"bytes"
	"errors"
	"os"
	"path"
	"strings"

	"github.com/spagu/ssg/internal/apidoc"
	"github.com/spagu/ssg/internal/apisource"
	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

// file is one parsed source file and what it declares, imports and exports.
type file struct {
	path    string // relative to the root, slash-separated
	scan    *scan
	sites   map[string]int          // top-level declaration sites, see sites.go
	locals  map[string]*decl        // top-level declarations by local name
	imports map[string]exportRef    // local binding → where it comes from
	exports map[string]exportRef    // public name → what it is
	owners  map[string]*file        // name → star re-exported module, see starOwners
	stars   []string                // specifiers of "export * from"
	deps    []string                // every relative specifier, in source order
	parsed  map[int]*apidoc.Comment // doc comments parsed so far, by index
	cjs     bool                    // the file assigns CommonJS exports
}

// loader reads and parses the files of one package, once each.
type loader struct {
	root   *os.Root
	filter apisource.Filter
	files  map[string]*file     // nil = read and failed
	specs  map[[2]string]string // (from, specifier) → resolved path, "" = external
	diags  []apisource.Diagnostic
}

// diag records a diagnostic.
func (ld *loader) diag(sev apisource.Severity, file string, line int, msg string) {
	ld.diags = append(ld.diags, apisource.Diagnostic{Severity: sev, File: file, Line: line, Message: msg})
}

// load returns the parsed file at p (relative to the root), or nil when it
// cannot be read or parsed; the reason becomes a diagnostic.
func (ld *loader) load(p string) *file {
	if f, ok := ld.files[p]; ok {
		return f
	}
	ld.files[p] = nil // a cycle met while loading sees "not available"
	src, err := ld.root.ReadFile(p)
	if err != nil {
		ld.diag(apisource.Error, p, 0, "cannot read the file: "+unwrapPath(err))
		return nil
	}
	ast, err := js.Parse(parse.NewInputBytes(bytes.Clone(src)), js.Options{})
	if err != nil {
		ld.diag(apisource.Error, p, errorLine(err), "syntax error: "+errorMessage(err))
		return nil
	}
	f := newFile(p, src)
	ld.files[p] = f
	f.collect(ld, ast.List)
	return f
}

// newFile prepares an empty file record with its token scan.
func newFile(p string, src []byte) *file {
	s := scanSource(src)
	return &file{
		path: p, scan: s, sites: s.topLevelSites(),
		locals: map[string]*decl{}, imports: map[string]exportRef{}, exports: map[string]exportRef{},
		parsed: map[int]*apidoc.Comment{},
	}
}

// resolveSpec turns an import specifier into a file path relative to the
// root, or "" when it is external: a bare package name, a path leaving the
// root, a file the filters exclude, or one that does not exist.
func (ld *loader) resolveSpec(from, spec string) string {
	if !strings.HasPrefix(spec, "./") && !strings.HasPrefix(spec, "../") {
		return ""
	}
	joined := path.Join(path.Dir(from), spec)
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return ""
	}
	for _, c := range candidates(joined) {
		if ld.filter.Allows(c) && ld.exists(c) {
			return c
		}
	}
	return ""
}

// candidates are the files a specifier without a known extension may name.
func candidates(p string) []string {
	if isScript(p) {
		return []string{p}
	}
	out := make([]string, 0, 2*len(scriptExts))
	for _, ext := range scriptExts {
		out = append(out, p+ext)
	}
	for _, ext := range scriptExts {
		out = append(out, p+"/index"+ext)
	}
	return out
}

// exists reports whether a regular file exists under the root.
func (ld *loader) exists(p string) bool {
	fi, err := ld.root.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// dep returns the file a specifier of f names, loading it, or nil when it
// is external or unreadable.
func (ld *loader) dep(f *file, spec string) *file {
	key := [2]string{f.path, spec}
	p, ok := ld.specs[key]
	if !ok {
		p = ld.resolveSpec(f.path, spec)
		ld.specs[key] = p
	}
	if p == "" {
		return nil
	}
	return ld.load(p)
}

// errorLine returns the line of a parse error, or 0.
func errorLine(err error) int {
	var pe *parse.Error
	if errors.As(err, &pe) {
		return pe.Line
	}
	return 0
}

// errorMessage returns the message of a parse error without its context.
func errorMessage(err error) string {
	var pe *parse.Error
	if errors.As(err, &pe) {
		return pe.Message
	}
	return err.Error()
}

// unwrapPath returns the reason of a file error without the path.
func unwrapPath(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}
