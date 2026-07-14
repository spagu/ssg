package generator

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// helperItem is the struct used across the generic-helper tests.
type helperItem struct {
	Title    string
	Type     string
	Draft    bool
	Views    int
	Score    float64
	Modified time.Time
	Tags     []string
}

func ht(day int) time.Time { return time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC) }

func helperItems() []helperItem {
	return []helperItem{
		{Title: "B", Type: "guide", Draft: false, Views: 20, Score: 2.5, Modified: ht(2), Tags: []string{"go", "ssg"}},
		{Title: "A", Type: "post", Draft: true, Views: 10, Score: 1.5, Modified: ht(3), Tags: []string{"news"}},
		{Title: "C", Type: "guide", Draft: false, Views: 30, Score: 3.5, Modified: ht(1), Tags: []string{"go"}},
	}
}

// assertHelperResult verifies the (result, err) pair of a helper call against an
// expected Title list or an expected error substring.
func assertHelperResult(t *testing.T, got any, err error, want, wantErr string) {
	t.Helper()
	if wantErr != "" {
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("err = %v, want contains %q", err, wantErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if titles := titlesOf(t, got); titles != want {
		t.Errorf("result = %q, want %q", titles, want)
	}
}

// titlesOf extracts Title from a helper result (structs or maps) for assertions.
func titlesOf(t *testing.T, collection any) string {
	t.Helper()
	v := reflect.ValueOf(collection)
	parts := make([]string, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		fv, err := getFieldOrKey(v.Index(i), "Title", "titlesOf")
		if err != nil {
			t.Fatalf("titlesOf: %v", err)
		}
		parts = append(parts, indirectValue(fv).String())
	}
	return strings.Join(parts, ",")
}

func TestWhere(t *testing.T) {
	items := helperItems()
	ptrs := []*helperItem{&items[0], &items[1], &items[2]}
	maps := []map[string]any{{"Type": "guide", "Title": "M1"}, {"Type": "post", "Title": "M2"}}

	cases := []struct {
		name       string
		field      string
		expected   any
		collection any
		want       string
		wantErr    string
	}{
		{"struct string", "Type", "guide", items, "B,C", ""},
		{"struct bool", "Draft", false, items, "B,C", ""},
		{"struct int", "Views", 10, items, "A", ""},
		{"numeric cross-type", "Views", 10.0, items, "A", ""},
		{"pointer slice", "Type", "guide", ptrs, "B,C", ""},
		{"map slice", "Type", "guide", maps, "M1", ""},
		{"empty collection", "Type", "guide", []helperItem{}, "", ""},
		{"missing field", "Nope", "x", items, "", `field "Nope" does not exist`},
		{"invalid collection", "Type", "x", "not-a-slice", "", "expected a slice or array, got string"},
		{"nil pointer element", "Type", "guide", []*helperItem{nil}, "", "nil element"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := tmplWhere(c.field, c.expected, c.collection)
			assertHelperResult(t, got, err, c.want, c.wantErr)
		})
	}
}

func TestFilterOperators(t *testing.T) {
	items := helperItems()
	cases := []struct {
		name     string
		field    string
		operator string
		expected any
		want     string
		wantErr  string
	}{
		{"eq", "Type", "eq", "guide", "B,C", ""},
		{"ne", "Type", "ne", "guide", "A", ""},
		{"gt int", "Views", "gt", 15, "B,C", ""},
		{"ge float", "Score", "ge", 2.5, "B,C", ""},
		{"lt time", "Modified", "lt", ht(2), "C", ""},
		{"le", "Views", "le", 20, "B,A", ""},
		{"contains slice", "Tags", "contains", "go", "B,C", ""},
		{"contains string", "Title", "contains", "A", "A", ""},
		{"notContains", "Tags", "notContains", "go", "A", ""},
		{"in", "Type", "in", []string{"guide", "tutorial"}, "B,C", ""},
		{"notIn", "Type", "notIn", []string{"guide"}, "A", ""},
		{"bad operator", "Type", "newest", "x", "", `unsupported operator "newest"`},
		{"in wants collection", "Type", "in", 42, "", "expected a slice or array"},
		{"gt type mismatch", "Type", "gt", 5, "", "cannot compare"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := tmplFilter(c.field, c.operator, c.expected, items)
			assertHelperResult(t, got, err, c.want, c.wantErr)
		})
	}
}

func TestSortBy(t *testing.T) {
	items := helperItems()
	cases := []struct {
		name      string
		field     string
		direction string
		want      string
		wantErr   string
	}{
		{"strings asc", "Title", "asc", "A,B,C", ""},
		{"strings desc", "Title", "desc", "C,B,A", ""},
		{"ints asc", "Views", "asc", "A,B,C", ""},
		{"floats desc", "Score", "desc", "C,B,A", ""},
		{"time desc", "Modified", "desc", "A,B,C", ""},
		{"bools asc", "Draft", "asc", "B,C,A", ""}, // stable: B,C keep source order
		{"invalid direction", "Title", "newest", "", `unsupported direction "newest"`},
		{"missing field", "Nope", "asc", "", `field "Nope" does not exist`},
		{"unsupported type", "Tags", "asc", "", "cannot compare"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := tmplSortBy(c.field, c.direction, items)
			assertHelperResult(t, got, err, c.want, c.wantErr)
		})
	}

	// The original collection must never be mutated.
	if titlesOf(t, items) != "B,A,C" {
		t.Errorf("sort mutated its input: %s", titlesOf(t, items))
	}
	// Empty collections sort without error.
	if out, err := tmplSortBy("Title", "asc", []helperItem{}); err != nil || reflect.ValueOf(out).Len() != 0 {
		t.Errorf("empty sort = %v, %v", out, err)
	}
}

func TestPaginationHelpers(t *testing.T) {
	items := helperItems()
	type fn func(int, any) (any, error)
	helpers := map[string]fn{"first": tmplFirst, "last": tmplLast, "limit": tmplLimit, "offset": tmplOffset}
	wants := map[string]map[int]string{
		"first":  {0: "", 2: "B,A", 3: "B,A,C", 99: "B,A,C"},
		"limit":  {0: "", 2: "B,A", 3: "B,A,C", 99: "B,A,C"},
		"last":   {0: "", 2: "A,C", 3: "B,A,C", 99: "B,A,C"},
		"offset": {0: "B,A,C", 2: "C", 3: "", 99: ""},
	}
	for name, helper := range helpers {
		for count, want := range wants[name] {
			got, err := helper(count, items)
			if err != nil {
				t.Fatalf("%s(%d): %v", name, count, err)
			}
			if titles := titlesOf(t, got); titles != want {
				t.Errorf("%s(%d) = %q, want %q", name, count, titles, want)
			}
		}
		if _, err := helper(-1, items); err == nil {
			t.Errorf("%s(-1) should error", name)
		}
		if out, err := helper(2, []helperItem{}); err != nil || reflect.ValueOf(out).Len() != 0 {
			t.Errorf("%s on empty = %v, %v", name, out, err)
		}
	}
	if titlesOf(t, items) != "B,A,C" {
		t.Error("pagination mutated its input")
	}
}

func TestGroupByUniqReverse(t *testing.T) {
	items := helperItems()
	groups, err := tmplGroupBy("Type", items)
	if err != nil {
		t.Fatalf("groupBy: %v", err)
	}
	if len(groups) != 2 || titlesOf(t, groups["guide"]) != "B,C" || titlesOf(t, groups["post"]) != "A" {
		t.Errorf("groupBy = %#v", groups)
	}
	if _, err := tmplGroupBy("Nope", items); err == nil {
		t.Error("groupBy missing field should error")
	}
	if _, err := tmplGroupBy("Tags", items); err == nil {
		t.Error("groupBy on a slice field should error")
	}

	u, err := tmplUniq([]string{"a", "b", "a", "c", "b"})
	if err != nil || !reflect.DeepEqual(u, []string{"a", "b", "c"}) {
		t.Errorf("uniq = %v, %v", u, err)
	}
	if _, err := tmplUniq(items); err == nil {
		t.Error("uniq on structs should error")
	}
	ub, err := tmplUniqBy("Type", items)
	if err != nil || titlesOf(t, ub) != "B,A" {
		t.Errorf("uniqBy = %v, %v", ub, err)
	}

	r, err := tmplReverse(items)
	if err != nil || titlesOf(t, r) != "C,A,B" {
		t.Errorf("reverse = %v, %v", r, err)
	}
	if titlesOf(t, items) != "B,A,C" {
		t.Error("reverse mutated its input")
	}
}

func TestSlicePluckIndexBy(t *testing.T) {
	s := tmplSliceOf("guide", "tutorial", 3)
	if len(s) != 3 || s[0] != "guide" || s[2] != 3 {
		t.Errorf("slice = %v", s)
	}

	items := helperItems()
	titles, err := tmplPluck("Title", items)
	if err != nil || !reflect.DeepEqual(titles, []any{"B", "A", "C"}) {
		t.Errorf("pluck = %v, %v", titles, err)
	}
	if _, err := tmplPluck("Nope", items); err == nil {
		t.Error("pluck missing field should error")
	}

	idx, err := tmplIndexBy("Title", items)
	if err != nil || idx["B"].(helperItem).Views != 20 {
		t.Errorf("indexBy = %v, %v", idx, err)
	}
	if _, err := tmplIndexBy("Title", []helperItem{{Title: "X"}, {Title: "X"}}); err == nil ||
		!strings.Contains(err.Error(), "duplicate key") {
		t.Errorf("indexBy duplicate = %v", err)
	}
	if _, err := tmplIndexBy("Title", []helperItem{{Title: ""}}); err == nil ||
		!strings.Contains(err.Error(), "empty") {
		t.Errorf("indexBy empty key = %v", err)
	}
}

// contentPages is the shared fixture for the content-helper tests.
func contentPages() []models.Page {
	return []models.Page{
		{ID: 1, Title: "P1", Slug: "p1", Status: "publish", Author: 1, Date: ht(1), Modified: ht(3),
			Tags: []string{"go"}, Categories: []int{2}},
		{ID: 2, Title: "P2", Slug: "p2", Status: "draft", Author: 2, Date: ht(2), Modified: ht(1),
			Tags: []string{"go", "web"}, Categories: []int{2}},
		{ID: 3, Title: "P3", Slug: "p3", Status: "publish", Author: 1, Date: ht(3), Modified: ht(2),
			Category: "Guides"},
	}
}

func TestContentHelpersGeneric(t *testing.T) {
	pages := contentPages()
	out, err := tmplLatest("Modified", 2, pages)
	assertHelperResult(t, out, err, "P1,P3", "")
	if _, err := tmplLatest("Nope", 2, pages); err == nil {
		t.Error("latest missing field should error")
	}
	out, err = tmplPublished(pages)
	assertHelperResult(t, out, err, "P1,P3", "")
	out, err = tmplByTag("web", pages)
	assertHelperResult(t, out, err, "P2", "")
}

func TestContentHelpersSiteData(t *testing.T) {
	pages := contentPages()
	g := newTestGen(t, "")
	g.siteData.Categories[2] = models.Category{ID: 2, Name: "News", Slug: "news"}
	g.siteData.Authors = map[int]models.Author{1: {ID: 1, Name: "Ada", Slug: "ada"}}

	out, err := g.tmplByCategory("news", pages)
	assertHelperResult(t, out, err, "P1,P2", "")
	out, err = g.tmplByCategory("guides", pages)
	assertHelperResult(t, out, err, "P3", "")
	if _, err := g.tmplByCategory("x", "nope"); err == nil {
		t.Error("byCategory wants []models.Page")
	}
	out, err = g.tmplByAuthor("ada", pages)
	assertHelperResult(t, out, err, "P1,P3", "")
	out, err = g.tmplByAuthor("2", pages)
	assertHelperResult(t, out, err, "P2", "")
}

func TestRelated(t *testing.T) {
	pages := contentPages()
	// P2 shares a tag (3) + a category (2) = 5 > P3 same author (1).
	rel, err := tmplRelated(pages[0], 5, pages)
	assertHelperResult(t, rel, err, "P2,P3", "")
	one, err := tmplRelated(pages[0], 1, pages)
	assertHelperResult(t, one, err, "P2", "")
	if _, err := tmplRelated(pages[0], -1, pages); err == nil {
		t.Error("related negative count should error")
	}
	if _, err := tmplRelated(pages[0], 3, "nope"); err == nil {
		t.Error("related wants []models.Page")
	}
}
