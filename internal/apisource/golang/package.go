package golang

import (
	"errors"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/scanner"
	"io/fs"
	"path"
	"strings"

	"github.com/spagu/ssg/internal/apisource"
)

// pkgDir is one package directory to document.
type pkgDir struct {
	dir        string // slash path relative to the root, "." for the root
	importPath string
	name       string // the package clause
	modPath    string // Module.Path: the package name for the root, else dir
	modID      string
	internal   bool // under an internal directory
	files      []*ast.File
	docPkg     *doc.Package
	imports    map[string]map[string]string // file → import name → import path
}

// loadDir parses the files of one directory into a package; nil when it
// holds no documentable package (only tests, package main, nothing that
// parses).
func (ld *loader) loadDir(dir, modPath string, names []string) *pkgDir {
	var srcs, tests []*ast.File
	for _, name := range names {
		f := ld.parse(name)
		switch {
		case f == nil:
		case strings.HasSuffix(name, "_test.go"):
			tests = append(tests, f)
		case !ignored(f):
			srcs = append(srcs, f)
		}
	}
	pkgName := chooseName(srcs)
	if pkgName == "" || pkgName == "main" {
		return nil
	}
	d := &pkgDir{dir: dir, name: pkgName, importPath: modPath, modPath: pkgName,
		internal: isInternal(dir), imports: map[string]map[string]string{}}
	if dir != "." {
		d.importPath, d.modPath = modPath+"/"+dir, dir
	}
	for _, f := range srcs {
		if f.Name.Name == pkgName {
			d.files = append(d.files, f)
			continue
		}
		pos := ld.fset.Position(f.Name.Pos())
		ld.diag(apisource.Warning, pos.Filename, pos.Line, "package "+f.Name.Name+" is not "+pkgName+"; file skipped")
	}
	for _, f := range tests {
		if n := f.Name.Name; n == pkgName || n == pkgName+"_test" {
			d.files = append(d.files, f)
		}
	}
	// NewFromFiles fails only on files missing from the file set or without
	// a .go name, which cannot happen here.
	d.docPkg, _ = doc.NewFromFiles(ld.fset, d.files, d.importPath)
	return d
}

// parse reads and parses one file; nil, with a diagnostic, when it cannot.
func (ld *loader) parse(name string) *ast.File {
	src, err := fs.ReadFile(ld.fsys, name)
	if err != nil {
		ld.diag(apisource.Error, name, 0, "cannot read: "+err.Error())
		return nil
	}
	f, err := parser.ParseFile(ld.fset, name, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		line, msg := 0, err.Error()
		var list scanner.ErrorList
		if errors.As(err, &list) && len(list) > 0 {
			line, msg = list[0].Pos.Line, list[0].Msg
		}
		ld.diag(apisource.Error, name, line, "cannot parse: "+msg)
		return nil
	}
	return f
}

// ignored reports whether a file is excluded from every build by
// "//go:build ignore", as generators living next to a library are.
func ignored(f *ast.File) bool {
	for _, g := range f.Comments {
		if g.Pos() > f.Package {
			break
		}
		for _, c := range g.List {
			if strings.TrimSpace(c.Text) == "//go:build ignore" {
				return true
			}
		}
	}
	return false
}

// chooseName picks the package clause most files share; on a tie a name
// other than main wins, then the first in alphabetical order.
func chooseName(files []*ast.File) string {
	count := map[string]int{}
	for _, f := range files {
		count[f.Name.Name]++
	}
	best := ""
	for name, n := range count {
		if best == "" || better(name, n, best, count[best]) {
			best = name
		}
	}
	return best
}

// better reports whether package name a (on n files) beats b (on m files).
func better(a string, n int, b string, m int) bool {
	if n != m {
		return n > m
	}
	if (a == "main") != (b == "main") {
		return b == "main"
	}
	return a < b
}

// isInternal reports whether a directory is, or is under, an internal one.
func isInternal(dir string) bool {
	for seg := range strings.SplitSeq(path.Clean(dir), "/") {
		if seg == "internal" {
			return true
		}
	}
	return false
}
