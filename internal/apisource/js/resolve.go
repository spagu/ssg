package js

import "sort"

// target is what an exported name resolves to: a declaration, or the whole
// namespace of a module ("export * as ns").
type target struct {
	decl *decl
	ns   *file
}

// resolver follows exports through the module graph. Seen marks the
// (file, name) pairs of one lookup, so cycles end instead of looping.
type resolver struct {
	ld   *loader
	seen map[[2]string]bool
}

// resolve returns what the public name of f refers to.
func (ld *loader) resolve(f *file, name string) (target, bool) {
	r := &resolver{ld: ld, seen: map[[2]string]bool{}}
	return r.export(f, name)
}

// export resolves a public name of f: its own exports first, then the
// modules it re-exports with "export *" ("default" is never re-exported
// that way).
func (r *resolver) export(f *file, name string) (target, bool) {
	key := [2]string{f.path, name}
	if r.seen[key] {
		return target{}, false
	}
	r.seen[key] = true
	if ref, ok := f.exports[name]; ok {
		return r.ref(f, ref)
	}
	if g := r.ld.starOwners(f)[name]; g != nil {
		return r.export(g, name)
	}
	return target{}, false
}

// starOwners maps each name f re-exports with "export *" to the module it
// comes from — the first in source order that has it. It is computed once
// per file, so resolving every name of a module with many star re-exports
// stays linear.
func (ld *loader) starOwners(f *file) map[string]*file {
	if f.owners != nil {
		return f.owners
	}
	f.owners = map[string]*file{}
	for _, spec := range f.stars {
		g := ld.dep(f, spec)
		if g == nil {
			continue
		}
		for _, n := range ld.exportNames(g) {
			if _, ok := f.owners[n]; !ok && n != "default" {
				f.owners[n] = g
			}
		}
	}
	return f.owners
}

// ref resolves an export or import reference made in f.
func (r *resolver) ref(f *file, ref exportRef) (target, bool) {
	if ref.spec == "" {
		return r.local(f, ref.name)
	}
	g := r.ld.dep(f, ref.spec)
	if g == nil {
		return target{}, false
	}
	if ref.name == "*" {
		return target{ns: g}, true
	}
	return r.export(g, ref.name)
}

// local resolves a binding of f: a declaration, or an import.
func (r *resolver) local(f *file, name string) (target, bool) {
	if d, ok := f.locals[name]; ok {
		return target{decl: d}, true
	}
	if imp, ok := f.imports[name]; ok {
		return r.ref(f, imp)
	}
	return target{}, false
}

// exportNames lists the public names of f, star re-exports included,
// sorted. A name f exports itself wins over one it re-exports.
func (ld *loader) exportNames(f *file) []string {
	names := map[string]bool{}
	ld.collectNames(f, names, map[string]bool{}, true)
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// collectNames adds the public names of f to names; visited ends cycles.
func (ld *loader) collectNames(f *file, names, visited map[string]bool, top bool) {
	if visited[f.path] {
		return
	}
	visited[f.path] = true
	for n := range f.exports {
		if top || n != "default" {
			names[n] = true
		}
	}
	for _, spec := range f.stars {
		if g := ld.dep(f, spec); g != nil {
			ld.collectNames(g, names, visited, false)
		}
	}
}

// reachable lists the files reachable from f through any relative import
// or export, f included, sorted by path.
func (ld *loader) reachable(f *file) []*file {
	seen := map[string]*file{}
	queue := []*file{f}
	for len(queue) > 0 {
		g := queue[0]
		queue = queue[1:]
		if _, ok := seen[g.path]; ok {
			continue
		}
		seen[g.path] = g
		for _, spec := range g.deps {
			if h := ld.dep(g, spec); h != nil {
				queue = append(queue, h)
			}
		}
	}
	out := make([]*file, 0, len(seen))
	for _, g := range seen {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}
