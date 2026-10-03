package openapi

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxDepth bounds every recursive walk ($ref chains, schema nesting, example
// values) so that cyclic documents or YAML alias loops cannot hang the reader.
const maxDepth = 32

// pair is one key/value entry of a YAML mapping, in document order.
type pair struct {
	key   string
	value *yaml.Node
}

// alias follows a YAML alias to its anchored node so callers never see one.
func alias(n *yaml.Node) *yaml.Node {
	if n != nil && n.Kind == yaml.AliasNode {
		return n.Alias
	}
	return n
}

// get returns the value of key in mapping m, or nil.
func get(m *yaml.Node, key string) *yaml.Node {
	for _, p := range pairs(m) {
		if p.key == key {
			return p.value
		}
	}
	return nil
}

// pairs lists the entries of mapping m in document order; nil for non-mappings.
func pairs(m *yaml.Node) []pair {
	m = alias(m)
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	out := make([]pair, 0, len(m.Content)/2)
	for i := 0; i+1 < len(m.Content); i += 2 {
		out = append(out, pair{key: m.Content[i].Value, value: alias(m.Content[i+1])})
	}
	return out
}

// items lists the elements of sequence s; nil for non-sequences.
func items(s *yaml.Node) []*yaml.Node {
	s = alias(s)
	if s == nil || s.Kind != yaml.SequenceNode {
		return nil
	}
	out := make([]*yaml.Node, 0, len(s.Content))
	for _, n := range s.Content {
		out = append(out, alias(n))
	}
	return out
}

// str returns a scalar's text, or "" for anything else.
func str(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// flag reports whether n is the scalar true.
func flag(n *yaml.Node) bool { return str(n) == "true" }

// strs returns the scalar elements of a sequence.
func strs(n *yaml.Node) []string {
	var out []string
	for _, item := range items(n) {
		out = append(out, str(item))
	}
	return out
}

// number parses a numeric scalar; nil when absent or not a number.
func number(n *yaml.Node) *float64 {
	f, err := strconv.ParseFloat(str(n), 64)
	if err != nil {
		return nil
	}
	return &f
}

// integer parses an integer scalar; nil when absent or not an integer.
func integer(n *yaml.Node) *int {
	i, err := strconv.Atoi(str(n))
	if err != nil {
		return nil
	}
	return &i
}

// toAny converts a node into a JSON-friendly value: mappings become
// map[string]any, sequences []any, scalars their natural Go type.
func toAny(n *yaml.Node) any { return toAnyAt(n, 0) }

// toAnyAt is toAny with a depth guard against alias cycles.
func toAnyAt(n *yaml.Node, depth int) any {
	n = alias(n)
	if n == nil || depth > maxDepth {
		return nil
	}
	switch n.Kind {
	case yaml.MappingNode:
		out := make(map[string]any, len(n.Content)/2)
		for _, p := range pairs(n) {
			out[p.key] = toAnyAt(p.value, depth+1)
		}
		return out
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, item := range n.Content {
			out = append(out, toAnyAt(item, depth+1))
		}
		return out
	}
	if n.ShortTag() == "!!timestamp" {
		return n.Value // keep dates as written, not time.Time
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return n.Value
	}
	return v
}

// pointer appends RFC 6901-escaped tokens to a JSON pointer.
func pointer(base string, tokens ...string) string {
	var b strings.Builder
	b.WriteString(base)
	for _, t := range tokens {
		b.WriteByte('/')
		b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(t))
	}
	return b.String()
}

// unescape reverses RFC 6901 escaping of one pointer token.
func unescape(token string) string {
	return strings.NewReplacer("~1", "/", "~0", "~").Replace(token)
}

// isExtension reports whether a map key is a specification extension (x-...).
func isExtension(key string) bool { return strings.HasPrefix(key, "x-") }
