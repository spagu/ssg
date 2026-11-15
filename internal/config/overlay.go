package config

// The settings overlay (GO-177): `.ssg.designer.json`, beside the config
// file, written whole by designer-ui so `.ssg.yaml` — hand-written, with its
// comments — is never rewritten by a program.
//
// Order of sources, lowest first: built-in defaults, `include:` files, the
// config file, the overlay, command-line flags. `$NAME` references are
// resolved from the environment where each key is used, after all of that.
//
// The overlay is merged into the YAML document before it is decoded, so a
// value in it means exactly what the same value written in .ssg.yaml means:
// mappings merge key by key, any other value — a list included — replaces
// the file's value whole, and null removes the key (the default applies).
// It is validated against the configuration schema first: an unknown key, a
// value of the wrong type, invalid JSON, or a literal in a secret key stops
// the build with the key's path. Nothing is ever silently dropped.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// OverlayFileName is the overlay's name, next to the config file.
const OverlayFileName = ".ssg.designer.json"

// OverlayPath is the overlay file belonging to configPath ("" for "").
func OverlayPath(configPath string) string {
	if configPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(configPath), OverlayFileName)
}

// readOverlay returns the validated overlay of configPath, or nil when there
// is none.
func readOverlay(configPath string) (map[string]any, error) {
	raw, err := os.ReadFile(OverlayPath(configPath)) // #nosec G304 -- the overlay beside the operator's own config
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if dec.More() {
		return nil, errors.New("invalid JSON: more than one value")
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("invalid overlay: the top level must be an object")
	}
	if err := ValidateOverlay(m); err != nil {
		return nil, err
	}
	return normalizeNumbers(m).(map[string]any), nil
}

// applyOverlay merges the overlay beside path into the YAML config data.
// Without an overlay file data is returned unchanged, byte for byte.
func applyOverlay(path string, data []byte, ext string) ([]byte, error) {
	overlay, err := readOverlay(path)
	if err != nil || overlay == nil {
		return data, err
	}
	if ext != ".yaml" && ext != ".yml" {
		return nil, fmt.Errorf("%s works with a YAML config only; %s is %s", OverlayFileName, filepath.Base(path), ext)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 { // an empty file
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: the config is not a mapping", filepath.Base(path))
	}
	mergeIntoNode(root, overlay)
	return yaml.Marshal(&doc)
}

// mergeIntoNode merges an overlay mapping into a YAML mapping node in place:
// mappings recurse, null deletes, anything else replaces. The values come from
// JSON (maps, lists, strings, numbers, booleans), which always encode.
func mergeIntoNode(m *yaml.Node, overlay map[string]any) {
	keys := make([]string, 0, len(overlay))
	for k := range overlay {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := overlay[k]
		i := mappingIndex(m, k)
		if v == nil {
			if i >= 0 {
				m.Content = append(m.Content[:i], m.Content[i+2:]...)
			}
			continue
		}
		if sub, ok := v.(map[string]any); ok && i >= 0 {
			target := m.Content[i+1]
			if target.Kind == yaml.AliasNode { // never edit the anchor others share
				target = copyNode(target.Alias)
				m.Content[i+1] = target
			}
			if target.Kind == yaml.MappingNode {
				mergeIntoNode(target, sub)
				continue
			}
		}
		var n yaml.Node
		_ = n.Encode(v) // JSON-decoded data always encodes
		if i >= 0 {
			m.Content[i+1] = &n
			continue
		}
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, &n)
	}
}

// copyNode deep-copies a node tree.
func copyNode(n *yaml.Node) *yaml.Node {
	c := *n
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, child := range n.Content {
		c.Content[i] = copyNode(child)
	}
	return &c
}

// normalizeNumbers turns json.Number into int64 or float64, so the YAML
// encoder writes a number, not a quoted string.
func normalizeNumbers(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		f, _ := t.Float64() // validated as a number already
		return f
	case map[string]any:
		for k, e := range t {
			t[k] = normalizeNumbers(e)
		}
	case []any:
		for i, e := range t {
			t[i] = normalizeNumbers(e)
		}
	}
	return v
}

// OverlayKeys lists the leaf paths the overlay of configPath sets, sorted —
// what `ssg config view --effective` names as coming from it. No overlay is
// an empty list.
func OverlayKeys(configPath string) ([]string, error) {
	overlay, err := readOverlay(configPath)
	if err != nil {
		return nil, err
	}
	var out []string
	var walk func(prefix string, m map[string]any)
	walk = func(prefix string, m map[string]any) {
		for k, v := range m {
			if sub, ok := v.(map[string]any); ok && len(sub) > 0 {
				walk(joinPath(prefix, k), sub)
				continue
			}
			out = append(out, joinPath(prefix, k))
		}
	}
	walk("", overlay)
	sort.Strings(out)
	return out, nil
}

// hasEnvRef reports whether s holds a $NAME or ${NAME} reference.
func hasEnvRef(s string) bool { return envRefRe.MatchString(s) }

// trimLogin drops the "login:" part of a server_users entry.
func trimLogin(s string) (string, bool) {
	_, ref, ok := strings.Cut(s, ":")
	return ref, ok
}
