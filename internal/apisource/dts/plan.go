package dts

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// builder turns parsed files into the model: it decides what each module
// lists, gives every listed declaration its ID, then builds the symbols
// with type references resolved to those IDs.
type builder struct {
	pkg      string
	loader   *loader
	exports  map[*scope]map[string]*node
	fileNS   map[*file]*node
	ids      map[*node]string
	comments map[*node]*commentInfo
	building map[*node]bool
	diags    []apisource.Diagnostic
}

// newBuilder returns a builder for package pkg over the loader's files.
func newBuilder(pkg string, l *loader) *builder {
	return &builder{
		pkg: pkg, loader: l,
		exports: map[*scope]map[string]*node{}, fileNS: map[*file]*node{},
		ids: map[*node]string{}, comments: map[*node]*commentInfo{}, building: map[*node]bool{},
	}
}

// modulePlan is one module to write: its ID, path, overview comment and
// the declarations it lists.
type modulePlan struct {
	id, path string
	doc      string
	listings []listing
}

// listing is one public name of a module and the declaration it denotes.
type listing struct {
	name      string
	id        string
	n         *node
	isDefault bool
}

// plan lists the modules: one per entry file, then one per
// `declare module "name"` block of the files read. Blocks naming the same
// module join it; a name listed twice keeps its first declaration. A block
// that declares nothing (`declare module "x";`) adds no module.
func (b *builder) plan(entries []string) []*modulePlan {
	var plans []*modulePlan
	byID := map[string]*modulePlan{}
	add := func(id, doc string, e *env, ambient bool) {
		listed := b.listings(e, id)
		mp, ok := byID[id]
		if !ok && ambient && len(listed) == 0 {
			return
		}
		if !ok {
			mp = &modulePlan{id: id, path: strings.TrimPrefix(id, b.pkg+"/"), doc: doc}
			byID[id] = mp
			plans = append(plans, mp)
		}
		mp.listings = mergeListings(mp.listings, listed)
	}
	for _, entry := range entries {
		if f := b.loader.files[entry]; f != nil {
			add(apimodel.ModuleID(b.pkg, entry), f.doc, fileEnv(f), false)
		}
	}
	for _, p := range sortedKeys(b.loader.files) {
		f := b.loader.files[p]
		if f == nil {
			continue
		}
		for _, name := range sortedKeys(f.ambients) {
			add(b.ambientID(name, entries), "", &env{scope: f.ambients[name], file: f, parent: fileEnv(f)}, true)
		}
	}
	return plans
}

// ambientID is the module ID of `declare module "name"`: the package's own
// name means its main entry, "pkg/sub" the module "sub".
func (b *builder) ambientID(name string, entries []string) string {
	if name == b.pkg && len(entries) > 0 {
		return apimodel.ModuleID(b.pkg, entries[0])
	}
	return apimodel.ModuleID(b.pkg, strings.TrimPrefix(name, b.pkg+"/"))
}

// mergeListings appends the listings whose IDs are new.
func mergeListings(old, more []listing) []listing {
	seen := map[string]bool{}
	for _, l := range old {
		seen[l.id] = true
	}
	for _, l := range more {
		if !seen[l.id] {
			seen[l.id] = true
			old = append(old, l)
		}
	}
	return old
}

// listings returns what a module lists, by public name. A default export
// is listed under the declaration's own name when no other export uses that
// name, else as "default" — the same IDs the JavaScript extractor gives.
func (b *builder) listings(e *env, moduleID string) []listing {
	pubs := b.exportsOf(e)
	var out []listing
	for _, public := range sortedKeys(pubs) {
		n := pubs[public]
		if b.hidden(n) {
			continue
		}
		name := public
		if public == "default" {
			if own := defaultName(n); pubs[own] == nil {
				name = own
			}
		}
		out = append(out, listing{name: name, id: apimodel.SymbolID(moduleID, name), n: n, isDefault: public == "default"})
	}
	return out
}

// defaultName is a default export's own name: the declaration's, or
// "default" for an anonymous one or a whole module.
func defaultName(n *node) string {
	if n.fileNS != nil || n.name == "" {
		return "default"
	}
	return n.name
}

// child is one member of a symbol by the name it is listed under.
type child struct {
	name string
	n    *node
}

// children lists the members of a declaration: its own members, then the
// public contents of a namespace merged with it, without hidden ones. A
// name is listed once.
func (b *builder) children(n *node) []child {
	var out []child
	seen := map[string]bool{}
	add := func(name string, c *node) {
		if !seen[name] && !b.hidden(c) {
			seen[name] = true
			out = append(out, child{name: name, n: c})
		}
	}
	for _, m := range n.members {
		add(m.name, m)
	}
	var inner map[string]*node
	switch {
	case n.fileNS != nil:
		inner = b.exportsOf(fileEnv(n.fileNS))
	case n.body != nil:
		inner = b.exportsOf(bodyEnv(n))
	}
	for _, name := range sortedKeys(inner) {
		listed := name
		if own := defaultName(inner[name]); name == "default" && inner[own] == nil {
			listed = own
		}
		add(listed, inner[name])
	}
	return out
}

// assign gives a declaration and its members their IDs; the first listing
// of a declaration is the one type references link to.
func (b *builder) assign(n *node, id string) {
	if _, done := b.ids[n]; done {
		return
	}
	b.ids[n] = id
	for _, c := range b.children(n) {
		b.assign(c.n, apimodel.MemberID(id, c.name))
	}
}
