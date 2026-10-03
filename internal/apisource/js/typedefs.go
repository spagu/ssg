package js

import "github.com/spagu/ssg/internal/apidoc"

// typeDecl is a type declared in a comment: @typedef or @callback.
type typeDecl struct {
	name     string
	file     *file
	line     int
	comment  *apidoc.Comment // the whole comment, for @template and modifiers
	typedef  *apidoc.Typedef
	callback *apidoc.Callback
}

// at locates the type declaration for diagnostics.
func (t *typeDecl) at() where { return where{file: t.file.path, line: t.line} }

// typeDecls lists the typedefs and callbacks in the file's doc comments,
// in source order. Hidden and private ones are left out.
func (f *file) typeDecls() []*typeDecl {
	var out []*typeDecl
	for i, dc := range f.scan.docs {
		c := f.docComment(i)
		if skipped(c) {
			continue
		}
		for j := range c.Typedefs {
			td := &c.Typedefs[j]
			if td.Name != "" {
				out = append(out, &typeDecl{name: td.Name, file: f, line: dc.line, comment: c, typedef: td})
			}
		}
		for j := range c.Callbacks {
			cb := &c.Callbacks[j]
			if cb.Name != "" {
				out = append(out, &typeDecl{name: cb.Name, file: f, line: dc.line, comment: c, callback: cb})
			}
		}
	}
	return out
}
