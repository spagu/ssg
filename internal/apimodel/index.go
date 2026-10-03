package apimodel

// Index finds modules and symbols by ID. It is built once from a finished
// model; pages, link resolution and MCP lookups read it.
type Index struct {
	modules map[string]*Module
	symbols map[string]*Symbol
	parent  map[string]string // member ID → owner ID
}

// NewIndex indexes every module, symbol and member of a.
func NewIndex(a *API) *Index {
	ix := &Index{modules: map[string]*Module{}, symbols: map[string]*Symbol{}, parent: map[string]string{}}
	for _, p := range a.Packages {
		for _, m := range p.Modules {
			ix.modules[m.ID] = m
			Walk(m.Symbols, func(s *Symbol, parent *Symbol) {
				ix.symbols[s.ID] = s
				if parent != nil {
					ix.parent[s.ID] = parent.ID
				}
			})
		}
	}
	return ix
}

// Module returns the module with this ID.
func (ix *Index) Module(id string) (*Module, bool) {
	m, ok := ix.modules[id]
	return m, ok
}

// Symbol returns the symbol or member with this ID.
func (ix *Index) Symbol(id string) (*Symbol, bool) {
	s, ok := ix.symbols[id]
	return s, ok
}

// Parent returns the ID of the symbol a member belongs to, or "" for a
// top-level symbol.
func (ix *Index) Parent(id string) string { return ix.parent[id] }

// Len is the number of symbols and members indexed.
func (ix *Index) Len() int { return len(ix.symbols) }

// Walk calls visit for every symbol and, depth first, its members, with the
// owning symbol (nil at the top level).
func Walk(syms []*Symbol, visit func(s, parent *Symbol)) {
	walk(syms, nil, visit)
}

func walk(syms []*Symbol, parent *Symbol, visit func(s, parent *Symbol)) {
	for _, s := range syms {
		visit(s, parent)
		walk(s.Members, s, visit)
	}
}
