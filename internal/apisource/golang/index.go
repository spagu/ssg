package golang

import (
	"go/ast"
	"go/printer"
	"go/token"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spagu/ssg/internal/apimodel"
)

// index knows every package of the extraction and the exported types each
// declares, so a type written anywhere can point at its symbol. It is the
// first pass; buildModule is the second.
type index struct {
	fset   *token.FileSet
	byPath map[string]*pkgDir           // import path → package
	types  map[string]map[string]string // import path → type name → symbol ID
}

// newIndex assigns module IDs and indexes the types and imports of dirs.
func newIndex(pkgName string, fset *token.FileSet, dirs []*pkgDir) *index {
	ix := &index{fset: fset, byPath: map[string]*pkgDir{}, types: map[string]map[string]string{}}
	for _, d := range dirs {
		d.modID = apimodel.ModuleID(pkgName, d.modPath)
		ix.byPath[d.importPath] = d
		names := map[string]string{}
		for _, t := range d.docPkg.Types {
			names[t.Name] = apimodel.SymbolID(d.modID, t.Name)
		}
		ix.types[d.importPath] = names
	}
	for _, d := range dirs {
		for _, f := range d.files {
			d.imports[ix.fset.Position(f.Package).Filename] = ix.importNames(f)
		}
	}
	return ix
}

// importNames maps the names a file imports packages of this extraction
// under to their import paths. Other imports are left out: nothing of
// theirs is documented here.
func (ix *index) importNames(f *ast.File) map[string]string {
	names := map[string]string{}
	for _, spec := range f.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		d := ix.byPath[p]
		if err != nil || d == nil {
			continue
		}
		name := d.name
		if spec.Name != nil {
			name = spec.Name.Name
		}
		names[name] = p
	}
	return names
}

// print renders a node as Go source.
func (ix *index) print(node any) string {
	var b strings.Builder
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	// A strings.Builder does not fail and every node printed is one the
	// printer supports.
	_ = cfg.Fprint(&b, ix.fset, node)
	return b.String()
}

// source locates a position for links to the code.
func (ix *index) source(pos token.Pos) *apimodel.Source {
	p := ix.fset.Position(pos)
	return &apimodel.Source{File: path.Clean(p.Filename), Line: p.Line}
}

// maxValue is how many characters of a value expression a declaration shows.
const maxValue = 80

// truncate shortens s to maxValue characters, marking the cut.
func truncate(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= maxValue {
		return s
	}
	return string([]rune(s)[:maxValue-1]) + "…"
}
