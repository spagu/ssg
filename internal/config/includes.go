package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spagu/ssg/internal/fetch"
	"gopkg.in/yaml.v3"
)

// YAML includes (GO-076). A `.ssg.yaml` may split across files and pull them in:
//
//	include:
//	  - shared/base.yaml                    # local path (relative to this file)
//	  - path: workers/comments/config.yaml
//	  - url: https://example.com/base.yaml  # remote
//	    auth: { type: bearer, token: $TOKEN }
//
// Merge order is base-first: includes are merged in listed order, then the
// including file is overlaid on top, so the main file always wins. Maps merge
// recursively; a list of maps that all carry a `name` key merges by name (so
// each worker's own config file can contribute one `workers:` entry without
// clobbering the others); any other list is replaced wholesale.

// maxIncludeDepth bounds nesting so a mistaken chain cannot recurse forever.
const maxIncludeDepth = 20

// resolveIncludes expands an `include:` list in the YAML at path and returns the
// merged YAML. When there is no `include:` key it returns data unchanged, so an
// ordinary config passes through byte-for-byte.
func resolveIncludes(path string, data []byte) ([]byte, error) {
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
	}
	if _, ok := root["include"]; !ok {
		return data, nil // fast path: nothing to do, config untouched
	}
	abs, _ := filepath.Abs(path)
	merged, err := mergeWithIncludes(abs, root, map[string]bool{abs: true}, 0)
	if err != nil {
		return nil, err
	}
	out, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("re-encoding merged config: %w", err)
	}
	return out, nil
}

// mergeWithIncludes resolves the `include:` list of one already-parsed document
// (whose own path is `self`) and returns base-first-merged content: each include
// merged in order, then this document overlaid on top.
func mergeWithIncludes(self string, doc map[string]interface{}, seen map[string]bool, depth int) (map[string]interface{}, error) {
	if depth > maxIncludeDepth {
		return nil, fmt.Errorf("include nesting exceeds %d levels (cycle?)", maxIncludeDepth)
	}
	rawIncludes := doc["include"]
	delete(doc, "include") // never a config key itself

	base := map[string]interface{}{}
	entries, err := parseIncludeEntries(rawIncludes)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		child, key, err := e.load(self)
		if err != nil {
			return nil, err
		}
		if seen[key] {
			return nil, fmt.Errorf("include cycle: %s is included more than once", e.display())
		}
		seen[key] = true
		resolvedChild, err := mergeWithIncludes(key, child, seen, depth+1)
		delete(seen, key) // a diamond include (two files pulling the same base) is fine; only a true cycle errors
		if err != nil {
			return nil, err
		}
		base = deepMerge(base, resolvedChild)
	}
	return deepMerge(base, doc), nil
}

// includeEntry is one item of the `include:` list.
type includeEntry struct {
	path string // local path (mutually exclusive with url)
	url  string
	auth fetch.Auth
}

func (e includeEntry) display() string {
	if e.url != "" {
		return e.url
	}
	return e.path
}

// load reads the entry's content (relative to includer) and returns it parsed,
// plus a canonical key for cycle detection.
func (e includeEntry) load(includer string) (map[string]interface{}, string, error) {
	var raw []byte
	var key string
	if e.url != "" {
		auth, err := fetch.ExpandAuth(e.auth)
		if err != nil {
			return nil, "", fmt.Errorf("include %s: %w", e.url, err)
		}
		if raw, err = fetch.Bytes(e.url, auth, 0); err != nil {
			return nil, "", fmt.Errorf("include: %w", err)
		}
		key = e.url
	} else {
		p := e.path
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(includer), p)
		}
		key, _ = filepath.Abs(p)
		data, err := os.ReadFile(key) // #nosec G304 -- include path from the user's own config
		if err != nil {
			return nil, "", fmt.Errorf("include %s: %w", e.path, err)
		}
		raw = data
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, "", fmt.Errorf("include %s: %w", e.display(), err)
	}
	if doc == nil {
		doc = map[string]interface{}{}
	}
	return doc, key, nil
}

// parseIncludeEntries normalises the `include:` value (a list of bare strings or
// {path|url, auth} maps) into includeEntry values.
func parseIncludeEntries(raw interface{}) ([]includeEntry, error) {
	if raw == nil {
		return nil, nil // a document with no include: — the common case for a base file
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("include: must be a list of paths or URLs")
	}
	var out []includeEntry
	for _, item := range list {
		switch v := item.(type) {
		case string:
			out = append(out, entryFromRef(v))
		case map[string]interface{}:
			e, err := entryFromMap(v)
			if err != nil {
				return nil, err
			}
			out = append(out, e)
		default:
			return nil, fmt.Errorf("include: entry %v is neither a path/URL string nor a map", item)
		}
	}
	return out, nil
}

// entryFromRef classifies a bare string as a URL or a local path.
func entryFromRef(ref string) includeEntry {
	if fetch.IsURL(ref) {
		return includeEntry{url: ref}
	}
	return includeEntry{path: ref}
}

// entryFromMap reads the {path|url, auth} include form.
func entryFromMap(m map[string]interface{}) (includeEntry, error) {
	e := entryFromRef(asString(m["path"]))
	if u := asString(m["url"]); u != "" {
		e = includeEntry{url: u}
	}
	if e.path == "" && e.url == "" {
		return includeEntry{}, fmt.Errorf("include: entry needs a `path` or a `url`")
	}
	if am, ok := m["auth"].(map[string]interface{}); ok {
		e.auth = fetch.Auth{
			Type:     asString(am["type"]),
			Token:    asString(am["token"]),
			Username: asString(am["username"]),
			Password: asString(am["password"]),
			Header:   asString(am["header"]),
			Value:    asString(am["value"]),
		}
	}
	return e, nil
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// deepMerge overlays src onto dst and returns the result. Maps merge
// recursively; lists of named maps merge by name; everything else is replaced.
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	if dst == nil {
		dst = map[string]interface{}{}
	}
	for k, sv := range src {
		dv, present := dst[k]
		if !present {
			dst[k] = sv
			continue
		}
		dst[k] = mergeValue(dv, sv)
	}
	return dst
}

// mergeValue merges one overlay value onto one base value.
func mergeValue(base, over interface{}) interface{} {
	bm, bok := base.(map[string]interface{})
	om, ook := over.(map[string]interface{})
	if bok && ook {
		return deepMerge(bm, om)
	}
	bl, blok := base.([]interface{})
	ol, olok := over.([]interface{})
	if blok && olok && allNamedMaps(bl) && allNamedMaps(ol) {
		return mergeNamedLists(bl, ol)
	}
	return over // scalars, mismatched kinds, and plain lists: overlay wins
}

// allNamedMaps reports whether every element is a map carrying a string `name`.
func allNamedMaps(list []interface{}) bool {
	if len(list) == 0 {
		return false
	}
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			return false
		}
		if _, ok := m["name"].(string); !ok {
			return false
		}
	}
	return true
}

// mergeNamedLists merges two lists of named maps by their `name`: an existing
// name is deep-merged (overlay wins per key), a new name is appended. Order is
// the base list, then any names the overlay introduces, sorted for determinism.
// A name repeated within one list is merged in place (not dropped or emitted
// twice), so malformed duplicate-name input degrades to a merge rather than
// silent corruption.
func mergeNamedLists(base, over []interface{}) []interface{} {
	byName := map[string]map[string]interface{}{}
	var order []string
	ingest := func(list []interface{}) {
		for _, item := range list {
			m := item.(map[string]interface{})
			name := m["name"].(string)
			if existing, ok := byName[name]; ok {
				byName[name] = deepMerge(existing, m)
				continue
			}
			byName[name] = m
			order = append(order, name)
		}
	}
	ingest(base)
	introduced := len(order)
	ingest(over)
	// Only the names the overlay newly introduces are sorted, for determinism;
	// the base order is preserved.
	sort.Strings(order[introduced:])
	out := make([]interface{}, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}
