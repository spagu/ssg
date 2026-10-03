package apipage

import (
	"github.com/spagu/ssg/internal/apimodel"
)

// group is one heading of a listing: "Classes", "Functions"…
type group struct {
	title string
	syms  []*apimodel.Symbol
}

// groupOrder is the order and title of each kind's heading.
var groupOrder = []struct {
	kinds []apimodel.Kind
	title string
}{
	{[]apimodel.Kind{apimodel.KindNamespace}, "Namespaces"},
	{[]apimodel.Kind{apimodel.KindClass}, "Classes"},
	{[]apimodel.Kind{apimodel.KindInterface}, "Interfaces"},
	{[]apimodel.Kind{apimodel.KindConstructor}, "Constructor"},
	{[]apimodel.Kind{apimodel.KindProperty, apimodel.KindAccessor}, "Properties"},
	{[]apimodel.Kind{apimodel.KindFunction}, "Functions"},
	{[]apimodel.Kind{apimodel.KindMethod}, "Methods"},
	{[]apimodel.Kind{apimodel.KindType}, "Types"},
	{[]apimodel.Kind{apimodel.KindEnum}, "Enums"},
	{[]apimodel.Kind{apimodel.KindVariable}, "Variables"},
	{[]apimodel.Kind{apimodel.KindEnumMember}, "Members"},
}

// groups splits symbols into headed groups, in groupOrder, keeping each
// group's input order; empty groups are left out.
func groups(syms []*apimodel.Symbol) []group {
	var out []group
	for _, g := range groupOrder {
		var in []*apimodel.Symbol
		for _, s := range syms {
			for _, k := range g.kinds {
				if s.Kind == k {
					in = append(in, s)
				}
			}
		}
		if len(in) > 0 {
			out = append(out, group{title: g.title, syms: in})
		}
	}
	return out
}

// Visible returns a copy of pkg without the symbols visibility and
// stability hide. visibility "public" (or "") hides internal ones; "internal"
// shows them; "all" shows everything. A symbol a doc comment marks hidden is
// never shown, whatever the setting. stability lists the non-stable levels to
// keep; empty keeps all.
func Visible(pkg *apimodel.Package, visibility string, stability []string) *apimodel.Package {
	keep := func(s *apimodel.Symbol) bool {
		if s.Flags.Internal && visibility != "internal" && visibility != "all" {
			return false
		}
		if s.Flags.Stability != apimodel.Stable && len(stability) > 0 {
			for _, st := range stability {
				if string(s.Flags.Stability) == st {
					return true
				}
			}
			return false
		}
		return true
	}
	out := *pkg
	out.Modules = make([]*apimodel.Module, len(pkg.Modules))
	for i, m := range pkg.Modules {
		mc := *m
		mc.Symbols = filterSymbols(m.Symbols, keep)
		out.Modules[i] = &mc
	}
	return &out
}

// filterSymbols keeps the symbols keep accepts, filtering members too.
func filterSymbols(syms []*apimodel.Symbol, keep func(*apimodel.Symbol) bool) []*apimodel.Symbol {
	var out []*apimodel.Symbol
	for _, s := range syms {
		if !keep(s) {
			continue
		}
		c := *s
		c.Members = filterSymbols(s.Members, keep)
		out = append(out, &c)
	}
	return out
}
