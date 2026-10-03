package python

import (
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// origin is where a name is defined: a declaration and its module.
type origin struct {
	m *module
	s *stmt
}

// key identifies an origin across modules.
func (o origin) key() string { return o.m.path + "#" + o.s.name }

// export is one name a module lists, resolved to its definition.
type export struct {
	name string // the public name in the listing module
	id   string
	o    origin
}

// plan gives every module its ID and exports, then indexes which
// documented symbol each definition is, for type links.
func (x *extraction) plan(pkg string) {
	for _, m := range x.modules {
		m.id = apimodel.ModuleID(pkg, m.path)
		x.ordered = append(x.ordered, m)
	}
	sort.Slice(x.ordered, func(i, j int) bool { return x.ordered[i].id < x.ordered[j].id })
	for _, m := range x.ordered {
		m.exports = x.exportsOf(m)
	}
	x.index = map[string]string{}
	for _, own := range []bool{true, false} {
		for _, m := range x.ordered {
			for _, e := range m.exports {
				if _, taken := x.index[e.o.key()]; !taken && (e.o.m == m) == own {
					x.index[e.o.key()] = e.id
				}
			}
		}
	}
}

// exportsOf lists what a module makes public: its literal __all__, else
// every top-level declaration whose name does not start with "_".
func (x *extraction) exportsOf(m *module) []export {
	var out []export
	if !m.hasAll {
		for _, s := range m.stmts {
			if !strings.HasPrefix(s.name, "_") {
				out = append(out, export{name: s.name, id: apimodel.SymbolID(m.id, s.name), o: origin{m, s}})
			}
		}
		return out
	}
	seen := map[string]bool{}
	for _, name := range m.all {
		if seen[name] {
			continue
		}
		seen[name] = true
		o, ok := x.lookup(m, name, map[string]bool{})
		if !ok {
			if !x.isModule(m, name) {
				x.diag(apisource.Warning, m.file, m.allLine, "__all__ lists "+name+", which is not defined or imported here")
			}
			continue
		}
		if o.m != m && !privateModule(o.m) && o.s.name == name {
			// Documented where it is defined; a second copy here would be
			// the same page twice under two URLs. A renamed re-export
			// ("Thing as Alias") is a name of its own and stays.
			continue
		}
		out = append(out, export{name: name, id: apimodel.SymbolID(m.id, name), o: o})
	}
	return out
}

// privateModule reports whether a module is private by Python convention:
// a path segment starting with "_" ("textkit/_impl"). Its names reach the
// reference through the public modules that re-export them.
func privateModule(m *module) bool {
	for _, seg := range strings.Split(m.path, "/") {
		dunder := strings.HasPrefix(seg, "__") && strings.HasSuffix(seg, "__")
		if strings.HasPrefix(seg, "_") && !dunder {
			return true
		}
	}
	return false
}

// isModule reports whether name, as seen from m, is a submodule or an
// imported module rather than a symbol.
func (x *extraction) isModule(m *module, name string) bool {
	if imp, ok := m.imports[name]; ok {
		_, sub := x.modules[joinPath(imp.module, imp.name)]
		return sub
	}
	_, sub := x.modules[joinPath(m.path, name)]
	return sub
}

// joinPath joins module path segments, ignoring empty ones.
func joinPath(a, b string) string { return joinNonEmpty("/", a, b) }

// lookup follows a name from a module to its definition, through
// from-imports and star imports within the package. seen stops cycles.
func (x *extraction) lookup(m *module, name string, seen map[string]bool) (origin, bool) {
	if s, ok := m.defs[name]; ok {
		return origin{m, s}, true
	}
	k := m.path + "#" + name
	if seen[k] {
		return origin{}, false
	}
	seen[k] = true
	if imp, ok := m.imports[name]; ok {
		if t := x.modules[imp.module]; t != nil {
			return x.lookup(t, imp.name, seen)
		}
		return origin{}, false
	}
	for _, star := range m.stars {
		if t := x.modules[star]; t != nil && t.public(name) {
			if o, ok := x.lookup(t, name, seen); ok {
				return o, true
			}
		}
	}
	return origin{}, false
}

// public reports whether a star import of the module brings name.
func (m *module) public(name string) bool {
	if m.hasAll {
		for _, n := range m.all {
			if n == name {
				return true
			}
		}
		return false
	}
	return !strings.HasPrefix(name, "_")
}

// resolverFor maps names used in a module's annotations to the IDs of
// documented symbols.
func (x *extraction) resolverFor(m *module) resolver {
	if r, ok := x.resolvers[m]; ok {
		return r
	}
	r := func(name string) string {
		if o, ok := x.lookup(m, name, map[string]bool{}); ok {
			return x.index[o.key()]
		}
		return ""
	}
	x.resolvers[m] = r
	return r
}
