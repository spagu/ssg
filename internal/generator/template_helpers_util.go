// P1 utility helpers: groupBy, uniq, reverse, slice, pluck, indexBy
// (v1.8.3, audit/feature.md).
package generator

import (
	"fmt"
	"reflect"
)

// tmplGroupBy groups elements by a scalar field into map[key]→typed slice.
// Go templates iterate maps in sorted key order, so output is deterministic:
//
//	{{ range $category, $pages := (.Site.Pages | groupBy "Category") }} … {{ end }}
func tmplGroupBy(field string, collection any) (map[string]any, error) {
	v, err := requireCollection(collection, "groupBy")
	if err != nil {
		return nil, err
	}
	sliceType := emptyLike(v).Type()
	groups := map[string]reflect.Value{}
	order := []string{}
	for i := 0; i < v.Len(); i++ {
		fv, err := getFieldOrKey(v.Index(i), field, "groupBy")
		if err != nil {
			return nil, err
		}
		key, err := scalarGroupKey(fv, "groupBy")
		if err != nil {
			return nil, err
		}
		g, ok := groups[key]
		if !ok {
			g = reflect.MakeSlice(sliceType, 0, 1)
			order = append(order, key)
		}
		groups[key] = reflect.Append(g, v.Index(i))
	}
	out := make(map[string]any, len(order))
	for _, k := range order {
		out[k] = groups[k].Interface()
	}
	return out, nil
}

// tmplUniq removes duplicate comparable primitives, keeping first occurrences:
//
//	{{ .Site.Pages | pluck "Category" | uniq }}
func tmplUniq(collection any) (any, error) {
	v, err := requireCollection(collection, "uniq")
	if err != nil {
		return nil, err
	}
	out := emptyLike(v)
	seen := map[string]bool{}
	for i := 0; i < v.Len(); i++ {
		key, err := scalarGroupKey(v.Index(i), "uniq")
		if err != nil {
			return nil, fmt.Errorf("uniq: only primitive values are supported; use pluck first or uniqBy (%w)", err)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = reflect.Append(out, v.Index(i))
	}
	return out.Interface(), nil
}

// tmplUniqBy removes elements whose field value repeats, keeping first occurrences:
//
//	{{ .Site.Pages | uniqBy "Category" }}
func tmplUniqBy(field string, collection any) (any, error) {
	v, err := requireCollection(collection, "uniqBy")
	if err != nil {
		return nil, err
	}
	out := emptyLike(v)
	seen := map[string]bool{}
	for i := 0; i < v.Len(); i++ {
		fv, err := getFieldOrKey(v.Index(i), field, "uniqBy")
		if err != nil {
			return nil, err
		}
		key, err := scalarGroupKey(fv, "uniqBy")
		if err != nil {
			return nil, err
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = reflect.Append(out, v.Index(i))
	}
	return out.Interface(), nil
}

// tmplReverse returns a reversed copy: {{ .Site.Pages | reverse }}
func tmplReverse(collection any) (any, error) {
	v, err := requireCollection(collection, "reverse")
	if err != nil {
		return nil, err
	}
	out := emptyLike(v)
	for i := v.Len() - 1; i >= 0; i-- {
		out = reflect.Append(out, v.Index(i))
	}
	return out.Interface(), nil
}

// tmplSliceOf builds a []any from its arguments: {{ slice "guide" "tutorial" }}.
// NOTE: registering this overrides Go's builtin slice(str, i, j) sub-slicing —
// documented in docs/TEMPLATE_HELPERS.md.
func tmplSliceOf(values ...any) []any {
	out := make([]any, len(values))
	copy(out, values)
	return out
}

// tmplPluck extracts one field from every element into a []any:
//
//	{{ $titles := .Site.Pages | pluck "Title" }}
func tmplPluck(field string, collection any) (any, error) {
	v, err := requireCollection(collection, "pluck")
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		fv, err := getFieldOrKey(v.Index(i), field, "pluck")
		if err != nil {
			return nil, err
		}
		fv = indirectValue(fv)
		if !fv.IsValid() {
			out = append(out, nil)
			continue
		}
		out = append(out, fv.Interface())
	}
	return out, nil
}

// tmplIndexBy builds a lookup map keyed by a field. Duplicate and empty keys are
// errors so silent data loss cannot happen:
//
//	{{ $bySlug := .Site.Pages | indexBy "Slug" }}{{ $page := index $bySlug "intro" }}
func tmplIndexBy(field string, collection any) (map[string]any, error) {
	v, err := requireCollection(collection, "indexBy")
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, v.Len())
	for i := 0; i < v.Len(); i++ {
		fv, err := getFieldOrKey(v.Index(i), field, "indexBy")
		if err != nil {
			return nil, err
		}
		key, err := scalarGroupKey(fv, "indexBy")
		if err != nil {
			return nil, err
		}
		if key == "" {
			return nil, fmt.Errorf("indexBy: element %d has an empty %q key", i, field)
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("indexBy: duplicate key %q for field %q", key, field)
		}
		item := indirectValue(v.Index(i))
		if !item.IsValid() {
			return nil, fmt.Errorf("indexBy: element %d is nil", i)
		}
		out[key] = item.Interface()
	}
	return out, nil
}
