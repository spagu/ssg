// Template collection & conditional helpers (v1.8.3, audit/feature.md).
//
// This file holds the shared reflection utilities used by every helper in
// template_helpers_*.go. Helpers take the collection as their FINAL argument so
// they compose naturally in Go template pipelines:
//
//	{{ .Site.Pages | where "Type" "guide" | sort "Modified" "desc" | first 5 }}
//
// Helpers work generically over []models.Page, slices of structs, pointers to
// structs, maps and primitives. Invalid usage returns a descriptive error during
// template execution — helpers never panic and never mutate their input.
package generator

import (
	"fmt"
	"reflect"
	"time"
)

// timeType is cached for the frequent time.Time special-casing in comparisons.
var timeType = reflect.TypeOf(time.Time{})

// indirectValue dereferences pointers and interfaces until it reaches a concrete
// value. It returns an invalid reflect.Value for nil pointers/interfaces so the
// caller can handle nil elements explicitly instead of panicking.
func indirectValue(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

// requireCollection validates that value is a slice or array (possibly behind
// pointers/interfaces) and returns it, or a descriptive helper-prefixed error.
func requireCollection(value any, helperName string) (reflect.Value, error) {
	if value == nil {
		return reflect.Value{}, fmt.Errorf("%s: expected a slice or array, got nil", helperName)
	}
	v := indirectValue(reflect.ValueOf(value))
	if !v.IsValid() {
		return reflect.Value{}, fmt.Errorf("%s: expected a slice or array, got a nil pointer", helperName)
	}
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return reflect.Value{}, fmt.Errorf("%s: expected a slice or array, got %s", helperName, v.Kind())
	}
	return v, nil
}

// getFieldOrKey reads a struct field or string-keyed map entry from item. A
// missing field/key is an error (no silent fallback, per spec).
func getFieldOrKey(item reflect.Value, name, helperName string) (reflect.Value, error) {
	v := indirectValue(item)
	if !v.IsValid() {
		return reflect.Value{}, fmt.Errorf("%s: cannot read field %q from a nil element", helperName, name)
	}
	switch v.Kind() {
	case reflect.Struct:
		f := v.FieldByName(name)
		if !f.IsValid() {
			return reflect.Value{}, fmt.Errorf("%s: field %q does not exist on %s", helperName, name, v.Type())
		}
		return f, nil
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return reflect.Value{}, fmt.Errorf("%s: map keys must be strings, got %s", helperName, v.Type().Key())
		}
		mv := v.MapIndex(reflect.ValueOf(name).Convert(v.Type().Key()))
		if !mv.IsValid() {
			return reflect.Value{}, fmt.Errorf("%s: key %q does not exist in map", helperName, name)
		}
		return mv, nil
	default:
		return reflect.Value{}, fmt.Errorf("%s: cannot read field %q from %s", helperName, name, v.Kind())
	}
}

// numericKind reports whether k is an integer or float kind.
func numericKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// asFloat converts any numeric reflect.Value to float64 for cross-type compares.
func asFloat(v reflect.Value) float64 {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint())
	default:
		return v.Float()
	}
}

// compareValues orders two values: -1 (a<b), 0 (equal), 1 (a>b). It supports
// strings, booleans (false < true), all numeric kinds (compared cross-type),
// time.Time and compatible aliases. Anything else is an error.
func compareValues(a, b reflect.Value) (int, error) {
	a, b = indirectValue(a), indirectValue(b)
	if !a.IsValid() || !b.IsValid() {
		return 0, fmt.Errorf("cannot compare nil values")
	}
	// time.Time (and aliases convertible to it) first — it is a struct kind.
	if a.Type().ConvertibleTo(timeType) && b.Type().ConvertibleTo(timeType) &&
		a.Kind() == reflect.Struct && b.Kind() == reflect.Struct {
		at := a.Convert(timeType).Interface().(time.Time)
		bt := b.Convert(timeType).Interface().(time.Time)
		switch {
		case at.Before(bt):
			return -1, nil
		case at.After(bt):
			return 1, nil
		default:
			return 0, nil
		}
	}
	if numericKind(a.Kind()) && numericKind(b.Kind()) {
		af, bf := asFloat(a), asFloat(b)
		switch {
		case af < bf:
			return -1, nil
		case af > bf:
			return 1, nil
		default:
			return 0, nil
		}
	}
	if a.Kind() == reflect.String && b.Kind() == reflect.String {
		as, bs := a.String(), b.String()
		switch {
		case as < bs:
			return -1, nil
		case as > bs:
			return 1, nil
		default:
			return 0, nil
		}
	}
	if a.Kind() == reflect.Bool && b.Kind() == reflect.Bool {
		ab, bb := a.Bool(), b.Bool()
		switch {
		case ab == bb:
			return 0, nil
		case !ab:
			return -1, nil
		default:
			return 1, nil
		}
	}
	return 0, fmt.Errorf("cannot compare %s with %s", a.Type(), b.Type())
}

// valuesEqual reports semantic equality: ordered types compare via compareValues
// (so int(5) equals float64(5) and string aliases match), everything else falls
// back to reflect.DeepEqual on the dereferenced values.
func valuesEqual(a, b reflect.Value) bool {
	if c, err := compareValues(a, b); err == nil {
		return c == 0
	}
	ai, bi := indirectValue(a), indirectValue(b)
	if !ai.IsValid() || !bi.IsValid() {
		return ai.IsValid() == bi.IsValid() // both nil → equal
	}
	return reflect.DeepEqual(ai.Interface(), bi.Interface())
}

// emptyLike returns a new empty slice matching v's element type. Arrays yield the
// equivalent slice type so every helper returns an appendable, rangeable result.
func emptyLike(v reflect.Value) reflect.Value {
	t := v.Type()
	if t.Kind() == reflect.Array {
		return reflect.MakeSlice(reflect.SliceOf(t.Elem()), 0, 0)
	}
	return reflect.MakeSlice(t, 0, v.Len())
}

// cloneSlice returns a copy of v as a slice (arrays are converted), so sorting
// and slicing never mutate the caller's collection.
func cloneSlice(v reflect.Value) reflect.Value {
	out := reflect.MakeSlice(reflect.SliceOf(v.Type().Elem()), v.Len(), v.Len())
	reflect.Copy(out, v)
	return out
}

// scalarGroupKey renders a scalar value as a deterministic string map key for
// groupBy/indexBy. Non-scalar kinds (slices, maps, non-time structs) error out.
func scalarGroupKey(v reflect.Value, helperName string) (string, error) {
	v = indirectValue(v)
	if !v.IsValid() {
		return "", fmt.Errorf("%s: cannot use a nil value as a group key", helperName)
	}
	if v.Kind() == reflect.Struct {
		if v.Type().ConvertibleTo(timeType) {
			return v.Convert(timeType).Interface().(time.Time).Format(time.RFC3339), nil
		}
		return "", fmt.Errorf("%s: cannot use %s as a group key", helperName, v.Type())
	}
	switch v.Kind() {
	case reflect.String:
		return v.String(), nil
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%v", v.Interface()), nil
	default:
		return "", fmt.Errorf("%s: cannot use %s as a group key", helperName, v.Kind())
	}
}
