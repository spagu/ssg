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
	// enumMembers maps an enum member's own name to its IDs: in Go a
	// constant of an enum type is a top-level name ([Sentence]), while the
	// model keeps it under its type (Style.Sentence).
	enumMembers map[string][]string
}

// NewResolver indexes every symbol of a by its local name.
func NewResolver(a *apimodel.API) *Resolver {
	r := &Resolver{ix: apimodel.NewIndex(a), byName: map[string][]string{}, enumMembers: map[string][]string{}}
	for _, p := range a.Packages {
		for _, m := range p.Modules {
			apimodel.Walk(m.Symbols, func(s, _ *apimodel.Symbol) {
				_, local, _ := strings.Cut(s.ID, "#")
				r.byName[local] = append(r.byName[local], s.ID)
				if s.Kind == apimodel.KindEnumMember {
					r.enumMembers[s.Name] = append(r.enumMembers[s.Name], s.ID)
				}
			})
		}
	}
	return r
}

// Resolve returns the ID a link target written in module fromModule means,
// trying in order: the target as a full ID; a name in fromModule (so a
// module's own Token wins over another module's); a name that exactly one
// symbol in the whole API has, or exactly one in fromModule's package; an
// enum member by its own name. "module#Name" with a module path but no
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
	ids := r.byName[target]
	if len(ids) == 1 {
		return ids[0], true
	}
	// Several packages may each have a Lexer: the one in the link's own
	// package is the one it means.
	if id, ok := onlyInPackage(ids, fromModule); ok {
		return id, true
	}
	return r.enumMember(fromModule, target)
}

// onlyInPackage returns the one ID among ids in fromModule's package.
func onlyInPackage(ids []string, fromModule string) (string, bool) {
	pkg, _, _ := strings.Cut(fromModule, "/")
	var found []string
	for _, id := range ids {
		if p, _, _ := strings.Cut(id, "/"); p == pkg && pkg != "" {
			found = append(found, id)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return "", false
}

// enumMember resolves a bare enum member name: the one in fromModule, else
// the only one in the whole API.
func (r *Resolver) enumMember(fromModule, name string) (string, bool) {
	var local []string
	for _, id := range r.enumMembers[name] {
		if apimodel.ModuleOf(id) == fromModule {
			local = append(local, id)
		}
	}
	switch {
	case len(local) == 1:
		return local[0], true
	case len(local) == 0 && len(r.enumMembers[name]) == 1:
		return r.enumMembers[name][0], true
	}
	return "", false
}
