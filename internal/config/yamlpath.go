package config

// Reaching into a config file by path (GO-101).
//
// [SetYAMLKey] edits one top-level key, which is all the MCP designer tools and
// `ssg migrate` ever needed. A person editing their own config needs more: the
// interesting settings are nested (`taxonomies.audience.multiple`), some keys
// are not identifiers at all (`headers."/css/*".Cache-Control` is a real line
// in this project's own config), and lists are addressed by position.
//
// The document is parsed to find where a value lives; the change itself is
// spliced into the original text (see yamlsplice.go), so one setting changed
// is one line changed.

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// pathSeg is one step of a path: a mapping key, or a position in a sequence.
type pathSeg struct {
	key     string
	index   int
	isIndex bool
}

// String renders a segment the way it was written, for error messages.
func (s pathSeg) String() string {
	if s.isIndex {
		return "[" + strconv.Itoa(s.index) + "]"
	}
	if strings.ContainsAny(s.key, `.[]"`) {
		return `"` + s.key + `"`
	}
	return s.key
}

// ParseYAMLPath splits a dotted path into its segments.
//
//	highlight_style
//	taxonomies.audience.multiple
//	headers."/css/*".Cache-Control
//	robots_rules[1].allow
//
// A quoted segment carries anything but a quote, which is how a key containing
// dots or slashes is addressed at all.
func ParseYAMLPath(path string) ([]pathSeg, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("empty path")
	}
	var segs []pathSeg
	var cur strings.Builder
	started := false // a segment has begun, even if its text is empty ("")
	flush := func() error {
		if !started {
			return fmt.Errorf("%q: empty path segment", path)
		}
		segs = append(segs, pathSeg{key: cur.String()})
		cur.Reset()
		started = false
		return nil
	}
	for i := 0; i < len(path); i++ {
		switch c := path[i]; c {
		case '"':
			end := strings.IndexByte(path[i+1:], '"')
			if end < 0 {
				return nil, fmt.Errorf("%q: unclosed quote", path)
			}
			cur.WriteString(path[i+1 : i+1+end])
			started = true
			i += end + 1
		case '.':
			if err := flush(); err != nil {
				return nil, err
			}
		case '[':
			end := strings.IndexByte(path[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("%q: unclosed [", path)
			}
			if started {
				if err := flush(); err != nil {
					return nil, err
				}
			}
			n, err := strconv.Atoi(path[i+1 : i+end])
			if err != nil || n < 0 {
				return nil, fmt.Errorf("%q: %q is not a list index", path, path[i:i+end+1])
			}
			segs = append(segs, pathSeg{index: n, isIndex: true})
			i += end
			// A dot after an index is separator noise: a[0].b and a[0]b would
			// otherwise disagree about where the next segment starts.
			if i+1 < len(path) && path[i+1] == '.' {
				i++
			}
		case ']':
			return nil, fmt.Errorf("%q: unmatched ]", path)
		default:
			cur.WriteByte(c)
			started = true
		}
	}
	if started {
		if err := flush(); err != nil {
			return nil, err
		}
	} else if strings.HasSuffix(path, ".") {
		return nil, fmt.Errorf("%q: empty path segment", path)
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("%q: no path segments", path)
	}
	return segs, nil
}

// GetYAMLPath returns the value at path, encoded as YAML: a scalar as its own
// text, a mapping or sequence as a YAML fragment.
func GetYAMLPath(src []byte, path string) ([]byte, error) {
	segs, err := ParseYAMLPath(path)
	if err != nil {
		return nil, err
	}
	root, _, err := parseRoot(src)
	if err != nil {
		return nil, err
	}
	node, err := descend(root, segs, path)
	if err != nil {
		return nil, err
	}
	if node.Kind == yaml.ScalarNode {
		return []byte(node.Value + "\n"), nil
	}
	return marshalYAML(node)
}

// SetYAMLPath sets the value at path, creating the mappings on the way to it.
// A list position must already exist: inventing list entries is guesswork, and
// a config file is not the place for it.
func SetYAMLPath(src []byte, path string, value interface{}) ([]byte, error) {
	segs, err := ParseYAMLPath(path)
	if err != nil {
		return nil, err
	}
	return setSegments(src, segs, value)
}

// setSegments is SetYAMLPath with the path already parsed, so a caller holding
// a literal key never has it re-parsed as a path.
func setSegments(src []byte, segs []pathSeg, value interface{}) ([]byte, error) {
	root, _, err := parseRoot(src)
	if err != nil {
		// A file with nothing in it — freshly created, or only comments — is a
		// config whose first key has yet to be written, not a malformed one.
		// `ssg config set` on one has to work, or the command is useless
		// exactly when someone is starting out. A file holding a list or a
		// scalar stays refused: that is a different document, and writing a key
		// into it would lose whatever it held.
		if !emptyDocument(src) {
			return nil, err
		}
		root = &yaml.Node{Kind: yaml.MappingNode}
	}
	full := pathString(segs)
	node, parentKey := root, (*yaml.Node)(nil)
	for n, seg := range segs[:len(segs)-1] {
		if seg.isIndex {
			if node.Kind != yaml.SequenceNode {
				return nil, fmt.Errorf("%s: %s is not a list", full, pathString(segs[:n]))
			}
			if seg.index >= len(node.Content) {
				return nil, fmt.Errorf("%s: the list at %s has %d entries", full, pathString(segs[:n]), len(node.Content))
			}
			node, parentKey = node.Content[seg.index], nil
			continue
		}
		if node.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s: %s is not a mapping", full, pathString(segs[:n]))
		}
		i := mappingIndex(node, seg.key)
		if i < 0 {
			// The rest of the path does not exist yet: write it as one nested
			// block rather than refusing an edit the caller clearly meant.
			return insertNested(src, node, parentKey, segs[n:], value)
		}
		parentKey, node = node.Content[i], node.Content[i+1]
	}

	last := segs[len(segs)-1]
	if last.isIndex {
		if node.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("%s: %s is not a list", full, pathString(segs[:len(segs)-1]))
		}
		if last.index >= len(node.Content) {
			return nil, fmt.Errorf("%s: the list has %d entries", full, len(node.Content))
		}
		return spliceSetSeqEntry(src, node.Content[last.index], value)
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: %s is not a mapping", full, pathString(segs[:len(segs)-1]))
	}
	if i := mappingIndex(node, last.key); i >= 0 {
		out, err := spliceSet(src, node.Content[i], node.Content[i+1], value)
		if err != nil {
			return nil, err
		}
		return out, nil
	}
	// A brand-new key in a mapping that exists.
	return insertNested(src, node, parentKey, segs[len(segs)-1:], value)
}

// UnsetYAMLPath removes the value at path. A path that is not set is an error
// rather than a silent success: "removed it" and "it was never there" are
// different answers, and only one of them means the caller was right.
func UnsetYAMLPath(src []byte, path string) ([]byte, error) {
	segs, err := ParseYAMLPath(path)
	if err != nil {
		return nil, err
	}
	root, _, err := parseRoot(src)
	if err != nil {
		return nil, err
	}
	parent, err := descend(root, segs[:len(segs)-1], path)
	if err != nil {
		return nil, err
	}
	last := segs[len(segs)-1]
	if last.isIndex {
		if parent.Kind != yaml.SequenceNode || last.index >= len(parent.Content) {
			return nil, fmt.Errorf("%s is not set", path)
		}
		return spliceRemove(src, parent.Content[last.index], false)
	}
	i := -1
	if parent.Kind == yaml.MappingNode {
		i = mappingIndex(parent, last.key)
	}
	if i < 0 {
		return nil, fmt.Errorf("%s is not set", path)
	}
	return spliceRemove(src, parent.Content[i], true)
}

// parseRoot returns the document's root mapping and the document node.
func parseRoot(src []byte) (*yaml.Node, *yaml.Node, error) {
	doc := &yaml.Node{}
	if err := yaml.Unmarshal(src, doc); err != nil {
		return nil, nil, err
	}
	root := documentMapping(doc)
	if root == nil {
		return nil, nil, fmt.Errorf("the config file is not a YAML mapping")
	}
	return root, doc, nil
}

// emptyDocument reports whether a file holds no YAML value at all, which is
// what an empty file and a file of only comments both produce.
//
// It is deliberately not folded into parseRoot: SetYAMLKey shares that and is a
// filler that must never invent a document, while `ssg config set` is a person
// asking for a key to be written.
func emptyDocument(src []byte) bool {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return false
	}
	return doc.Kind == 0 || (doc.Kind == yaml.DocumentNode && len(doc.Content) == 0)
}

// descend walks the segments from a mapping node, creating nothing.
func descend(node *yaml.Node, segs []pathSeg, path string) (*yaml.Node, error) {
	for n, seg := range segs {
		if seg.isIndex {
			if node.Kind != yaml.SequenceNode {
				return nil, fmt.Errorf("%s: %s is not a list", path, pathString(segs[:n]))
			}
			if seg.index >= len(node.Content) {
				return nil, fmt.Errorf("%s: the list at %s has %d entries", path, pathString(segs[:n]), len(node.Content))
			}
			node = node.Content[seg.index]
			continue
		}
		if node.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s: %s is not a mapping", path, pathString(segs[:n]))
		}
		i := mappingIndex(node, seg.key)
		if i < 0 {
			return nil, fmt.Errorf("%s is not set", path)
		}
		node = node.Content[i+1]
	}
	return node, nil
}

// mappingIndex returns the position of a key in a mapping's content, or -1.
func mappingIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// pathString renders segments back to the path syntax, for error messages.
func pathString(segs []pathSeg) string {
	if len(segs) == 0 {
		return "the document root"
	}
	var b strings.Builder
	for i, s := range segs {
		if i > 0 && !s.isIndex {
			b.WriteByte('.')
		}
		b.WriteString(s.String())
	}
	return b.String()
}

// ParseYAMLValue turns a command-line string into the value to write: a list in
// [a,b,c] form, a bool, a number, or a string. forceString keeps "true" and
// "8080" as text, which a setting whose value happens to look like a number
// needs.
func ParseYAMLValue(raw string, forceString bool) interface{} {
	if forceString {
		return raw
	}
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		if inner == "" {
			return []interface{}{}
		}
		parts := strings.Split(inner, ",")
		out := make([]interface{}, 0, len(parts))
		for _, p := range parts {
			out = append(out, ParseYAMLValue(strings.TrimSpace(p), false))
		}
		return out
	}
	switch strings.ToLower(trimmed) {
	case "true":
		return true
	case "false":
		return false
	case "null", "~":
		return nil
	}
	if n, err := strconv.Atoi(trimmed); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return f
	}
	return raw
}

// ParseYAMLMap decodes a YAML mapping into a plain map, for a caller that
// needs the values rather than the document: a form filling its fields in
// (GO-102) reads here, and writes back through SetYAMLPath so the file keeps
// its shape.
func ParseYAMLMap(src []byte) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	if err := yaml.Unmarshal(src, &out); err != nil {
		return nil, err
	}
	return out, nil
}
