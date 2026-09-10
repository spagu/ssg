package externalsource

// Records into pages (GO-098).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// productMap is the mapping the documentation shows.
func productMap() ContentMap {
	return ContentMap{
		Title:      "name",
		Slug:       "product-{{.sku}}",
		Content:    "description",
		Date:       "updated_at",
		Type:       "=product",
		Taxonomies: map[string]string{"tag": "categories"},
	}
}

func mappedSource(m ContentMap) Source {
	return Source{Name: "products", Type: "file", Mode: "content", ContentMap: m,
		ContentFormat: FormatMarkdown, ContentErrors: "warn"}
}

// TestMapRecordsBuildsPages: the fields a page needs, from the fields a record
// has, through constants, templates and plain paths.
func TestMapRecordsBuildsPages(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"sku": "W-1", "name": "Blue Widget", "description": "A **blue** widget.",
			"updated_at": "2026-01-15", "categories": []interface{}{"Widgets", "Blue"}},
		map[string]interface{}{"sku": "W-2", "name": "Red Widget", "description": "Red.",
			"updated_at": "2026-02-20", "categories": "Widgets"},
	}
	imp, warnings, err := MapRecords(mappedSource(productMap()), records)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(imp.Posts) != 2 || len(imp.Pages) != 0 {
		t.Fatalf("pages=%d posts=%d", len(imp.Pages), len(imp.Posts))
	}
	p := imp.Posts[0]
	if p.Title != "Blue Widget" || p.Slug != "product-w-1" || p.Type != "product" || p.Status != "publish" {
		t.Errorf("page = %+v", p)
	}
	if p.Content != "A **blue** widget." {
		t.Errorf("content = %q", p.Content)
	}
	if !p.Date.Equal(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("date = %v", p.Date)
	}
	if len(p.Tags) != 2 || p.Tags[0] != "Widgets" {
		t.Errorf("tags = %v", p.Tags)
	}
	// A single value where a list was expected is one term, and a CSV column
	// holding "a,b" is two.
	if len(imp.Posts[1].Tags) != 1 || imp.Posts[1].Tags[0] != "Widgets" {
		t.Errorf("second record's tags = %v", imp.Posts[1].Tags)
	}
	if got := imp.Taxonomies["tag"]; len(got) != 2 || got[0] != "Blue" || got[1] != "Widgets" {
		t.Errorf("site terms = %v (want them sorted and deduplicated)", got)
	}
	if imp.Posts[0].ID == imp.Posts[1].ID || imp.Posts[0].ID == 0 {
		t.Error("mapped pages need distinct, non-zero ids")
	}
}

// TestMapRecordsRoutesPagesAndPosts: `type: page` renders on the page pipeline,
// anything else on the post pipeline, the same rule the CMS import uses.
func TestMapRecordsRoutesPagesAndPosts(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"name": "About", "kind": "page"},
		map[string]interface{}{"name": "News", "kind": "post"},
	}
	imp, _, err := MapRecords(mappedSource(ContentMap{Title: "name", Type: "kind"}), records)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Pages) != 1 || len(imp.Posts) != 1 {
		t.Fatalf("pages=%d posts=%d", len(imp.Pages), len(imp.Posts))
	}
	if imp.Pages[0].Slug != "about" {
		t.Errorf("a missing slug should come from the title, got %q", imp.Pages[0].Slug)
	}
}

// TestMapRecordsDottedPathsAndConstants: an API that nests its fields, and a
// constant that must not be mistaken for one.
func TestMapRecordsDottedPathsAndConstants(t *testing.T) {
	records := []interface{}{map[string]interface{}{
		"attributes": map[string]interface{}{"title": "Nested", "body": "Text"},
		"meta":       map[string]interface{}{"lang": "pl"},
	}}
	imp, warns, err := MapRecords(mappedSource(ContentMap{
		Title: "attributes.title", Content: "attributes.body", Lang: "meta.lang", Status: "=draft",
	}), records)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Errorf("warnings: %v", warns)
	}
	p := imp.Posts[0]
	if p.Title != "Nested" || p.Content != "Text" || p.Lang != "pl" || p.Status != "draft" {
		t.Errorf("page = %+v", p)
	}
}

// TestMapRecordsReportsAMissingField, and points at the constant syntax —
// which is the mistake this error exists to catch.
func TestMapRecordsReportsAMissingField(t *testing.T) {
	records := []interface{}{map[string]interface{}{"name": "One"}}
	src := mappedSource(ContentMap{Title: "name", Type: "product"}) // no "=" — a field, not a constant
	imp, warns, err := MapRecords(src, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], `"=product"`) {
		t.Errorf("warnings = %v", warns)
	}
	if imp.Posts[0].Type != "post" {
		t.Errorf("an unresolved type falls back to post, got %q", imp.Posts[0].Type)
	}

	strict := src
	strict.ContentErrors = "strict"
	if _, _, err := MapRecords(strict, records); err == nil {
		t.Error("content_errors: strict must fail the build on the same problem")
	}
}

// TestMapRecordsSkipsWhatCannotBeAPage: no title is no page, reported and
// skipped under warn, fatal under strict.
func TestMapRecordsSkipsWhatCannotBeAPage(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"name": "Good"},
		map[string]interface{}{"name": ""},
		map[string]interface{}{"name": "!!!"}, // slugifies to nothing
	}
	src := mappedSource(ContentMap{Title: "name", Slug: "name"})
	imp, warns, err := MapRecords(src, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Posts) != 1 {
		t.Errorf("kept %d records, want 1", len(imp.Posts))
	}
	if len(warns) != 2 {
		t.Errorf("warnings = %v", warns)
	}
	strict := src
	strict.ContentErrors = "strict"
	if _, _, err := MapRecords(strict, records); err == nil {
		t.Error("strict must fail on a record that cannot be a page")
	}
}

// TestMapRecordsRefusesDuplicateSlugs: two pages at one URL is a site that
// silently loses a page, so it stops the build whatever the policy.
func TestMapRecordsRefusesDuplicateSlugs(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"name": "Widget", "sku": "A"},
		map[string]interface{}{"name": "Widget", "sku": "B"},
	}
	_, _, err := MapRecords(mappedSource(ContentMap{Title: "name"}), records)
	if err == nil || !strings.Contains(err.Error(), "slug") {
		t.Errorf("got %v", err)
	}
	// The template form is the way out, and the error says so.
	if !strings.Contains(err.Error(), "{{") {
		t.Errorf("the error should suggest a template: %v", err)
	}
	imp, _, err := MapRecords(mappedSource(ContentMap{Title: "name", Slug: "{{.sku}}-{{.name}}"}), records)
	if err != nil {
		t.Fatal(err)
	}
	if imp.Posts[0].Slug != "a-widget" || imp.Posts[1].Slug != "b-widget" {
		t.Errorf("slugs = %q %q", imp.Posts[0].Slug, imp.Posts[1].Slug)
	}
}

// TestMapRecordsHonoursMaxRows: a runaway source stops rather than building a
// hundred thousand pages nobody asked for.
func TestMapRecordsHonoursMaxRows(t *testing.T) {
	records := make([]interface{}, 5)
	for i := range records {
		records[i] = map[string]interface{}{"name": string(rune('a' + i))}
	}
	src := mappedSource(ContentMap{Title: "name"})
	src.MaxRows = 3
	if _, _, err := MapRecords(src, records); err == nil || !strings.Contains(err.Error(), "max_rows") {
		t.Errorf("got %v", err)
	}
	src.MaxRows = 0 // the default applies
	if _, _, err := MapRecords(src, records); err != nil {
		t.Errorf("the default cap should allow five records: %v", err)
	}
}

// TestMapRecordsDates: the spellings an API actually uses.
func TestMapRecordsDates(t *testing.T) {
	for _, raw := range []string{"2026-01-15", "2026-01-15T10:30:00Z", "2026-01-15 10:30:00", "15/01/2026", "1768435200"} {
		imp, warns, err := MapRecords(mappedSource(ContentMap{Title: "name", Date: "when"}),
			[]interface{}{map[string]interface{}{"name": "X", "when": raw}})
		if err != nil {
			t.Fatal(err)
		}
		if len(warns) != 0 || imp.Posts[0].Date.IsZero() {
			t.Errorf("%q: date=%v warns=%v", raw, imp.Posts[0].Date, warns)
		}
	}
	imp, warns, err := MapRecords(mappedSource(ContentMap{Title: "name", Date: "when", Modified: "when"}),
		[]interface{}{map[string]interface{}{"name": "X", "when": "last tuesday"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !imp.Posts[0].Date.IsZero() {
		t.Errorf("an unreadable date should warn once and leave the field empty: %v", warns)
	}
}

// TestContentFormatText: prose from an API is not Markdown, and rendering it as
// Markdown would eat its punctuation.
func TestContentFormatText(t *testing.T) {
	body := "Save 50% on *everything* — sizes #1 to #3 [while stocks last]"
	src := mappedSource(ContentMap{Title: "name", Content: "body"})
	src.ContentFormat = FormatText
	imp, _, err := MapRecords(src, []interface{}{map[string]interface{}{"name": "Sale", "body": body}})
	if err != nil {
		t.Fatal(err)
	}
	got := imp.Posts[0].Content
	for _, raw := range []string{`\*everything\*`, `\#1`, `\[while stocks last\]`} {
		if !strings.Contains(got, raw) {
			t.Errorf("%q not escaped in %q", raw, got)
		}
	}
	// markdown and html leave the body exactly as it came.
	for _, format := range []string{FormatMarkdown, FormatHTML} {
		src.ContentFormat = format
		imp, _, err := MapRecords(src, []interface{}{map[string]interface{}{"name": "Sale", "body": body}})
		if err != nil {
			t.Fatal(err)
		}
		if imp.Posts[0].Content != body {
			t.Errorf("%s changed the body: %q", format, imp.Posts[0].Content)
		}
	}
}

// TestRecordsOfShapes: the parsers produce several shapes, and every one of
// them is a list of records or a clear error.
func TestRecordsOfShapes(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want int
	}{
		{"list of objects", []interface{}{map[string]interface{}{"a": 1}}, 1},
		{"csv rows", []map[string]string{{"a": "1"}, {"a": "2"}}, 2},
		{"typed list", []map[string]interface{}{{"a": 1}}, 1},
		{"one object", map[string]interface{}{"a": 1}, 1},
		{"map of objects", map[string]interface{}{"x": map[string]interface{}{"a": 1}, "y": map[string]interface{}{"a": 2}}, 2},
		{"nothing", nil, 0},
	}
	for _, c := range cases {
		got, err := recordsOf(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(got) != c.want {
			t.Errorf("%s: %d records, want %d", c.name, len(got), c.want)
		}
	}
	if _, err := recordsOf([]interface{}{"not an object"}); err == nil {
		t.Error("a list of scalars is not a list of records")
	}
	if _, err := recordsOf("a string"); err == nil {
		t.Error("a scalar source holds no records")
	}
}

// TestResolveMappingErrors: a broken template and a path through a scalar are
// each reported rather than silently producing an empty page.
func TestResolveMappingErrors(t *testing.T) {
	rec := map[string]interface{}{"a": "x", "n": 3, "b": true, "f": 1.5, "half": 0.5}
	if _, err := resolveMapping(rec, "{{ .a"); err == nil {
		t.Error("a malformed template must be reported")
	}
	if _, err := resolveMapping(rec, "{{ fail }}"); err == nil {
		t.Error("an unknown function must be reported")
	}
	if _, err := resolveMapping(rec, "a.deeper"); err == nil {
		t.Error("a path through a scalar must be reported")
	}
	// Scalars render the way a person writes them, not the way Go prints them.
	for spec, want := range map[string]string{"n": "3", "b": "true", "f": "1.5", "half": "0.5", "a": "x"} {
		got, err := resolveMapping(rec, spec)
		if err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", spec, got, err, want)
		}
	}
	if _, err := resolveMappingList(rec, "missing"); err == nil {
		t.Error("a missing taxonomy field must be reported")
	}
	if _, err := resolveMappingList(rec, "{{ .a"); err == nil {
		t.Error("a malformed taxonomy template must be reported")
	}
	if got, _ := resolveMappingList(rec, "=One, Two"); len(got) != 2 {
		t.Errorf("a constant term list = %v", got)
	}
	if got, _ := resolveMappingList(map[string]interface{}{"t": []string{"a", "b"}}, "t"); len(got) != 2 {
		t.Error("a []string field is a term list")
	}
	if got, _ := resolveMappingList(map[string]interface{}{"t": "{{}}"}, "{{ .t }}"); len(got) != 1 {
		t.Error("a template term list")
	}
}

// TestMapRecordsWarnsOnABadTaxonomyField without losing the page.
func TestMapRecordsWarnsOnABadTaxonomyField(t *testing.T) {
	imp, warns, err := MapRecords(mappedSource(ContentMap{Title: "name", Taxonomies: map[string]string{"tag": "missing"}}),
		[]interface{}{map[string]interface{}{"name": "One"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Posts) != 1 {
		t.Error("the page should survive a taxonomy it could not read")
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "taxonomies.tag") {
		t.Errorf("warnings = %v", warns)
	}
}

// TestCategoryMappingFillsTheLegacyFields, which is what the category archive
// and the CMS import path both still read.
func TestCategoryMappingFillsTheLegacyFields(t *testing.T) {
	imp, _, err := MapRecords(mappedSource(ContentMap{Title: "name", Taxonomies: map[string]string{"category": "cats"}}),
		[]interface{}{map[string]interface{}{"name": "One", "cats": []interface{}{"News", "Updates"}}})
	if err != nil {
		t.Fatal(err)
	}
	p := imp.Posts[0]
	if p.Category != "News" || len(p.CategoriesRaw) != 2 {
		t.Errorf("category=%q raw=%v", p.Category, p.CategoriesRaw)
	}
	if _, ok := p.TaxonomiesFM["category"].([]interface{}); !ok {
		t.Errorf("the generic map must hold the shape the resolver reads: %T", p.TaxonomiesFM["category"])
	}
}

// TestContentMapConfigValidation: the promise `mode: content` makes for a
// non-CMS source is now checked at load, where it can still be explained.
func TestContentMapConfigValidation(t *testing.T) {
	base := func(sc SourceConfig) (Source, error) {
		return resolveSource("products", sc, Defaults{}, defaultMaxSize)
	}
	file := SourceConfig{Type: "file", Path: "x.csv", Format: "csv"}

	withMode := file
	withMode.Mode = "content"
	if _, err := base(withMode); err == nil || !strings.Contains(err.Error(), "content_map") {
		t.Errorf("mode: content without a map must be refused: %v", err)
	}

	withMap := file
	withMap.ContentMap = ContentMap{Content: "body"} // no title
	withMap.Mode = "content"
	if _, err := base(withMap); err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("a map without a title must be refused: %v", err)
	}

	dataWithMap := file
	dataWithMap.ContentMap = ContentMap{Title: "name"}
	if _, err := base(dataWithMap); err == nil || !strings.Contains(err.Error(), "mode: content") {
		t.Errorf("a map without the mode must be refused: %v", err)
	}

	good := file
	good.Mode = "content"
	good.ContentMap = ContentMap{Title: "name"}
	src, err := base(good)
	if err != nil {
		t.Fatal(err)
	}
	if src.Mode != "content" || src.ContentFormat != FormatMarkdown || src.ContentErrors != "warn" {
		t.Errorf("defaults not applied: %+v", src)
	}

	for _, bad := range []SourceConfig{
		{Type: "file", Path: "x", Format: "csv", Mode: "sideways"},
		{Type: "file", Path: "x", Format: "csv", Mode: "content", ContentMap: ContentMap{Title: "n"}, ContentFormat: "pdf"},
		{Type: "file", Path: "x", Format: "csv", Mode: "content", ContentMap: ContentMap{Title: "n"}, ContentErrors: "shout"},
	} {
		if _, err := base(bad); err == nil {
			t.Errorf("%+v should be refused", bad)
		}
	}
}

// TestContentMapEndToEnd loads a CSV through the registry, which is where the
// mapping actually runs, and checks the import the generator will merge.
func TestContentMapEndToEnd(t *testing.T) {
	dir := t.TempDir()
	csv := filepath.Join(dir, "products.csv")
	body := "sku,name,description,updated_at,categories\n" +
		"W-1,Blue Widget,A blue widget.,2026-01-15,\"Widgets,Blue\"\n" +
		"W-2,Red Widget,A red widget.,2026-02-20,Widgets\n"
	if err := os.WriteFile(csv, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Enabled: true, Sources: map[string]SourceConfig{
		"products": {Type: "file", Path: csv, Format: "csv", Mode: "content", ContentMap: productMap()},
	}}
	reg, warnings, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings: %v", warnings)
	}
	imports := reg.CMSImports()
	if len(imports) != 1 || len(imports[0].Posts) != 2 {
		t.Fatalf("imports = %+v", imports)
	}
	if imports[0].Posts[0].Slug != "product-w-1" {
		t.Errorf("slug = %q", imports[0].Posts[0].Slug)
	}
	// The data namespace still holds the records: a mapped source is pages AND
	// data, so a template can still iterate it.
	if reg.Results["products"].Data == nil {
		t.Error("the parsed records should remain available to templates")
	}
	if got := reg.Results["products"].Metadata.RecordCount; got != 2 {
		t.Errorf("record count = %d", got)
	}
}

// TestContentMapEndToEndReportsABadMapping: a mapping that cannot produce
// pages fails the source, and a required source failing fails the build.
func TestContentMapEndToEndReportsABadMapping(t *testing.T) {
	dir := t.TempDir()
	csv := filepath.Join(dir, "products.csv")
	if err := os.WriteFile(csv, []byte("name\nWidget\nWidget\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Enabled: true, Sources: map[string]SourceConfig{
		"products": {Type: "file", Path: csv, Format: "csv", Mode: "content", ContentMap: ContentMap{Title: "name"}},
	}}
	if _, _, err := Load(cfg); err == nil || !strings.Contains(err.Error(), "slug") {
		t.Errorf("duplicate slugs must fail the build: %v", err)
	}
}

// TestContentMapWarningsReachTheBuild: a per-record problem under the warn
// policy is reported, not swallowed.
func TestContentMapWarningsReachTheBuild(t *testing.T) {
	dir := t.TempDir()
	csv := filepath.Join(dir, "p.csv")
	if err := os.WriteFile(csv, []byte("name,when\nWidget,not-a-date\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Enabled: true, Sources: map[string]SourceConfig{
		"p": {Type: "file", Path: csv, Format: "csv", Mode: "content",
			ContentMap: ContentMap{Title: "name", Date: "when"}},
	}}
	_, warnings, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "content_map.date") {
		t.Errorf("warnings = %v", warnings)
	}
}

// TestSlugifyIsShared: the mapper and the generator must agree about what a
// label turns into, because the answer is a URL.
func TestRecordIDIsStableAndDistinct(t *testing.T) {
	if recordID("products", 0) == recordID("articles", 0) {
		t.Error("two sources must not collide")
	}
	if recordID("products", 0) == recordID("products", 1) {
		t.Error("two records must not collide")
	}
	first := recordID("products", 4)
	if again := recordID("products", 4); first != again {
		t.Errorf("ids must be stable between builds: %d then %d", first, again)
	}
	if recordID("", 0) <= 0 {
		t.Error("an id must be positive")
	}
}

// TestScalarStringSpellings: a record value reaches a page the way a person
// would write it, not the way Go prints it.
func TestScalarStringSpellings(t *testing.T) {
	when := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		in   interface{}
		want string
	}{
		"nil":    {nil, ""},
		"string": {"x", "x"},
		"bool":   {true, "true"},
		"whole":  {float64(3), "3"},
		"frac":   {1.5, "1.5"},
		"int":    {7, "7"},
		"int64":  {int64(8), "8"},
		"time":   {when, "2026-01-15T10:00:00Z"},
		"other":  {[]int{1}, "[1]"},
	}
	for name, c := range cases {
		if got := scalarString(c.in); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

// TestToRecordShapes: a decoded object arrives as one of two map types, and
// anything else is not a record.
func TestToRecordShapes(t *testing.T) {
	if rec, ok := toRecord(map[string]string{"a": "1"}); !ok || rec["a"] != "1" {
		t.Errorf("string map: %v %v", rec, ok)
	}
	if rec, ok := toRecord(map[string]interface{}{"a": 1}); !ok || rec["a"] != 1 {
		t.Errorf("any map: %v %v", rec, ok)
	}
	if _, ok := toRecord("scalar"); ok {
		t.Error("a scalar is not a record")
	}
	// A map of scalars is one record, not a list of them.
	got, err := recordsOf(map[string]interface{}{"a": 1, "b": 2})
	if err != nil || len(got) != 1 {
		t.Errorf("recordsOf = %v, %v", got, err)
	}
}

// TestCollectTermsIgnoresWhatItCannotRead: a taxonomy value of an unexpected
// shape is skipped rather than crashing the build.
func TestCollectTermsIgnoresWhatItCannotRead(t *testing.T) {
	into := map[string]map[string]bool{}
	page := &models.Page{TaxonomiesFM: map[string]interface{}{
		"tag":  []interface{}{"a", 3},
		"bad":  "not a list",
		"more": []interface{}{"b"},
	}}
	collectTerms(into, page)
	if len(into["tag"]) != 1 || !into["tag"]["a"] {
		t.Errorf("tag terms = %v", into["tag"])
	}
	if _, ok := into["bad"]; ok {
		t.Error("a value that is not a list should be skipped")
	}
	if len(sortedTerms(into)) != 2 {
		t.Errorf("sortedTerms = %v", sortedTerms(into))
	}
}
