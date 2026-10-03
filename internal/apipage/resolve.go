package apipage

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// Resolver finds the symbol an inline link means (GO-106/108). A link is
// written the way a reader thinks of the name — "Lexer", "Lexer.next",
// "lex#Lexer" — not as a full ID, so the answer depends on where the link is.
type Resolver struct {
	ix     *apimodel.Index
	byName map[string][]string // local name ("Lexer.next") → IDs
}

// NewResolver indexes every symbol of a by its local name.
func NewResolver(a *apimodel.API) *Resolver {
	r := &Resolver{ix: apimodel.NewIndex(a), byName: map[string][]string{}}
	for _, p := range a.Packages {
		for _, m := range p.Modules {
			apimodel.Walk(m.Symbols, func(s, _ *apimodel.Symbol) {
				_, local, _ := strings.Cut(s.ID, "#")
				r.byName[local] = append(r.byName[local], s.ID)
			})
		}
	}
	return r
}

// Resolve returns the ID a link target written in module fromModule means,
// trying in order: the target as a full ID; a name in fromModule (so a
// module's own Token wins over another module's); a name that exactly one
// symbol in the whole API has. "module#Name" with a module path but no
// package is tried against fromModule's package. An ambiguous or unknown
// name is not resolved — a wrong link is worse than a reported one.
func (r *Resolver) Resolve(fromModule, target string) (string, bool) {
	target = strings.TrimSpace(target)
	target = strings.TrimSuffix(target, "()")
	if target == "" {
		return "", false
	}
	if _, ok := r.ix.Symbol(target); ok {
		return target, true
	}
	if mod, name, ok := strings.Cut(target, "#"); ok {
		pkg, _, _ := strings.Cut(fromModule, "/")
		id := apimodel.SymbolID(apimodel.ModuleID(pkg, mod), name)
		_, found := r.ix.Symbol(id)
		return id, found
	}
	if fromModule != "" {
		id := apimodel.SymbolID(fromModule, target)
		if _, ok := r.ix.Symbol(id); ok {
			return id, true
		}
	}
	if ids := r.byName[target]; len(ids) == 1 {
		return ids[0], true
	}
	return "", false
}
