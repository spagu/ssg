package openapi

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// parser carries the document root and the diagnostics collected so far.
type parser struct {
	root  *yaml.Node
	diags []Diagnostic
}

// diag records one diagnostic.
func (p *parser) diag(path, format string, args ...any) {
	p.diags = append(p.diags, Diagnostic{Path: path, Message: fmt.Sprintf(format, args...)})
}

// refOf returns the $ref string of a mapping, if it has one.
func refOf(n *yaml.Node) (string, bool) {
	ref := get(n, "$ref")
	if ref == nil {
		return "", false
	}
	return str(ref), true
}

// deref follows local $refs until it reaches a concrete node. It returns nil,
// with a diagnostic at path, for external, unresolved or cyclic references.
func (p *parser) deref(n *yaml.Node, path string) *yaml.Node {
	n = alias(n)
	for depth := 0; ; depth++ {
		ref, ok := refOf(n)
		if !ok {
			return n
		}
		if depth >= maxDepth {
			p.diag(path, "$ref chain deeper than %d (cycle?) at %q", maxDepth, ref)
			return nil
		}
		if !strings.HasPrefix(ref, "#") {
			p.diag(path, "external $ref not supported: %q", ref)
			return nil
		}
		target := p.lookup(ref)
		if target == nil {
			p.diag(path, "unresolved $ref %q", ref)
			return nil
		}
		n = target
	}
}

// lookup resolves a local "#/a/b" reference against the document root.
func (p *parser) lookup(ref string) *yaml.Node {
	if ref == "#" {
		return p.root
	}
	rest, ok := strings.CutPrefix(ref, "#/")
	if !ok {
		return nil
	}
	n := p.root
	for _, token := range strings.Split(rest, "/") {
		if decoded, err := url.PathUnescape(token); err == nil {
			token = decoded
		}
		n = child(n, unescape(token))
		if n == nil {
			return nil
		}
	}
	return n
}

// child steps one pointer token into a mapping (by key) or sequence (by index).
func child(n *yaml.Node, token string) *yaml.Node {
	n = alias(n)
	if n != nil && n.Kind == yaml.SequenceNode {
		i, err := strconv.Atoi(token)
		if err != nil || i < 0 || i >= len(n.Content) {
			return nil
		}
		return alias(n.Content[i])
	}
	return get(n, token)
}

// hasComponent reports whether components.<kind>.<name> exists.
func (p *parser) hasComponent(kind, name string) bool {
	return get(get(get(p.root, "components"), kind), name) != nil
}
