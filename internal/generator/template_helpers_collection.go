// P0 collection helpers: where, filter, sort, first, last, limit, offset
// (v1.8.3, audit/feature.md). The collection is always the final argument so
// the helpers chain in pipelines. None of them mutate their input.
package generator

import (
	"fmt"
	"sort"
	"strings"

	"reflect"
)

// tmplWhere filters a collection to elements whose field/key equals expected.
//
//	{{ .Site.Pages | where "Type" "guide" }}
func tmplWhere(field string, expected any, collection any) (any, error) {
	v, err := requireCollection(collection, "where")
	if err != nil {
		return nil, err
	}
	out := emptyLike(v)
	want := reflect.ValueOf(expected)
	for i := 0; i < v.Len(); i++ {
		fv, err := getFieldOrKey(v.Index(i), field, "where")
		if err != nil {
			return nil, err
		}
		if valuesEqual(fv, want) {
			out = reflect.Append(out, v.Index(i))
		}
	}
	return out.Interface(), nil
}

// filterMatch evaluates one filter operator against a field value.
func filterMatch(fv reflect.Value, operator string, want reflect.Value) (bool, error) {
	switch operator {
	case "eq":
		return valuesEqual(fv, want), nil
	case "ne":
		return !valuesEqual(fv, want), nil
	case "gt", "ge", "lt", "le":
		c, err := compareValues(fv, want)
		if err != nil {
			return false, fmt.Errorf("filter: %w", err)
		}
		switch operator {
		case "gt":
			return c > 0, nil
		case "ge":
			return c >= 0, nil
		case "lt":
			return c < 0, nil
		default:
			return c <= 0, nil
		}
	case "contains", "notContains":
		ok, err := containerHas(fv, want, "filter")
		if err != nil {
			return false, err
		}
		if operator == "notContains" {
			return !ok, nil
		}
		return ok, nil
	case "in", "notIn":
		set, err := requireCollection(want.Interface(), "filter (operator "+operator+")")
		if err != nil {
			return false, err
		}
		found := false
		for j := 0; j < set.Len(); j++ {
			if valuesEqual(fv, set.Index(j)) {
				found = true
				break
			}
		}
		if operator == "notIn" {
			return !found, nil
		}
		return found, nil
	case "hasPrefix", "hasSuffix", "matches":
		// String tests (#96). hasPrefix, hasSuffix and matches already existed as
		// standalone helpers but could not be used as filter predicates, so
		// "pages under /special/" had no expression: contains is substring-wise,
		// which also matches /not-special/, and on a []string field it tests
		// membership rather than text at all.
		return stringFilterMatch(fv, operator, want)
	default:
		return false, fmt.Errorf("filter: unsupported operator %q; expected one of eq, ne, gt, ge, lt, le, contains, notContains, in, notIn, hasPrefix, hasSuffix, matches", operator)
	}
}

// tmplFilter filters a collection with an explicit operator.
//
//	{{ .Site.Pages | filter "Tags" "contains" "go" }}
//	{{ .Site.Pages | filter "Type" "in" (slice "guide" "tutorial") }}
func tmplFilter(field, operator string, expected any, collection any) (any, error) {
	v, err := requireCollection(collection, "filter")
	if err != nil {
		return nil, err
	}
	out := emptyLike(v)
	want := reflect.ValueOf(expected)
	for i := 0; i < v.Len(); i++ {
		fv, err := getFieldOrKey(v.Index(i), field, "filter")
		if err != nil {
			return nil, err
		}
		ok, err := filterMatch(fv, operator, want)
		if err != nil {
			return nil, err
		}
		if ok {
			out = reflect.Append(out, v.Index(i))
		}
	}
	return out.Interface(), nil
}

// tmplSortBy returns a stably sorted copy of a collection, ordered by a field.
//
//	{{ .Site.Pages | sort "Modified" "desc" }}
func tmplSortBy(field, direction string, collection any) (any, error) {
	desc := false
	switch strings.ToLower(direction) {
	case "asc":
	case "desc":
		desc = true
	default:
		return nil, fmt.Errorf("sort: unsupported direction %q; expected \"asc\" or \"desc\"", direction)
	}
	v, err := requireCollection(collection, "sort")
	if err != nil {
		return nil, err
	}
	if v.Len() == 0 {
		return emptyLike(v).Interface(), nil
	}
	// Extract keys up front so field/type errors surface before sorting begins.
	keys := make([]reflect.Value, v.Len())
	for i := 0; i < v.Len(); i++ {
		fv, err := getFieldOrKey(v.Index(i), field, "sort")
		if err != nil {
			return nil, err
		}
		keys[i] = fv
	}
	for i := 1; i < len(keys); i++ { // validate comparability against the first key
		if _, err := compareValues(keys[0], keys[i]); err != nil {
			return nil, fmt.Errorf("sort: %w", err)
		}
	}
	out := cloneSlice(v)
	order := make([]int, v.Len())
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		c, _ := compareValues(keys[order[a]], keys[order[b]]) // pre-validated above
		if desc {
			return c > 0
		}
		return c < 0
	})
	sorted := reflect.MakeSlice(out.Type(), v.Len(), v.Len())
	for i, idx := range order {
		sorted.Index(i).Set(v.Index(idx))
	}
	return sorted.Interface(), nil
}

// takeSlice implements first/last/limit/offset bounds handling on a copy.
func takeSlice(count int, collection any, helperName string, fromEnd, skip bool) (any, error) {
	if count < 0 {
		return nil, fmt.Errorf("%s: count must be greater than or equal to zero", helperName)
	}
	v, err := requireCollection(collection, helperName)
	if err != nil {
		return nil, err
	}
	n := v.Len()
	if count > n {
		count = n
	}
	var lo, hi int
	switch {
	case skip: // offset: drop the first count elements
		lo, hi = count, n
	case fromEnd: // last
		lo, hi = n-count, n
	default: // first / limit
		lo, hi = 0, count
	}
	out := emptyLike(v)
	for i := lo; i < hi; i++ {
		out = reflect.Append(out, v.Index(i))
	}
	return out.Interface(), nil
}

// tmplFirst returns the first count elements: {{ .Site.Pages | first 5 }}
func tmplFirst(count int, collection any) (any, error) {
	return takeSlice(count, collection, "first", false, false)
}

// tmplLast returns the last count elements: {{ .Site.Pages | last 5 }}
func tmplLast(count int, collection any) (any, error) {
	return takeSlice(count, collection, "last", true, false)
}

// tmplLimit is a query-style alias of first:
//
//	{{ .Site.Pages | sort "Modified" "desc" | limit 5 }}
func tmplLimit(count int, collection any) (any, error) {
	return takeSlice(count, collection, "limit", false, false)
}

// tmplOffset skips the first count elements: {{ .Site.Pages | offset 10 | limit 10 }}
func tmplOffset(count int, collection any) (any, error) {
	return takeSlice(count, collection, "offset", false, true)
}

// tmplAppend returns a new collection with the extra values added (#96).
//
// A theme that needs a derived collection — "the sub-pages of this section" —
// has to accumulate one, and `slice` only builds a literal. Without this the
// pattern has no direct expression in a Go template: the loop that would gather
// the pages cannot record what it finds.
//
// Argument order is deliberately looser than the rest of this file, which puts
// the collection last so helpers chain in pipelines. Both spellings are in use
// and both are natural:
//
//	{{ $kids = append $kids . }}   // Go's own append(slice, elems...)
//	{{ $kids = $kids | append . }} // this file's pipeline convention
//
// so the collection is whichever end holds one. When both ends are collections
// the first wins, matching Go's builtin — that is the only ambiguous case, and
// appending one slice to another is the reading people expect.
//
// The input is never mutated: reflect.Append may reuse the backing array, so a
// fresh slice is allocated and copied into first. Skipping that would let two
// templates appending to the same base collection overwrite each other.
func tmplAppend(args ...any) (any, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("append: need a collection and at least one value; see --help")
	}
	first, last := reflect.ValueOf(args[0]), reflect.ValueOf(args[len(args)-1])
	var base reflect.Value
	var items []any
	switch {
	case isCollectionKind(first):
		base, items = first, args[1:]
	case isCollectionKind(last):
		base, items = last, args[:len(args)-1]
	default:
		return nil, fmt.Errorf("append: neither the first nor the last argument is a list; see --help")
	}

	// Copy rather than append in place — see the note above.
	out := reflect.MakeSlice(reflect.SliceOf(base.Type().Elem()), 0, base.Len()+len(items))
	out = reflect.AppendSlice(out, base)

	elem := base.Type().Elem()
	for _, it := range items {
		iv := reflect.ValueOf(it)
		if !iv.IsValid() {
			// A nil value has no type to check against elem; widen and keep it,
			// rather than dropping it silently.
			return appendWidened(base, items), nil
		}
		if !iv.Type().AssignableTo(elem) {
			// Mixed types: widen to []any instead of failing. A template
			// gathering pages of more than one concrete type is doing something
			// reasonable, and an error here would be a dead end.
			return appendWidened(base, items), nil
		}
		out = reflect.Append(out, iv)
	}
	return out.Interface(), nil
}

// appendWidened builds an []any when the values do not share the collection's
// element type.
func appendWidened(base reflect.Value, items []any) any {
	out := make([]any, 0, base.Len()+len(items))
	for i := 0; i < base.Len(); i++ {
		out = append(out, base.Index(i).Interface())
	}
	return append(out, items...)
}

// isCollectionKind reports whether v is a slice or array, the two shapes every
// helper in this file accepts as a collection.
func isCollectionKind(v reflect.Value) bool {
	return v.IsValid() && (v.Kind() == reflect.Slice || v.Kind() == reflect.Array)
}

// stringFilterMatch applies a string predicate to one field value (#96).
//
// Both sides must be strings: applying a prefix test to a []string field is a
// mistake worth reporting rather than quietly answering false, which is how the
// same mistake with `contains` currently hides.
func stringFilterMatch(fv reflect.Value, operator string, want reflect.Value) (bool, error) {
	if fv.Kind() != reflect.String || !want.IsValid() || want.Kind() != reflect.String {
		return false, fmt.Errorf("filter: operator %q needs a string field and a string value; see --help", operator)
	}
	got, pattern := fv.String(), want.String()
	switch operator {
	case "hasPrefix":
		return strings.HasPrefix(got, pattern), nil
	case "hasSuffix":
		return strings.HasSuffix(got, pattern), nil
	default:
		return tmplMatches(pattern, got)
	}
}
