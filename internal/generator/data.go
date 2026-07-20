package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// loadData loads every *.yaml|*.yml|*.json under DataDir into the .Data.* template
// namespace (PLAT-002). Nested subdirectories become nested maps
// (data/authors/bio.yaml → .Data.authors.bio). A missing directory is a no-op.
func (g *Generator) loadData() error {
	dir := g.config.DataDir
	if dir == "" {
		dir = "data"
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil // no data directory → no .Data (not an error)
	}

	data := make(map[string]interface{})
	walkErr := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return nil
		}
		raw, rerr := os.ReadFile(path) // #nosec G304,G122 -- CLI reads its own data dir; path from local Walk, not attacker-controlled
		if rerr != nil {
			return rerr
		}
		var parsed interface{}
		if ext == ".json" {
			if e := json.Unmarshal(raw, &parsed); e != nil {
				return fmt.Errorf("parsing data file %s: %w", path, e)
			}
		} else {
			if e := yaml.Unmarshal(raw, &parsed); e != nil {
				// Enrich error with hints for common YAML pitfalls like space+#
				errWrapped := fmt.Errorf("parsing data file %s: %w", path, e)
				if hint := suggestYAMLHint(raw); hint != "" {
					errWrapped = fmt.Errorf("%w\n%s", errWrapped, hint)
				}
				return errWrapped
			}
		}
		rel, _ := filepath.Rel(dir, path)
		rel = strings.TrimSuffix(rel, filepath.Ext(rel))
		keys := strings.Split(filepath.ToSlash(rel), "/")
		setNestedData(data, keys, normalizeYAMLValue(parsed))
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	g.data = data
	return nil
}

// setNestedData inserts value into m following the key path, creating intermediate
// maps as needed (used to mirror the data/ directory tree under .Data.*).
func setNestedData(m map[string]interface{}, keys []string, value interface{}) {
	for i := 0; i < len(keys)-1; i++ {
		next, ok := m[keys[i]].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			m[keys[i]] = next
		}
		m = next
	}
	m[keys[len(keys)-1]] = value
}

// normalizeYAMLValue converts map[interface{}]interface{} (produced by some YAML
// shapes) into map[string]interface{} recursively so html/template can index it.
func normalizeYAMLValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, vv := range val {
			out[fmt.Sprintf("%v", k)] = normalizeYAMLValue(vv)
		}
		return out
	case map[string]interface{}:
		for k, vv := range val {
			val[k] = normalizeYAMLValue(vv)
		}
		return val
	case []interface{}:
		for i, vv := range val {
			val[i] = normalizeYAMLValue(vv)
		}
		return val
	default:
		return v
	}
}

// suggestYAMLHint returns a hint string if it finds likely causes of parsing failure
// in the raw YAML content, such as space followed by hash outside quotes.
func suggestYAMLHint(raw []byte) string {
	lines := strings.Split(string(raw), "\n")
	var hints []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if idx := strings.Index(line, " #"); idx != -1 {
			before := line[:idx]
			sqCount := strings.Count(before, "'")
			dqCount := strings.Count(before, "\"")
			if sqCount%2 == 0 && dqCount%2 == 0 {
				hints = append(hints, fmt.Sprintf("  - Line %d: %q contains ' #' (space followed by hash). This starts a comment in YAML unless the value is quoted.", i+1, trimmed))
			}
		}
	}
	if len(hints) > 0 {
		return "Potential YAML issues found:\n" + strings.Join(hints, "\n") + "\nTo fix, wrap the string value in double quotes, e.g. \"issue #123\"."
	}
	return ""
}
