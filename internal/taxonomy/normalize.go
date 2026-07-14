package taxonomy

import (
	"fmt"
)

// PageSources carries the raw taxonomy inputs of one page, in priority order
// (spec: taxonomies map > configured direct field > legacy fields).
type PageSources struct {
	TaxonomiesFM  map[string]interface{} // frontmatter `taxonomies:` map
	Extra         map[string]interface{} // unknown frontmatter fields (direct custom fields)
	CategoryNames []string               // legacy: resolved category display names
	Tags          []string               // legacy: tags
	Series        string                 // legacy: series
}

// ExtractAssignments normalizes every configured taxonomy for one page into
// map[name][]string, merging+deduplicating multi-value taxonomies and failing
// on single-value cardinality violations. sourceFile appears in errors.
func ExtractAssignments(defs map[string]Definition, names []string, src PageSources, sourceFile string) (map[string][]string, error) {
	out := make(map[string][]string, len(names))
	for _, name := range names {
		def := defs[name]
		var merged []string

		// Priority 1: the generic taxonomies map.
		if raw, ok := src.TaxonomiesFM[name]; ok {
			vals, err := coerceValues(raw)
			if err != nil {
				return nil, fmt.Errorf("%s: taxonomies.%s: %w", sourceFile, name, err)
			}
			merged = append(merged, vals...)
		}
		// Priority 2: the configured direct field (custom taxonomies live in Extra).
		if raw, ok := src.Extra[def.Field]; ok {
			vals, err := coerceValues(raw)
			if err != nil {
				return nil, fmt.Errorf("%s: field %q: %w", sourceFile, def.Field, err)
			}
			merged = append(merged, vals...)
		}
		// Priority 3: legacy model fields.
		merged = append(merged, legacyValues(name, src)...)

		merged = dedupeNormalized(merged, def.CaseSensitive)
		if !def.Multiple && len(merged) > 1 {
			return nil, fmt.Errorf("%s: taxonomy %q is single-value but has %d values %v",
				sourceFile, name, len(merged), merged)
		}
		if len(merged) > 0 {
			out[name] = merged
		}
	}
	return out, nil
}

// legacyValues maps the historical model fields onto the auto-registered
// taxonomies.
func legacyValues(name string, src PageSources) []string {
	switch name {
	case "category":
		return src.CategoryNames
	case "tag":
		return src.Tags
	case "series":
		if src.Series != "" {
			return []string{src.Series}
		}
	}
	return nil
}

// coerceValues accepts a scalar or a list of scalars from YAML frontmatter.
func coerceValues(raw interface{}) ([]string, error) {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil, nil
		}
		return []string{v}, nil
	case []interface{}:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("value %d must be a string, got %T", i, item)
			}
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	case []string:
		return v, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("must be a string or list of strings, got %T", raw)
	}
}

// dedupeNormalized removes duplicates by normalized identity while keeping the
// first-seen display value and order.
func dedupeNormalized(values []string, caseSensitive bool) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		key, display := NormalizeKey(v, caseSensitive)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, display)
	}
	return out
}
