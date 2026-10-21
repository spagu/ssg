package externalsource

import "math"

// A record's values as a page and its templates get them: the whole record
// in Extra, whole numbers for mapping templates, copies of shared values.

// recordExtra hands the whole record to the page's template as .Page.Extra,
// the way custom frontmatter reaches it: the mapping picks a title and a body,
// and the fields it did not pick (a version, a list of dependencies) are still
// what the page is about. A copy, so the data namespace stays untouched.
func recordExtra(rec map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(rec))
	for k, v := range rec {
		out[k] = v
	}
	return out
}

// templateValue is a record as a mapping template sees it: a whole number
// JSON decoded as float64 becomes an int64, so "{{.ts}}" prints 1761638001
// and not 1.761638001e+09, which is no date and no slug (#340). Only the
// template's copy changes; the record, and .ExternalData, keep their types.
func templateValue(v interface{}) interface{} {
	return mapLeaves(v, func(leaf interface{}) interface{} {
		if f, ok := leaf.(float64); ok && f == math.Trunc(f) && math.Abs(f) < 1<<53 {
			return int64(f)
		}
		return leaf
	})
}

// copyValue deep-copies maps and lists, so a value shared by every record can
// be handed to each page on its own.
func copyValue(v interface{}) interface{} {
	return mapLeaves(v, func(leaf interface{}) interface{} { return leaf })
}

// mapLeaves copies maps and lists recursively, passing every other value
// through leaf.
func mapLeaves(v interface{}, leaf func(interface{}) interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, x := range t {
			out[k] = mapLeaves(x, leaf)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, x := range t {
			out[i] = mapLeaves(x, leaf)
		}
		return out
	}
	return leaf(v)
}
