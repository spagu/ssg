package generator

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestReflectionUtilsEdge exercises the shared utilities' error and fallback
// branches directly (nil handling, DeepEqual fallback, array→slice conversion).
func TestReflectionUtilsEdge(t *testing.T) {
	// requireCollection: nil and typed-nil pointers.
	if _, err := requireCollection(nil, "x"); err == nil || !strings.Contains(err.Error(), "got nil") {
		t.Errorf("nil collection err = %v", err)
	}
	var nilSlicePtr *[]int
	if _, err := requireCollection(nilSlicePtr, "x"); err == nil || !strings.Contains(err.Error(), "nil pointer") {
		t.Errorf("nil pointer err = %v", err)
	}
	// Arrays are accepted and produce appendable slices.
	arr := [3]int{3, 1, 2}
	if out, err := tmplSortBy("", "asc", arr); err == nil {
		_ = out // arrays of primitives have no fields — expect the field error instead
		t.Error("sort on primitive array without field should error")
	}
	if out, err := tmplFirst(2, arr); err != nil || reflect.ValueOf(out).Len() != 2 {
		t.Errorf("first on array = %v, %v", out, err)
	}
	if out, err := tmplReverse(arr); err != nil || reflect.ValueOf(out).Index(0).Int() != 2 {
		t.Errorf("reverse on array = %v, %v", out, err)
	}

	// getFieldOrKey: non-string map keys and unsupported kinds.
	if _, err := getFieldOrKey(reflect.ValueOf(map[int]string{1: "a"}), "k", "x"); err == nil ||
		!strings.Contains(err.Error(), "map keys must be strings") {
		t.Errorf("int-key map err = %v", err)
	}
	if _, err := getFieldOrKey(reflect.ValueOf(42), "k", "x"); err == nil ||
		!strings.Contains(err.Error(), "cannot read field") {
		t.Errorf("int element err = %v", err)
	}

	// compareValues: nils, bool ordering and time aliases.
	if _, err := compareValues(reflect.ValueOf((*int)(nil)), reflect.ValueOf(1)); err == nil {
		t.Error("comparing nil should error")
	}
	if c, err := compareValues(reflect.ValueOf(false), reflect.ValueOf(true)); err != nil || c != -1 {
		t.Errorf("bool compare = %d, %v", c, err)
	}
	if c, err := compareValues(reflect.ValueOf(true), reflect.ValueOf(false)); err != nil || c != 1 {
		t.Errorf("bool compare = %d, %v", c, err)
	}
	if c, err := compareValues(reflect.ValueOf(uint8(7)), reflect.ValueOf(int64(7))); err != nil || c != 0 {
		t.Errorf("uint/int compare = %d, %v", c, err)
	}

	// valuesEqual: DeepEqual fallback for slices, and nil-vs-nil / nil-vs-value.
	if !valuesEqual(reflect.ValueOf([]string{"a"}), reflect.ValueOf([]string{"a"})) {
		t.Error("slice DeepEqual fallback failed")
	}
	if valuesEqual(reflect.ValueOf([]string{"a"}), reflect.ValueOf([]string{"b"})) {
		t.Error("distinct slices reported equal")
	}
	var p *int
	if !valuesEqual(reflect.ValueOf(p), reflect.ValueOf(p)) {
		t.Error("nil vs nil should be equal")
	}
	if valuesEqual(reflect.ValueOf(p), reflect.ValueOf(1)) {
		t.Error("nil vs value should differ")
	}

	// scalarGroupKey: time, bool, nil and non-time structs.
	when := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if k, err := scalarGroupKey(reflect.ValueOf(when), "x"); err != nil || !strings.HasPrefix(k, "2026-02-01") {
		t.Errorf("time key = %q, %v", k, err)
	}
	if k, err := scalarGroupKey(reflect.ValueOf(true), "x"); err != nil || k != "true" {
		t.Errorf("bool key = %q, %v", k, err)
	}
	if _, err := scalarGroupKey(reflect.ValueOf((*int)(nil)), "x"); err == nil {
		t.Error("nil group key should error")
	}
	if _, err := scalarGroupKey(reflect.ValueOf(struct{ A int }{1}), "x"); err == nil {
		t.Error("struct group key should error")
	}
}

// TestHelperErrorPropagation covers the invalid-collection error path of every
// helper that wraps requireCollection.
func TestHelperErrorPropagation(t *testing.T) {
	bad := "not-a-collection"
	if _, err := tmplFilter("F", "eq", 1, bad); err == nil {
		t.Error("filter should propagate collection errors")
	}
	if _, err := tmplSortBy("F", "asc", bad); err == nil {
		t.Error("sort should propagate collection errors")
	}
	if _, err := tmplGroupBy("F", bad); err == nil {
		t.Error("groupBy should propagate collection errors")
	}
	if _, err := tmplUniq(bad); err == nil {
		t.Error("uniq should propagate collection errors")
	}
	if _, err := tmplUniqBy("F", bad); err == nil {
		t.Error("uniqBy should propagate collection errors")
	}
	if _, err := tmplReverse(bad); err == nil {
		t.Error("reverse should propagate collection errors")
	}
	if _, err := tmplPluck("F", bad); err == nil {
		t.Error("pluck should propagate collection errors")
	}
	if _, err := tmplIndexBy("F", bad); err == nil {
		t.Error("indexBy should propagate collection errors")
	}
	if _, err := tmplLatest("F", 1, bad); err == nil {
		t.Error("latest should propagate collection errors")
	}
	if _, err := tmplPublished(bad); err == nil {
		t.Error("published should propagate collection errors")
	}
	if _, err := tmplByTag("t", bad); err == nil {
		t.Error("byTag should propagate collection errors")
	}
	items := helperItems()
	if _, err := tmplUniqBy("Nope", items); err == nil {
		t.Error("uniqBy missing field should error")
	}
	if _, err := tmplUniqBy("Tags", items); err == nil {
		t.Error("uniqBy slice key should error")
	}
	if _, err := tmplFilter("Nope", "eq", 1, items); err == nil {
		t.Error("filter missing field should error")
	}
	if _, err := tmplLatest("Title", -1, items); err == nil {
		t.Error("latest negative count should error")
	}
}

// TestHelperNilAndKeyEdges covers nil-element and key-conversion branches.
func TestHelperNilAndKeyEdges(t *testing.T) {
	// pluck keeps nils for nil pointer elements.
	type box struct{ V *int }
	n := 7
	vals, err := tmplPluck("V", []box{{V: &n}, {V: nil}})
	if err != nil || len(vals.([]any)) != 2 || vals.([]any)[0] != 7 || vals.([]any)[1] != nil {
		t.Errorf("pluck nil element = %v, %v", vals, err)
	}
	// indexBy rejects nil elements and accepts time keys.
	if _, err := tmplIndexBy("Title", []*helperItem{nil}); err == nil {
		t.Error("indexBy nil element should error")
	}
	byTime, err := tmplIndexBy("Modified", helperItems())
	if err != nil || len(byTime) != 3 {
		t.Errorf("indexBy time keys = %v, %v", byTime, err)
	}
	// containerHas: nil container, and a map whose key type rejects the value.
	if _, err := tmplContains(nil, "x"); err == nil {
		t.Error("contains nil container should error")
	}
	if _, err := tmplContains(map[string]int{"a": 1}, []int{1}); err == nil {
		t.Error("contains bad map key type should error")
	}
	// uniq dedupes unsigned ints via the scalar key.
	u, err := tmplUniq([]uint8{1, 2, 1})
	if err != nil || reflect.ValueOf(u).Len() != 2 {
		t.Errorf("uniq uint = %v, %v", u, err)
	}
	// where on a map missing the requested key errors.
	if _, err := tmplWhere("Nope", 1, []map[string]int{{"A": 1}}); err == nil {
		t.Error("where missing map key should error")
	}
	// byAuthor: unknown author matches nothing; invalid collection errors.
	g := newTestGen(t, "")
	out, err := g.tmplByAuthor("ghost", contentPages())
	if err != nil || reflect.ValueOf(out).Len() != 0 {
		t.Errorf("byAuthor unknown = %v, %v", out, err)
	}
	if _, err := g.tmplByAuthor("x", 42); err == nil {
		t.Error("byAuthor wants []models.Page")
	}
}
