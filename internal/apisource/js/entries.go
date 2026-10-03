package js

import (
	"encoding/json"
	"sort"
	"strings"
)

// packageJSON is the part of package.json the extractor reads.
type packageJSON struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Module  string          `json:"module"`
	Main    string          `json:"main"`
	Exports json.RawMessage `json:"exports"`
}

// conditionOrder is the order export conditions are tried in.
var conditionOrder = []string{"import", "module", "default"}

// entryCandidates lists the entry files the package declares, in order:
// the configured entries; else the "." export then the other subpath
// exports; else "module", then "main"; else index.js. Paths are relative to
// the root and may lack an extension.
func entryCandidates(configured []string, pj *packageJSON) []string {
	if len(configured) > 0 {
		return configured
	}
	if entries := exportEntries(pj.Exports); len(entries) > 0 {
		return entries
	}
	for _, p := range []string{pj.Module, pj.Main} {
		if p != "" {
			return []string{p}
		}
	}
	return []string{"index.js"}
}

// exportEntries reads the "exports" field: a string, a conditions object,
// or a map of subpaths ("." first, then the rest sorted). Subpath patterns
// ("./*") are skipped.
func exportEntries(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var subpaths map[string]json.RawMessage
	if json.Unmarshal(raw, &subpaths) != nil || !hasSubpaths(subpaths) {
		return compact([]string{exportTarget(raw)})
	}
	keys := make([]string, 0, len(subpaths))
	for k := range subpaths {
		if k != "." && !strings.Contains(k, "*") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := []string{exportTarget(subpaths["."])}
	for _, k := range keys {
		out = append(out, exportTarget(subpaths[k]))
	}
	return compact(out)
}

// hasSubpaths reports whether an exports object is keyed by subpaths
// rather than conditions.
func hasSubpaths(m map[string]json.RawMessage) bool {
	for k := range m {
		if strings.HasPrefix(k, ".") {
			return true
		}
	}
	return false
}

// exportTarget resolves one export value to a JavaScript file: a string, an
// array (first usable item) or a conditions object (import, module,
// default, nested). It returns "" when nothing fits.
func exportTarget(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if isScript(s) || !strings.Contains(path0(s), ".") {
			return s
		}
		return ""
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		for _, item := range list {
			if t := exportTarget(item); t != "" {
				return t
			}
		}
		return ""
	}
	var conds map[string]json.RawMessage
	if json.Unmarshal(raw, &conds) != nil {
		return ""
	}
	for _, c := range conditionOrder {
		if v, ok := conds[c]; ok {
			if t := exportTarget(v); t != "" {
				return t
			}
		}
	}
	return ""
}

// path0 returns the last segment of a slash path.
func path0(p string) string {
	return p[strings.LastIndex(p, "/")+1:]
}

// compact drops empty and repeated entries, keeping the order.
func compact(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range list {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// scriptExts are the extensions of the files the extractor reads.
var scriptExts = []string{".js", ".mjs", ".cjs", ".jsx"}

// isScript reports whether a path names a JavaScript file.
func isScript(p string) bool {
	for _, ext := range scriptExts {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}
