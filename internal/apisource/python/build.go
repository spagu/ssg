package python

import (
	"regexp"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// buildModule turns a planned module into its model: its docstring and
// its exports, sorted by name.
func (x *extraction) buildModule(m *module) *apimodel.Module {
	out := &apimodel.Module{ID: m.id, Path: m.path, Symbols: []*apimodel.Symbol{}}
	out.Doc = finishDoc(docFor(m.doc), nil)
	for _, e := range m.exports {
		out.Symbols = append(out.Symbols, x.symbol(e))
	}
	sort.Slice(out.Symbols, func(i, j int) bool { return out.Symbols[i].Name < out.Symbols[j].Name })
	return out
}

// symbol builds the model of one export from its definition, resolving
// types in the scope of the module that defines it.
func (x *extraction) symbol(e export) *apimodel.Symbol {
	s, res := e.o.s, x.resolverFor(e.o.m)
	src := &apimodel.Source{File: e.o.m.file, Line: s.line}
	switch s.kind {
	case sDef:
		info := docFor(docSource(s))
		return &apimodel.Symbol{ID: e.id, Name: e.name, Kind: apimodel.KindFunction, Source: src,
			Signatures: signatures(s, false, info, res), Flags: apimodel.Flags{Async: s.async},
			Doc: finishDoc(info, s.decorators)}
	case sClass:
		return x.class(e.id, e.name, s, src, res)
	case sTypeAlias:
		return &apimodel.Symbol{ID: e.id, Name: e.name, Kind: apimodel.KindType, Source: src,
			TypeParams: typeParams(s.typeParams, res), Type: parseType(s.value, res),
			Code: "type " + s.name + typeParamCode(s.typeParams) + " = " + joinToks(s.value),
			Doc:  finishDoc(docFor(s.doc), nil)}
	}
	return variable(e.id, e.name, s, src, res)
}

// allCaps matches a constant's name: MAX_DEPTH.
var allCaps = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// variable builds a module-level assignment: a type alias when annotated
// TypeAlias, else a variable, read-only when ALL_CAPS or Final.
func variable(id, name string, s *stmt, src *apimodel.Source, res resolver) *apimodel.Symbol {
	sym := &apimodel.Symbol{ID: id, Name: name, Kind: apimodel.KindVariable, Source: src,
		Code: assignCode(s.name, s.annot, s.value, " = "), Doc: finishDoc(docFor(s.doc), nil)}
	head := lastSegment(headName(s.annot))
	if head == "TypeAlias" {
		sym.Kind, sym.Type = apimodel.KindType, parseType(s.value, res)
		return sym
	}
	sym.Type = unwrap(parseType(s.annot, res), "Final")
	sym.Flags.Readonly = allCaps.MatchString(s.name) || head == "Final"
	return sym
}

// headName returns the dotted name an annotation starts with: "ClassVar"
// for "typing.ClassVar[int]" → "typing.ClassVar".
func headName(toks []token) string {
	var b strings.Builder
	for i, t := range toks {
		if t.kind != tName && (!t.is(".") || i == 0) {
			break
		}
		b.WriteString(t.text)
	}
	return b.String()
}

// lastSegment returns the part of a dotted name after the last ".".
func lastSegment(name string) string { return name[strings.LastIndex(name, ".")+1:] }

// unwrap returns the argument of a wrapper type such as Final[int] or
// ClassVar[int], or the type itself.
func unwrap(t *apimodel.TypeRef, wrapper string) *apimodel.TypeRef {
	if t != nil && t.Kind == apimodel.TypeName && lastSegment(t.Name) == wrapper && len(t.Args) == 1 {
		return t.Args[0]
	}
	return t
}

// docFor parses a docstring, or returns an empty docInfo for none.
func docFor(raw string) *docInfo {
	if strings.TrimSpace(raw) == "" {
		return &docInfo{}
	}
	return parseDocstring(raw)
}

// finishDoc completes a symbol's Doc with what its decorators say
// (@deprecated) and returns nil when it says nothing.
func finishDoc(info *docInfo, decos []decorator) *apimodel.Doc {
	d := info.doc
	for _, deco := range decos {
		if lastSegment(deco.name) == "deprecated" && d.Deprecated == nil {
			msg := ""
			for _, t := range deco.args {
				if t.kind != tString {
					break
				}
				msg += decodeString(t.text)
			}
			d.Deprecated = &msg
		}
	}
	if emptyDoc(&d) {
		return nil
	}
	return &d
}
