package python

import "strings"

// keywords are the reserved words that can start a line followed by ":"
// or "=" without being an assignment target.
var keywords = map[string]bool{
	"else": true, "try": true, "finally": true, "except": true, "lambda": true,
	"if": true, "elif": true, "while": true, "for": true, "with": true, "return": true,
	"pass": true, "break": true, "continue": true, "del": true, "global": true,
	"nonlocal": true, "raise": true, "yield": true, "import": true, "assert": true,
	"match": true, "case": true, "True": true, "False": true, "None": true,
}

// assignment reads "name = value", "name: T = value", "name: T" or
// "name += value"; further targets ("a = b = 1") stay in the value.
func (p *parser) assignment() *stmt {
	name := p.take()
	s := &stmt{kind: sAssign, name: name.text, line: name.line}
	switch op := p.take(); op.text {
	case ":":
		s.annot = p.upTo("=")
		if p.toks[p.i-1].is("=") {
			s.value = p.upTo(";")
		}
	case "+=":
		s.augmented = true
		s.value = p.upTo(";")
	default:
		s.value = p.upTo(";")
	}
	p.endSimple()
	return s
}

// fromImport reads "from .mod import a, b as c" and records each name
// with the module path it resolves to.
func (p *parser) fromImport() {
	line := p.take().line
	level := 0
	for t := p.peek(0); t.is(".") || t.is("..."); t = p.peek(0) {
		level += len(p.take().text)
	}
	module := strings.ReplaceAll(p.dottedName(), ".", "/")
	if p.peek(0).is("import") {
		p.i++
		var names []token
		if p.peek(0).is("(") {
			p.i++
			names = p.until(")")
		} else {
			names = p.upTo(";")
		}
		target := resolveRelative(p.pkgPath, level, module)
		for _, part := range splitTop(names, ",") {
			if imp, ok := importPart(part, target, line); ok {
				p.imports = append(p.imports, imp)
			}
		}
	}
	p.skipStatement()
}

// importPart reads "name" or "name as alias" (or "*") of a from-import.
func importPart(part []token, module string, line int) (importName, bool) {
	if len(part) == 0 || (part[0].kind != tName && !part[0].is("*")) {
		return importName{}, false
	}
	imp := importName{module: module, name: part[0].text, alias: part[0].text, line: line}
	if len(part) == 3 && part[1].is("as") {
		imp.alias = part[2].text
	}
	return imp, true
}

// resolveRelative turns a relative import (level dots, then module) made
// from package pkg into a module path; level 0 is absolute.
func resolveRelative(pkg string, level int, module string) string {
	if level == 0 {
		return module
	}
	base := pkg
	for range level - 1 {
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[:i]
		} else {
			base = ""
		}
	}
	switch {
	case module == "":
		return base
	case base == "":
		return module
	}
	return base + "/" + module
}

// splitTop splits tokens at a separator outside brackets. Empty runs (a
// trailing comma) are dropped.
func splitTop(toks []token, sep string) [][]token {
	var out [][]token
	depth, start := 0, 0
	flush := func(end int) {
		if end > start {
			out = append(out, toks[start:end])
		}
	}
	for i, t := range toks {
		switch {
		case t.is("(") || t.is("[") || t.is("{"):
			depth++
		case t.is(")") || t.is("]") || t.is("}"):
			depth--
		case depth == 0 && t.is(sep):
			flush(i)
			start = i + 1
		}
	}
	flush(len(toks))
	return out
}

// joinToks renders tokens as source text with whitespace normalised: one
// space where the source had any, none just inside brackets or before a
// comma. String literals stay verbatim.
func joinToks(toks []token) string {
	var b strings.Builder
	for i, t := range toks {
		if i > 0 {
			prev := toks[i-1]
			gap := t.start > prev.end
			tight := prev.is("(") || prev.is("[") || prev.is("{") ||
				t.is(")") || t.is("]") || t.is("}") || t.is(",")
			if gap && !tight {
				b.WriteByte(' ')
			}
		}
		b.WriteString(t.text)
	}
	return b.String()
}

// indexTop returns the index of the first separator outside brackets, or -1.
func indexTop(toks []token, sep string) int {
	depth := 0
	for i, t := range toks {
		switch {
		case t.is("(") || t.is("[") || t.is("{"):
			depth++
		case t.is(")") || t.is("]") || t.is("}"):
			depth--
		case depth == 0 && t.is(sep):
			return i
		}
	}
	return -1
}
