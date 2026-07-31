package generator

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

var ldBlockRe = regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`)

// parseLD extracts every JSON-LD object emitted for a page.
func parseLD(t *testing.T, html string) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	for _, m := range ldBlockRe.FindAllStringSubmatch(html, -1) {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(m[1]), &obj); err != nil {
			t.Fatalf("invalid JSON-LD %q: %v", m[1], err)
		}
		out = append(out, obj)
	}
	return out
}

// ldOfType returns the first parsed object with the given @type.
func ldOfType(blocks []map[string]interface{}, typ string) map[string]interface{} {
	for _, b := range blocks {
		if b["@type"] == typ {
			return b
		}
	}
	return nil
}

// TestBuildJSONLDPost covers #61: a post derives a valid BlogPosting entity from
// frontmatter, plus a BreadcrumbList from its URL.
func TestBuildJSONLDPost(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Authors = map[int]models.Author{7: {Name: "Ada Lovelace", Slug: "ada"}}
	page := models.Page{
		Title:         "Hello World",
		Slug:          "hello-world",
		Type:          "post",
		URLFormat:     "slug",
		Author:        7,
		Description:   "A first post",
		FeaturedImage: "/media/hero.jpg",
		Tags:          []string{"go", "ssg"},
		Date:          time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Modified:      time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
	}
	blocks := parseLD(t, g.buildJSONLD(page, true))

	main := ldOfType(blocks, "BlogPosting")
	if main == nil {
		t.Fatalf("expected a BlogPosting entity, got %v", blocks)
	}
	checks := map[string]interface{}{
		"headline":      "Hello World",
		"description":   "A first post",
		"datePublished": "2026-01-02T00:00:00Z",
		"dateModified":  "2026-01-03T00:00:00Z",
		"keywords":      "go, ssg",
		"url":           "https://example.com/hello-world/",
		"image":         "/media/hero.jpg",
	}
	for k, want := range checks {
		if main[k] != want {
			t.Errorf("BlogPosting[%q] = %v, want %v", k, main[k], want)
		}
	}
	author, _ := main["author"].(map[string]interface{})
	if author["@type"] != "Person" || author["name"] != "Ada Lovelace" {
		t.Errorf("author = %v, want Person Ada Lovelace", main["author"])
	}
	if moe, _ := main["mainEntityOfPage"].(map[string]interface{}); moe["@id"] != "https://example.com/hello-world/" {
		t.Errorf("mainEntityOfPage = %v", main["mainEntityOfPage"])
	}

	bc := ldOfType(blocks, "BreadcrumbList")
	if bc == nil {
		t.Fatalf("expected a BreadcrumbList")
	}
	items, _ := bc["itemListElement"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("breadcrumb items = %d, want 2 (Home + page)", len(items))
	}
	last, _ := items[1].(map[string]interface{})
	if last["name"] != "Hello World" || last["item"] != "https://example.com/hello-world/" || last["position"].(float64) != 2 {
		t.Errorf("last crumb = %v", last)
	}
}

// TestBuildJSONLDTypes covers the type mapping: home → WebSite, static page →
// WebPage, and that a page (non-post) omits article-only fields.
func TestBuildJSONLDTypes(t *testing.T) {
	g := newTestGen(t, "")

	home := models.Page{Title: "Home", Slug: "", Type: "page"}
	if got := ldOfType(parseLD(t, g.buildJSONLD(home, false)), "WebSite"); got == nil {
		t.Errorf("home page (URL /) must be WebSite")
	}
	// Home has no breadcrumb trail.
	if bc := ldOfType(parseLD(t, g.buildJSONLD(home, false)), "BreadcrumbList"); bc != nil {
		t.Errorf("home must not emit a BreadcrumbList")
	}

	page := models.Page{Title: "About", Slug: "about", Type: "page"}
	blocks := parseLD(t, g.buildJSONLD(page, false))
	wp := ldOfType(blocks, "WebPage")
	if wp == nil {
		t.Fatalf("static page must be WebPage, got %v", blocks)
	}
	if _, ok := wp["datePublished"]; ok {
		t.Errorf("WebPage must not carry article dates: %v", wp)
	}
	if ldOfType(blocks, "BreadcrumbList") == nil {
		t.Errorf("nested page must emit a BreadcrumbList")
	}
}

// TestBuildJSONLDOverrides covers precedence: site-wide config schema supplies a
// publisher default, per-page schema overrides derived fields (#61).
func TestBuildJSONLDOverrides(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Schema = map[string]interface{}{
		"publisher": map[string]interface{}{"@type": "Organization", "name": "Acme"},
	}
	page := models.Page{
		Title: "P", Slug: "p", Type: "page",
		Schema: map[string]interface{}{"@type": "TechArticle", "name": "Overridden"},
	}
	main := parseLD(t, g.buildJSONLD(page, false))[0]
	if main["@type"] != "TechArticle" {
		t.Errorf("per-page schema must override @type, got %v", main["@type"])
	}
	if main["name"] != "Overridden" {
		t.Errorf("per-page schema must override name, got %v", main["name"])
	}
	pub, _ := main["publisher"].(map[string]interface{})
	if pub["name"] != "Acme" {
		t.Errorf("site-wide publisher default must survive, got %v", main["publisher"])
	}
	// The shared config default must not be mutated by the merge.
	if _, leaked := g.config.Schema["@type"]; leaked {
		t.Errorf("config.Schema was mutated across the build")
	}
}

// TestDeepMergeAndClone covers the merge/clone helpers directly: nested maps
// merge recursively (overlay wins on scalars), arrays replace wholesale, and
// cloneLD deep-copies nested maps and slices so the source is never mutated.
func TestDeepMergeAndClone(t *testing.T) {
	base := map[string]interface{}{
		"publisher": map[string]interface{}{"@type": "Organization", "name": "Acme", "url": "https://acme.test"},
		"sameAs":    []interface{}{"https://a.test"},
	}
	clone := cloneLD(base)
	overlay := map[string]interface{}{
		"publisher": map[string]interface{}{"name": "Beta"}, // nested merge: name wins, url survives
		"sameAs":    []interface{}{"https://b.test"},        // arrays replace, not append
	}
	merged := deepMergeLD(clone, overlay)

	pub, _ := merged["publisher"].(map[string]interface{})
	if pub["name"] != "Beta" || pub["url"] != "https://acme.test" || pub["@type"] != "Organization" {
		t.Errorf("nested merge wrong: %v", pub)
	}
	if arr, _ := merged["sameAs"].([]interface{}); len(arr) != 1 || arr[0] != "https://b.test" {
		t.Errorf("array must be replaced, got %v", merged["sameAs"])
	}
	// Source untouched by both clone and merge.
	src, _ := base["publisher"].(map[string]interface{})
	if src["name"] != "Acme" {
		t.Errorf("cloneLD/deepMergeLD mutated the source: %v", src)
	}
	if arr, _ := base["sameAs"].([]interface{}); arr[0] != "https://a.test" {
		t.Errorf("source slice mutated: %v", base["sameAs"])
	}
}

// TestAuthorDisplayName covers the resolution fallbacks: registered author,
// unregistered id → raw string, and no usable author → empty.
func TestAuthorDisplayName(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Authors = map[int]models.Author{1: {Name: "Grace Hopper"}}

	if got := g.authorDisplayName(models.Page{Author: 1}); got != "Grace Hopper" {
		t.Errorf("registered author = %q", got)
	}
	// Unregistered id but a raw string name present → the raw name.
	if got := g.authorDisplayName(models.Page{Author: 9, AuthorRaw: "Jane Doe"}); got != "Jane Doe" {
		t.Errorf("unregistered id fallback = %q, want Jane Doe", got)
	}
	// Numeric AuthorRaw (not a string) and no registered author → empty.
	if got := g.authorDisplayName(models.Page{AuthorRaw: 5}); got != "" {
		t.Errorf("non-string author must yield empty, got %q", got)
	}
}

// TestBuildJSONLDEscapesScript covers XSS: a </script> in the title cannot break
// out of the JSON-LD block (encoding/json HTML-escapes it).
func TestBuildJSONLDEscapesScript(t *testing.T) {
	g := newTestGen(t, "")
	page := models.Page{Title: `x</script><script>alert(1)</script>`, Slug: "x", Type: "page"}
	out := g.buildJSONLD(page, false)
	if regexp.MustCompile(`<script>alert`).MatchString(out) {
		t.Errorf("script breakout not escaped: %s", out)
	}
	// It must still be valid, parseable JSON-LD.
	if len(parseLD(t, out)) == 0 {
		t.Errorf("no parseable JSON-LD emitted: %s", out)
	}
}
