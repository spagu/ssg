package python

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// dataclassInfo says whether a class is a @dataclass and whether frozen.
type dataclassInfo struct {
	found  bool
	frozen bool
}

// dataclassOf reads @dataclass or @dataclass(frozen=True).
func dataclassOf(decos []decorator) dataclassInfo {
	for _, d := range decos {
		if lastSegment(d.name) == "dataclass" {
			return dataclassInfo{found: true, frozen: strings.Contains(joinToks(d.args), "frozen=True")}
		}
	}
	return dataclassInfo{}
}

// dataclassInit builds the __init__ a dataclass generates: one parameter
// per annotated field in order, ClassVar fields and field(init=False)
// left out, keyword-only after a KW_ONLY marker.
func dataclassInit(sym *apimodel.Symbol, cls *stmt, res resolver) *apimodel.Symbol {
	sig := &apimodel.Signature{}
	codes := []string{"self"}
	for _, st := range mergeStmts(cls.body) {
		head := lastSegment(headName(st.annot))
		value := joinToks(st.value)
		isField := strings.HasPrefix(value, "field(") || strings.HasPrefix(value, "dataclasses.field(")
		switch {
		case st.kind != sAssign || len(st.annot) == 0 || head == "ClassVar":
			continue
		case head == "KW_ONLY":
			codes = append(codes, "*")
			continue
		case isField && strings.Contains(value, "init=False"):
			continue
		}
		p := &apimodel.Param{Name: st.name, Type: unwrap(parseType(st.annot, res), "InitVar"), Default: value,
			Optional: value != "" && (!isField || strings.Contains(value, "default"))}
		sig.Params = append(sig.Params, p)
		codes = append(codes, assignCode(st.name, st.annot, st.value, "="))
	}
	sig.Code = "def __init__(" + strings.Join(codes, ", ") + ") -> None"
	return &apimodel.Symbol{ID: apimodel.MemberID(sym.ID, "__init__"), Name: "__init__",
		Kind: apimodel.KindConstructor, Source: &apimodel.Source{Line: cls.line}, Signatures: []*apimodel.Signature{sig}}
}

// classParamDocs documents constructor parameters from the class
// docstring (NumPy and Google put them there) where __init__ does not.
func classParamDocs(ctor *apimodel.Symbol, info *docInfo, res resolver) {
	for _, sig := range ctor.Signatures {
		for _, p := range sig.Params {
			dp := lookup(info.params, p.Name)
			if dp == nil {
				dp = lookup(info.attrs, p.Name)
			}
			if dp == nil {
				continue
			}
			p.Doc = firstNonEmpty(p.Doc, dp.text)
			if p.Type == nil && dp.typ != "" {
				p.Type = parseTypeText(dp.typ, res)
			}
		}
	}
}

// attrDoc documents an attribute from the class docstring's Attributes
// section (or, for a dataclass, its parameters) when it has no doc.
func attrDoc(p *apimodel.Symbol, info *docInfo, dataclass bool) {
	if p.Doc != nil {
		return
	}
	dp := lookup(info.attrs, p.Name)
	if dp == nil && dataclass {
		dp = lookup(info.params, p.Name)
	}
	if dp != nil && dp.text != "" {
		p.Doc = &apimodel.Doc{Summary: dp.text}
	}
}
