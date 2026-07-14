// P1 conditional helpers: in, notIn, contains, startsWith, endsWith, matches,
// isNil, isEmpty, ternary (v1.8.3, audit/feature.md). startsWith/endsWith are
// registered directly as strings.HasPrefix/HasSuffix.
package generator

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	texttemplate "text/template"
)

// tmplIn reports whether value exists in a slice/array collection. Canonical
// signature: value first, collection second —
//
//	{{ if in .Page.Type (slice "guide" "tutorial" "docs") }} … {{ end }}
func tmplIn(value any, collection any) (bool, error) {
	v, err := requireCollection(collection, "in")
	if err != nil {
		return false, err
	}
	want := reflect.ValueOf(value)
	for i := 0; i < v.Len(); i++ {
		if valuesEqual(v.Index(i), want) {
			return true, nil
		}
	}
	return false, nil
}

// tmplNotIn is the negation of in:
//
//	{{ if notIn .Page.Type (slice "draft" "private") }} … {{ end }}
func tmplNotIn(value any, collection any) (bool, error) {
	ok, err := tmplIn(value, collection)
	return !ok, err
}

// containerHas implements contains semantics for a reflect.Value container:
// string→substring, slice/array→element equality, map→key presence.
func containerHas(container, value reflect.Value, helperName string) (bool, error) {
	c := indirectValue(container)
	if !c.IsValid() {
		return false, fmt.Errorf("%s: cannot search inside a nil container", helperName)
	}
	switch c.Kind() {
	case reflect.String:
		val := indirectValue(value)
		if !val.IsValid() || val.Kind() != reflect.String {
			return false, fmt.Errorf("%s: searching a string requires a string value", helperName)
		}
		return strings.Contains(c.String(), val.String()), nil
	case reflect.Slice, reflect.Array:
		for i := 0; i < c.Len(); i++ {
			if valuesEqual(c.Index(i), value) {
				return true, nil
			}
		}
		return false, nil
	case reflect.Map:
		val := indirectValue(value)
		if !val.IsValid() {
			return false, fmt.Errorf("%s: cannot use nil as a map key", helperName)
		}
		if !val.Type().ConvertibleTo(c.Type().Key()) {
			return false, fmt.Errorf("%s: %s is not a valid key type for %s", helperName, val.Type(), c.Type())
		}
		return c.MapIndex(val.Convert(c.Type().Key())).IsValid(), nil
	default:
		return false, fmt.Errorf("%s: expected a string, slice, array or map container, got %s", helperName, c.Kind())
	}
}

// tmplContains checks substring / slice element / map key membership:
//
//	{{ if contains .Page.Title "Go" }} … {{ if contains .Page.Tags "ssg" }} …
func tmplContains(container any, value any) (bool, error) {
	return containerHas(reflect.ValueOf(container), reflect.ValueOf(value), "contains")
}

// matchCache stores compiled regular expressions so repeated template calls do
// not recompile the same pattern (spec: cache compiled expressions).
var matchCache sync.Map // pattern string → *regexp.Regexp

// tmplMatches reports whether value matches the RE2 pattern:
//
//	{{ if matches `^guide-` .Page.Slug }} … {{ end }}
func tmplMatches(pattern, value string) (bool, error) {
	if cached, ok := matchCache.Load(pattern); ok {
		return cached.(*regexp.Regexp).MatchString(value), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("matches: invalid regular expression %q: %w", pattern, err)
	}
	matchCache.Store(pattern, re)
	return re.MatchString(value), nil
}

// tmplIsNil reports whether value is nil, including typed nil pointers, maps,
// slices, funcs and channels. It never panics on non-nilable values.
func tmplIsNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return v.IsNil()
	default:
		return false
	}
}

// tmplIsEmpty mirrors Go template truthiness: nil, "", 0, false, empty
// slices/arrays/maps and nil pointers are empty; structs never are (so a zero
// time.Time is NOT empty — compare with .IsZero instead).
func tmplIsEmpty(value any) bool {
	truth, ok := texttemplate.IsTrue(value)
	return !ok || !truth
}

// tmplTernary picks between two values on a boolean condition:
//
//	{{ ternary .Page.HasMath "math" "plain" }}
func tmplTernary(condition bool, trueValue, falseValue any) any {
	if condition {
		return trueValue
	}
	return falseValue
}
