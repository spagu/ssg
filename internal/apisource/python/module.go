package python

import (
	"io/fs"
	"strings"

	"github.com/spagu/ssg/internal/apisource"
)

// module is one parsed source file.
type module struct {
	file    string // relative to the root, "/" separators
	path    string // module path: "textkit/lexer"
	id      string
	doc     string
	stmts   []*stmt // top-level declarations, merged by name
	defs    map[string]*stmt
	imports map[string]importName // by local name
	stars   []string              // modules of "from x import *"
	all     []string              // the literal __all__, when hasAll
	hasAll  bool
	allLine int
	exports []export
}

// extraction is the state of one Extract run.
type extraction struct {
	fsys      fs.FS
	modules   map[string]*module // by module path
	ordered   []*module          // by ID
	index     map[string]string  // origin key → documented symbol ID
	resolvers map[*module]resolver
	diags     []apisource.Diagnostic
}

// diag records a diagnostic.
func (x *extraction) diag(sev apisource.Severity, file string, line int, msg string) {
	x.diags = append(x.diags, apisource.Diagnostic{Severity: sev, File: file, Line: line, Message: msg})
}

// load reads, scans and parses one file. Scanner problems are errors, but
// whatever could be read is kept.
func (x *extraction) load(file string) {
	data, err := fs.ReadFile(x.fsys, file)
	if err != nil {
		x.diag(apisource.Error, file, 0, "cannot read: "+err.Error())
		return
	}
	toks, lexDiags := tokenize(string(data))
	for _, d := range lexDiags {
		x.diag(apisource.Error, file, d.line, d.msg)
	}
	mp := modulePath(x.fsys, file)
	if prev, dup := x.modules[mp]; dup {
		x.diag(apisource.Warning, file, 0, "module "+mp+" is already read from "+prev.file)
		return
	}
	pf := parseFile(toks, packageOf(file, mp))
	m := &module{file: file, path: mp, doc: pf.doc, defs: map[string]*stmt{}, imports: map[string]importName{}}
	for _, imp := range pf.imports {
		if imp.name == "*" {
			m.stars = append(m.stars, imp.module)
			continue
		}
		m.imports[imp.alias] = imp
	}
	var decls []*stmt
	for _, s := range pf.stmts {
		if s.name == "__all__" {
			m.readAll(s)
		} else {
			decls = append(decls, s)
		}
	}
	for _, s := range mergeStmts(decls) {
		m.stmts = append(m.stmts, s)
		m.defs[s.name] = s
	}
	x.modules[mp] = m
}

// readAll takes the names of a literal __all__ list or tuple; anything
// else turns __all__ off, so every public name is listed.
func (m *module) readAll(s *stmt) {
	names, ok := stringList(s.value)
	switch {
	case !ok:
		m.hasAll, m.all = false, nil
	case s.augmented:
		m.all = append(m.all, names...)
	default:
		m.hasAll, m.all, m.allLine = true, names, s.line
	}
}

// stringList reads "[" or "(" then string literals then the closer.
func stringList(toks []token) ([]string, bool) {
	if !enclosed(toks, "[", "]") && !enclosed(toks, "(", ")") {
		return nil, false
	}
	var out []string
	for _, part := range splitTop(toks[1:len(toks)-1], ",") {
		if len(part) != 1 || part[0].kind != tString {
			return nil, false
		}
		out = append(out, decodeString(part[0].text))
	}
	return out, true
}

// mergeStmts folds what Python would see under one name: a later
// definition replaces an earlier one (keeping its place), @overload
// variants join the implementation, and @name.setter marks a property
// writable. Setters and deleters are not listed on their own.
func mergeStmts(stmts []*stmt) []*stmt {
	var out []*stmt
	at := map[string]int{}
	pending := map[string][]*stmt{}
	for _, s := range stmts {
		if s.kind == sDef {
			if name, accessor := accessorOf(s); accessor != "" {
				if i, ok := at[name]; ok && accessor == "setter" {
					out[i].hasSetter = true
				}
				continue
			}
			if hasDecorator(s.decorators, "overload") {
				pending[s.name] = append(pending[s.name], s)
				p := pending[s.name]
				s = &stmt{kind: sDef, name: s.name, line: p[0].line, async: s.async, decorators: s.decorators, overloads: append([]*stmt(nil), p...)}
			} else if p := pending[s.name]; len(p) > 0 {
				s.overloads = p
				delete(pending, s.name)
			}
		}
		if i, ok := at[s.name]; ok {
			out[i] = s
			continue
		}
		at[s.name] = len(out)
		out = append(out, s)
	}
	return out
}

// accessorOf returns the property name and "setter" or "deleter" for a
// def decorated "@name.setter" or "@name.deleter".
func accessorOf(s *stmt) (string, string) {
	for _, d := range s.decorators {
		if name, acc, ok := strings.Cut(d.name, "."); ok && (acc == "setter" || acc == "deleter") {
			return name, acc
		}
	}
	return "", ""
}

// hasDecorator reports whether a decorator's last name segment is one of
// names ("abc.abstractmethod" matches "abstractmethod").
func hasDecorator(decos []decorator, names ...string) bool {
	for _, d := range decos {
		last := d.name[strings.LastIndex(d.name, ".")+1:]
		for _, n := range names {
			if last == n {
				return true
			}
		}
	}
	return false
}

// enclosed reports whether tokens start with open and end with closer.
func enclosed(toks []token, open, closer string) bool {
	return len(toks) >= 2 && toks[0].is(open) && toks[len(toks)-1].is(closer)
}
