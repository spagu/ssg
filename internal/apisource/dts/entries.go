package dts

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

// packageJSON is the part of package.json the extractor reads.
type packageJSON struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Main    string          `json:"main"`
	Types   string          `json:"types"`
	Typings string          `json:"typings"`
	Exports json.RawMessage `json:"exports"`
}

// readPackageJSON reads package.json at the root; a missing file is an
// empty manifest, a malformed one an error.
func readPackageJSON(root *os.Root) (packageJSON, error) {
	var pj packageJSON
	data, err := root.ReadFile("package.json")
	if errors.Is(err, fs.ErrNotExist) {
		return pj, nil
	}
	if err != nil {
		return pj, err
	}
	if err := json.Unmarshal(data, &pj); err != nil {
		return pj, err
	}
	return pj, nil
}

// manifestEntries lists the declaration entries package.json names: the
// "types" (or "typings") field, then each "exports" subpath with a "types"
// condition, the main subpath first. Paths are cleaned and relative to the
// root; duplicates are dropped.
func manifestEntries(pj packageJSON) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p = cleanRel(p); p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	add(firstNonEmpty(pj.Types, pj.Typings))
	for _, p := range exportTypes(pj.Exports) {
		add(p)
	}
	return out
}

// exportTypes returns the "types" condition of each subpath of an
// "exports" field, "." first and then by subpath; patterns ("./*") are
// skipped, having no single file.
func exportTypes(raw json.RawMessage) []string {
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok || !hasSubpaths(m) {
		if t := findTypes(v); t != "" {
			return []string{t}
		}
		return nil
	}
	keys := sortedKeys(m)
	sort.SliceStable(keys, func(i, j int) bool { return keys[i] == "." && keys[j] != "." })
	var out []string
	for _, k := range keys {
		if strings.HasPrefix(k, ".") && !strings.Contains(k, "*") {
			if t := findTypes(m[k]); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

// hasSubpaths reports whether an exports object is keyed by subpaths.
func hasSubpaths(m map[string]any) bool {
	for k := range m {
		if strings.HasPrefix(k, ".") {
			return true
		}
	}
	return false
}

// conditionOrder is the order nested conditions are searched for "types".
var conditionOrder = []string{"import", "require", "default", "node"}

// findTypes finds the "types" condition in a conditional export, searching
// nested conditions in a fixed order so the answer is stable.
func findTypes(v any) string {
	switch c := v.(type) {
	case map[string]any:
		if s, ok := c["types"].(string); ok {
			return s
		}
		keys := append([]string{}, conditionOrder...)
		keys = append(keys, sortedKeys(c)...)
		for _, k := range keys {
			if t := findTypes(c[k]); t != "" {
				return t
			}
		}
	case []any:
		for _, item := range c {
			if t := findTypes(item); t != "" {
				return t
			}
		}
	}
	return ""
}

// sortedKeys returns a map's keys in order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// cleanRel turns "./dist/index.d.ts" or "dist\\index.d.ts" into
// "dist/index.d.ts"; "" for a path that leaves the root.
func cleanRel(p string) string {
	if p == "" {
		return ""
	}
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
	if p == "." || p == ".." || strings.HasPrefix(p, "../") || path.IsAbs(p) {
		return ""
	}
	return p
}

// firstNonEmpty returns the first argument that is not "".
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
