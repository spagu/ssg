package dts

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// exportsOf returns the public names of a scope and the declarations they
// denote, following re-exports into other files. A scope with no export
// statement exports every declaration. Results are cached; a cycle of star
// exports ends with what is known so far.
func (b *builder) exportsOf(e *env) map[string]*node {
	if out, ok := b.exports[e.scope]; ok {
		return out
	}
	out := map[string]*node{}
	b.exports[e.scope] = out
	sc := e.scope
	if !sc.explicit {
		for name, n := range sc.decls {
			out[name] = n
		}
		return out
	}
	for _, name := range sortedKeys(sc.exports) {
		if n := b.resolveExport(e, sc.exports[name]); n != nil {
			out[name] = n
		}
	}
	for _, spec := range sc.stars {
		target := b.loader.resolve(e.file, spec)
		if target == nil {
			continue
		}
		for name, n := range b.exportsOf(fileEnv(target)) {
			if _, taken := out[name]; !taken && name != "default" {
				out[name] = n
			}
		}
	}
	return out
}

// resolveExport returns the declaration an export entry denotes.
func (b *builder) resolveExport(e *env, ref exportRef) *node {
	if ref.from == "" {
		return b.resolveName(e, ref.local)
	}
	return b.importTarget(e.file, importRef{from: ref.from, name: ref.name})
}

// importTarget returns the declaration an import binding denotes: an
// export of the target file, or the whole file as a namespace for "*".
func (b *builder) importTarget(f *file, ref importRef) *node {
	target := b.loader.resolve(f, ref.from)
	if target == nil {
		return nil
	}
	if ref.name == "*" {
		return b.fileNamespace(target)
	}
	return b.exportsOf(fileEnv(target))[ref.name]
}

// fileNamespace returns the namespace node standing for a whole file.
func (b *builder) fileNamespace(f *file) *node {
	if n, ok := b.fileNS[f]; ok {
		return n
	}
	n := &node{kind: apimodel.KindNamespace, name: f.path, line: 1, fileNS: f, env: fileEnv(f), doc: f.doc}
	b.fileNS[f] = n
	return n
}

// resolveName resolves a possibly dotted name ("ns.Token") seen in e to a
// declaration or member; nil when it is not declared in the package.
func (b *builder) resolveName(e *env, name string) *node {
	parts := strings.Split(name, ".")
	n := b.lookup(e, parts[0])
	for _, part := range parts[1:] {
		if n == nil {
			return nil
		}
		n = b.child(n, part)
	}
	return n
}

// lookup finds a bare name: in the enclosing scopes, innermost first, then
// among the file's imports.
func (b *builder) lookup(e *env, name string) *node {
	for s := e; s != nil; s = s.parent {
		if n, ok := s.scope.decls[name]; ok {
			return n
		}
	}
	if ref, ok := e.file.imports[name]; ok {
		return b.importTarget(e.file, ref)
	}
	return nil
}

// child returns the member or exported namespace content of n named name.
func (b *builder) child(n *node, name string) *node {
	if n.fileNS != nil {
		return b.exportsOf(fileEnv(n.fileNS))[name]
	}
	if n.body != nil {
		if c := b.exportsOf(bodyEnv(n))[name]; c != nil {
			return c
		}
	}
	for _, m := range n.members {
		if m.name == name {
			return m
		}
	}
	return nil
}

// fileEnv is the top-level environment of a file.
func fileEnv(f *file) *env { return &env{scope: f.top, file: f} }

// bodyEnv is the environment inside a namespace declaration.
func bodyEnv(n *node) *env { return &env{scope: n.body, file: n.env.file, parent: n.env} }
