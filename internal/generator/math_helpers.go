package generator

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strconv"
	"strings"
)

// Arithmetic helpers (TPL-003). Go templates have no arithmetic, so a theme
// could not split a list into columns, compute "page N of M" or offset an
// index without preprocessing the data in Go first. These four cover that gap
// and mirror the naming every other Go SSG uses.
//
// Values may be any integer or float kind (frontmatter numbers arrive as int
// or float64). Integer operands yield an integer result — including div, which
// divides integrally, the form a column split needs — while a float operand
// anywhere yields a float. So {{ add 1 2 }} is 3, {{ div 7 2 }} is 3 and
// {{ div 7.0 2 }} is 3.5.

// toFloat converts a template argument to a float, reporting whether it came
// from an integer kind and whether the conversion succeeded at all.
func toFloat(v interface{}) (value float64, isInt bool, ok bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true, true
	case int8:
		return float64(n), true, true
	case int16:
		return float64(n), true, true
	case int32:
		return float64(n), true, true
	case int64:
		return float64(n), true, true
	case uint:
		return float64(n), true, true
	case uint8:
		return float64(n), true, true
	case uint16:
		return float64(n), true, true
	case uint32:
		return float64(n), true, true
	case uint64:
		return float64(n), true, true
	case float32:
		return float64(n), false, true
	case float64:
		return n, false, true
	case string:
		return parseNumericString(n)
	}
	return 0, false, false
}

// parseNumericString accepts a number that arrived as text.
//
// Every shortcode attribute is a string — `[reviews limit="3"]` hands the
// template "3", not 3 — so without this a numeric option could not be used for
// anything numeric: `add 0 .Attrs.limit` failed with "both arguments must be
// numbers", and the honest workarounds were hardcoding or an `if eq $s "1"`
// chain (#253). An integer spelling stays an integer, so `add 1 "2"` is 3 and
// not 3.0. Text that is not a number is still not a number: it fails, rather
// than silently becoming zero.
func parseNumericString(s string) (value float64, isInt bool, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false, false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return float64(n), true, true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, false
	}
	return f, false, true
}

// arithmetic applies op to two template arguments, keeping integers integral.
func arithmetic(name string, a, b interface{}, op func(x, y float64) (float64, error)) (interface{}, error) {
	x, xInt, xOK := toFloat(a)
	y, yInt, yOK := toFloat(b)
	if !xOK || !yOK {
		return nil, fmt.Errorf("%s: both arguments must be numbers, got %T and %T", name, a, b)
	}
	result, err := op(x, y)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if xInt && yInt && result == float64(int64(result)) {
		return int64(result), nil
	}
	return result, nil
}

// tmplAdd implements {{ add 2 3 }}.
func tmplAdd(a, b interface{}) (interface{}, error) {
	return arithmetic("add", a, b, func(x, y float64) (float64, error) { return x + y, nil })
}

// tmplSub implements {{ sub 5 2 }}.
func tmplSub(a, b interface{}) (interface{}, error) {
	return arithmetic("sub", a, b, func(x, y float64) (float64, error) { return x - y, nil })
}

// tmplMul implements {{ mul 3 4 }}.
func tmplMul(a, b interface{}) (interface{}, error) {
	return arithmetic("mul", a, b, func(x, y float64) (float64, error) { return x * y, nil })
}

// tmplDiv implements {{ div 7 2 }}. Integer operands divide integrally
// (7/2 = 3), matching how a column split is expressed; a fractional operand
// yields a float. Division by zero is a template error rather than ±Inf.
func tmplDiv(a, b interface{}) (interface{}, error) {
	_, xInt, _ := toFloat(a)
	_, yInt, _ := toFloat(b)
	return arithmetic("div", a, b, func(x, y float64) (float64, error) {
		if y == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		if xInt && yInt {
			return float64(int64(x) / int64(y)), nil
		}
		return x / y, nil
	})
}

// tmplInt converts a value to a whole number, so a shortcode attribute can feed
// a helper that counts: {{ first (int .Attrs.limit) .Posts }} (#253).
//
// A float truncates toward zero, the conversion Go's own int() performs. A
// value that is not a number is a template error rather than a silent 0 —
// `[reviews limit="six"]` is a mistake worth seeing, and the alternative is a
// listing that renders nothing for no visible reason. `default` composes in
// front of it for the optional case: {{ .Attrs.limit | default "6" | int }}.
//
// It returns `int`, not int64, because the helpers it exists to feed take one:
// text/template will not convert between integer widths, so `first (int
// .Attrs.limit)` with an int64 fails at execution with "wrong type for value".
func tmplInt(v interface{}) (int, error) {
	f, _, ok := toFloat(v)
	if !ok {
		return 0, fmt.Errorf("int: not a number: %v", v)
	}
	return int(f), nil
}

// tmplFloat is the same conversion, keeping the fractional part — for a
// comparison like {{ filter "rating" "ge" (float .Attrs.min) $reviews }}.
func tmplFloat(v interface{}) (float64, error) {
	f, _, ok := toFloat(v)
	if !ok {
		return 0, fmt.Errorf("float: not a number: %v", v)
	}
	return f, nil
}

// tmplToJSON marshals a value to inline JSON for a theme — a config blob in a
// <script type="application/json">, JSON-LD, etc. It returns template.JS, not
// template.HTML: inside a <script> html/template uses a JS context and would
// JSON-encode a plain/HTML string a SECOND time (wrapping the object in quotes).
// template.JS is treated as already-safe there, so the object is emitted once.
// json.Marshal escapes <, > and & to \u escapes, so "</script>" cannot break
// out (TPL-004).
func tmplToJSON(v interface{}) (template.JS, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("toJSON: %w", err)
	}
	return template.JS(b), nil // #nosec G203 -- json.Marshal escapes <>& ; safe in a <script>
}
