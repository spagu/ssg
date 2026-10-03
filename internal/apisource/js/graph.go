package js

import (
	"strings"

	"github.com/tdewolff/parse/v2/js"
)

// exportRef says where an exported or imported name comes from. With spec
// empty, name is a local binding of the same file; otherwise name is the
// export of the module spec names, "*" meaning its whole namespace.
type exportRef struct {
	spec string
	name string
}

// collect reads the top-level statements of a file into its tables.
func (f *file) collect(ld *loader, stmts []js.IStmt) {
	for _, st := range stmts {
		switch n := st.(type) {
		case *js.ImportStmt:
			f.collectImport(n)
		case *js.ExportStmt:
			f.collectExport(n)
		case *js.FuncDecl:
			f.declareNamed(n.Name, n)
		case *js.ClassDecl:
			f.declareNamed(n.Name, n)
		case *js.VarDecl:
			f.collectVars(n)
		case *js.ExprStmt:
			f.collectCommonJS(ld, n)
		}
	}
}

// collectImport records the bindings an import statement creates.
func (f *file) collectImport(n *js.ImportStmt) {
	spec := f.addDep(n.Module)
	if n.Default != nil {
		f.imports[string(n.Default)] = exportRef{spec: spec, name: "default"}
	}
	for _, a := range n.List {
		f.imports[string(a.Binding)] = exportRef{spec: spec, name: aliasName(a)}
	}
}

// collectExport records what an export statement makes public.
func (f *file) collectExport(n *js.ExportStmt) {
	if n.Decl != nil {
		f.collectExportDecl(n)
		return
	}
	spec := ""
	if n.Module != nil {
		spec = f.addDep(n.Module)
	}
	for _, a := range n.List {
		if spec != "" && a.Name == nil && string(a.Binding) == "*" {
			f.stars = append(f.stars, spec)
			continue
		}
		f.exports[string(a.Binding)] = exportRef{spec: spec, name: aliasName(a)}
	}
}

// collectExportDecl records "export <declaration>" and "export default …".
func (f *file) collectExportDecl(n *js.ExportStmt) {
	public := func(local string) string {
		if n.Default {
			return "default"
		}
		return local
	}
	switch d := n.Decl.(type) {
	case *js.VarDecl:
		for _, name := range f.collectVars(d) {
			f.exports[name] = exportRef{name: name}
		}
		return
	case *js.FuncDecl:
		if d.Name != nil {
			name := f.declareNamed(d.Name, d)
			f.exports[public(name)] = exportRef{name: name}
			return
		}
	case *js.ClassDecl:
		if d.Name != nil {
			name := f.declareNamed(d.Name, d)
			f.exports[public(name)] = exportRef{name: name}
			return
		}
	case *js.Var:
		f.exports["default"] = exportRef{name: string(d.Name())}
		return
	}
	f.declare("default", "default", n.Decl, false)
	f.exports["default"] = exportRef{name: "default"}
}

// collectVars declares the plain names a const/let/var binds and returns
// them; destructuring patterns are skipped.
func (f *file) collectVars(n *js.VarDecl) []string {
	var names []string
	for _, el := range n.List {
		if v, ok := el.Binding.(*js.Var); ok {
			name := string(v.Name())
			f.declare(name, name, el.Default, n.TokenType == js.ConstToken)
			names = append(names, name)
		}
	}
	return names
}

// declareNamed declares a function or class under its own name.
func (f *file) declareNamed(v *js.Var, node js.IExpr) string {
	if v == nil {
		return ""
	}
	name := string(v.Name())
	f.declare(name, name, node, false)
	return name
}

// declare records a top-level declaration found at the site key.
func (f *file) declare(name, key string, node js.IExpr, isConst bool) *decl {
	return f.declareAt(name, name, node, isConst, f.siteOf(key))
}

// declareAt records a declaration under a local key with a known start.
func (f *file) declareAt(local, name string, node js.IExpr, isConst bool, start int) *decl {
	if d, ok := f.locals[local]; ok {
		return d
	}
	d := &decl{name: name, node: node, isConst: isConst, file: f, start: start}
	f.locals[local] = d
	return d
}

// addDep records a module specifier and returns it unquoted.
func (f *file) addDep(module []byte) string {
	spec := unquote(string(module))
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		f.deps = append(f.deps, spec)
	}
	return spec
}

// aliasName is the name an alias refers to in its source: "a" in "a as b".
func aliasName(a js.Alias) string {
	if a.Name != nil {
		return string(a.Name)
	}
	return string(a.Binding)
}
