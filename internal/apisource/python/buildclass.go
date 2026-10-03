package python

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// enumBases are the base classes that make a class an enum.
var enumBases = map[string]bool{"Enum": true, "IntEnum": true, "StrEnum": true, "Flag": true, "IntFlag": true}

// propertyDecorators turn a method into a property.
var propertyDecorators = []string{"property", "cached_property"}

// class builds a class: an enum for Enum bases, an interface for Protocol,
// else a class (abstract with ABC or ABCMeta). Keyword arguments such as
// metaclass= stay in Code but not in Extends.
func (x *extraction) class(id, name string, s *stmt, src *apimodel.Source, res resolver) *apimodel.Symbol {
	info := docFor(s.doc)
	sym := &apimodel.Symbol{ID: id, Name: name, Kind: apimodel.KindClass, Source: src,
		TypeParams: typeParams(s.typeParams, res), Code: "class " + s.name + typeParamCode(s.typeParams)}
	var bases []string
	for _, b := range s.bases {
		bases = append(bases, joinToks(b))
		head := lastSegment(headName(b))
		if indexTop(b, "=") >= 0 {
			sym.Flags.Abstract = sym.Flags.Abstract || strings.HasSuffix(joinToks(b), "ABCMeta")
			continue
		}
		switch {
		case enumBases[head]:
			sym.Kind = apimodel.KindEnum
		case head == "Protocol" && sym.Kind == apimodel.KindClass:
			sym.Kind = apimodel.KindInterface
		case head == "ABC":
			sym.Flags.Abstract = true
		}
		sym.Extends = append(sym.Extends, parseType(b, res))
	}
	if len(bases) > 0 {
		sym.Code += "(" + strings.Join(bases, ", ") + ")"
	}
	sym.Members = members(sym, s, info, res)
	for _, m := range sym.Members {
		m.Source.File = src.File
	}
	sym.Doc = finishDoc(info, s.decorators)
	return sym
}

// members builds a class's public members: the constructor, then
// attributes and properties, then methods, each in declaration order.
// A dataclass without __init__ gets one built from its fields.
func members(sym *apimodel.Symbol, cls *stmt, info *docInfo, res resolver) []*apimodel.Symbol {
	dataclass := dataclassOf(cls.decorators)
	var ctor *apimodel.Symbol
	var props, methods []*apimodel.Symbol
	for _, st := range mergeStmts(cls.body) {
		switch st.kind {
		case sDef:
			m := method(sym.ID, st, res)
			switch {
			case m == nil:
			case m.Kind == apimodel.KindConstructor:
				ctor = m
			case m.Kind == apimodel.KindProperty:
				props = append(props, m)
			default:
				methods = append(methods, m)
			}
		case sAssign:
			if m := attribute(sym.ID, st, res, sym.Kind == apimodel.KindEnum, dataclass.frozen); m != nil {
				props = append(props, m)
			}
		}
	}
	if ctor == nil && dataclass.found {
		ctor = dataclassInit(sym, cls, res)
	}
	var out []*apimodel.Symbol
	if ctor != nil {
		classParamDocs(ctor, info, res)
		out = append(out, ctor)
	}
	for _, p := range props {
		attrDoc(p, info, dataclass.found)
	}
	return append(append(out, props...), methods...)
}

// method builds a def in a class body: __init__ as the constructor,
// @property as a property, else a method; other names starting with "_"
// are private and left out.
func method(parentID string, st *stmt, res resolver) *apimodel.Symbol {
	if st.name != "__init__" && strings.HasPrefix(st.name, "_") {
		return nil
	}
	info := docFor(docSource(st))
	src := &apimodel.Source{Line: st.line}
	m := &apimodel.Symbol{ID: apimodel.MemberID(parentID, st.name), Name: st.name, Kind: apimodel.KindMethod, Source: src}
	switch {
	case st.name == "__init__":
		m.Kind, m.Signatures = apimodel.KindConstructor, signatures(st, true, info, res)
		for _, sig := range m.Signatures {
			sig.Returns = nil
		}
	case hasDecorator(st.decorators, propertyDecorators...):
		m.Kind, m.Type = apimodel.KindProperty, parseType(st.returns, res)
		if m.Type == nil && info.returnType != "" {
			m.Type = parseTypeText(info.returnType, res)
		}
		m.Flags.Readonly = !st.hasSetter
		m.Code = assignCode(st.name, st.returns, nil, "")
	default:
		static := hasDecorator(st.decorators, "staticmethod")
		classMethod := hasDecorator(st.decorators, "classmethod")
		m.Signatures = signatures(st, !static, info, res)
		m.Flags = apimodel.Flags{Static: static || classMethod, Async: st.async,
			Abstract: hasDecorator(st.decorators, "abstractmethod")}
		if classMethod {
			info.doc.Tags = append(info.doc.Tags, apimodel.Tag{Name: "classmethod"})
		}
	}
	m.Doc = finishDoc(info, st.decorators)
	return m
}

// attribute builds a class-level assignment: an enum member in an enum,
// else a property (static for ClassVar, read-only for Final, ALL_CAPS or
// a frozen dataclass).
func attribute(parentID string, st *stmt, res resolver, enum, frozen bool) *apimodel.Symbol {
	if strings.HasPrefix(st.name, "_") || st.augmented {
		return nil
	}
	m := &apimodel.Symbol{ID: apimodel.MemberID(parentID, st.name), Name: st.name, Kind: apimodel.KindProperty,
		Source: &apimodel.Source{Line: st.line}, Code: assignCode(st.name, st.annot, st.value, " = "),
		Doc: finishDoc(docFor(st.doc), nil)}
	if enum && len(st.annot) == 0 {
		m.Kind = apimodel.KindEnumMember
		return m
	}
	head := lastSegment(headName(st.annot))
	m.Type = unwrap(unwrap(parseType(st.annot, res), "ClassVar"), "Final")
	m.Flags.Static = head == "ClassVar"
	m.Flags.Readonly = frozen || head == "Final" || allCaps.MatchString(st.name)
	return m
}
