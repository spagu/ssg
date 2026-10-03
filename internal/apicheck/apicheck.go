// Package apicheck finds documentation problems in an API model (GO-110):
// the gaps that make a reference page useless without anyone noticing — an
// export nobody described, a parameter with no word about it, a deprecation
// that does not say what to use instead, an example that is not code.
//
// It reads only the model. Problems an extractor or the link rewriter finds
// (an @param naming no parameter, a link to nothing) arrive separately as
// diagnostics, and the generator reports both together.
package apicheck

import (
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// Finding is one problem, located by symbol and, when known, source.
type Finding struct {
	ID      string // symbol or member ID
	Where   string // "file:line" when the model knows it, else the ID
	Message string
}

// Check returns every finding in a, sorted by location. Internal symbols are
// checked too: the setting that hides them from readers does not excuse them
// from maintainers.
func Check(a *apimodel.API) []Finding {
	var out []Finding
	for _, p := range a.Packages {
		for _, m := range p.Modules {
			apimodel.Walk(m.Symbols, func(s, parent *apimodel.Symbol) {
				out = append(out, checkSymbol(s, parent)...)
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Where != out[j].Where {
			return out[i].Where < out[j].Where
		}
		return out[i].Message < out[j].Message
	})
	return out
}

// checkSymbol applies the rules to one symbol.
func checkSymbol(s, parent *apimodel.Symbol) []Finding {
	var out []Finding
	add := func(msg string) {
		out = append(out, Finding{ID: s.ID, Where: where(s), Message: s.Name + ": " + msg})
	}
	if needsDescription(s, parent) && (s.Doc == nil || strings.TrimSpace(s.Doc.Summary) == "") {
		add("no description")
	}
	if s.Doc != nil {
		if s.Doc.Deprecated != nil && strings.TrimSpace(*s.Doc.Deprecated) == "" {
			add("@deprecated does not say what to use instead")
		}
		for _, ex := range s.Doc.Examples {
			if !strings.Contains(ex, "```") && !strings.Contains(ex, "~~~") {
				add("an @example is not a code block")
			}
		}
	}
	for _, sig := range s.Signatures {
		for _, p := range sig.Params {
			if strings.TrimSpace(p.Doc) == "" && s.Doc != nil {
				add("parameter " + p.Name + " has no description")
			}
		}
	}
	return out
}

// needsDescription is every symbol a reader looks up by name. Enum members,
// constructors and the members of an undocumented owner are spared: the
// owner's own finding already covers them, and a list of "Fast: no
// description" for every member says nothing new.
func needsDescription(s, parent *apimodel.Symbol) bool {
	switch s.Kind {
	case apimodel.KindEnumMember, apimodel.KindConstructor:
		return false
	}
	return parent == nil || (parent.Doc != nil && parent.Doc.Summary != "")
}

// where is "file:line" or, without a source, the ID.
func where(s *apimodel.Symbol) string {
	if s.Source == nil || s.Source.File == "" {
		return s.ID
	}
	return s.Source.File + ":" + itoa(s.Source.Line)
}

// itoa is strconv.Itoa for non-negative line numbers.
func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for ; n > 0; n /= 10 {
		i--
		b[i] = byte('0' + n%10)
	}
	return string(b[i:])
}
