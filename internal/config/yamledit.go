package config

// Editing a config file in place, as a YAML document rather than by
// re-serialising the parsed struct: comments, key order and formatting survive,
// so an annotated config stays annotated. Two callers need exactly this — the
// MCP designer tools (#1.8.16) and `ssg migrate`, which fills in what the
// source site told us about itself — so the editor lives here, once.

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// SetYAMLKey sets a top-level key, preserving the file's comments, key order
// and formatting by editing the document node in place. A key that is already
// present keeps its comments and gets the new value; a new key is appended.
func SetYAMLKey(src []byte, key string, value interface{}) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	var valueNode yaml.Node
	if err := valueNode.Encode(value); err != nil {
		return nil, err
	}
	root := documentMapping(&doc)
	if root == nil {
		return nil, fmt.Errorf("the config file is not a YAML mapping")
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			// Replace the value, keeping any comment attached to the key.
			valueNode.HeadComment = root.Content[i+1].HeadComment
			valueNode.LineComment = root.Content[i+1].LineComment
			valueNode.FootComment = root.Content[i+1].FootComment
			*root.Content[i+1] = valueNode
			return marshalYAML(&doc)
		}
	}
	keyNode := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	root.Content = append(root.Content, &keyNode, &valueNode)
	return marshalYAML(&doc)
}

// HasYAMLKey reports whether a top-level key is already present, so a caller
// that fills in defaults never overwrites a decision the author has made.
func HasYAMLKey(src []byte, key string) bool {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		// An unparseable file is treated as "already has it": the safe answer
		// is to touch nothing.
		return true
	}
	root := documentMapping(&doc)
	if root == nil {
		return true
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return true
		}
	}
	return false
}

// documentMapping unwraps a document node to its root mapping, or nil when the
// file does not hold one.
func documentMapping(doc *yaml.Node) *yaml.Node {
	n := doc
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil
		}
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	return n
}

// marshalYAML re-encodes the document with the project's usual two-space indent.
func marshalYAML(doc *yaml.Node) ([]byte, error) {
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}
