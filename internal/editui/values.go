package editui

// Turning what a form sends into what a frontmatter field holds (GO-102).
//
// A browser sends every field as text. A frontmatter field is a string, a
// number, a boolean, a date or a list, and writing "true" where a boolean
// belongs produces a file the build reads differently from the one the author
// thought they saved. The declared type from `content_schemas` decides; where
// nothing is declared, the value stays the text it arrived as, because guessing
// is how a version number becomes a float.

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// dateLayouts are the spellings a date field accepts, in the order they are
// tried. The first is what a save writes back.
var dateLayouts = []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04"}

// typedValue converts a form value according to a declared field type.
func typedValue(declared, raw string) (interface{}, error) {
	value := strings.TrimSpace(raw)
	switch declared {
	case "int":
		n, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("%q is not a whole number", raw)
		}
		return n, nil
	case "bool":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("%q is not true or false", raw)
		}
		return b, nil
	case "date":
		for _, layout := range dateLayouts {
			if t, err := time.Parse(layout, value); err == nil {
				return t.Format(dateLayouts[0]), nil
			}
		}
		return nil, fmt.Errorf("%q is not a date (write it as 2026-09-10)", raw)
	case "url":
		u, err := url.Parse(value)
		if err != nil || (u.Scheme == "" && !strings.HasPrefix(value, "/")) {
			return nil, fmt.Errorf("%q is not a URL or an absolute path", raw)
		}
		return value, nil
	case "list":
		return splitList(value), nil
	}
	// Undeclared, or "string": the text as it came. A frontmatter value that
	// looks like a number stays text, which is what an author who typed it
	// into a text box meant.
	return raw, nil
}

// splitList reads the comma-separated form a text box can carry.
func splitList(s string) []interface{} {
	out := []interface{}{}
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// stringOf renders a frontmatter value for a form control.
func stringOf(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case time.Time:
		return t.Format(dateLayouts[0])
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			parts = append(parts, stringOf(item))
		}
		return strings.Join(parts, ", ")
	case []string:
		return strings.Join(t, ", ")
	case map[string]interface{}:
		// A nested block is shown but not edited here: a text box is the wrong
		// control for a structure, and pretending otherwise would let a save
		// flatten it.
		return fmt.Sprintf("(%d nested keys — edit this one in the file)", len(t))
	}
	return fmt.Sprintf("%v", v)
}

// sortedKeys lists a map's keys in a stable order, so the form does not
// reshuffle itself between loads.
func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
