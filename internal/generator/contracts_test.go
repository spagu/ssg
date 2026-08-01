package generator

import (
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

func postSchema() map[string]models.ContentSchema {
	return map[string]models.ContentSchema{
		"post": {
			Required: []string{"title", "date", "author"},
			Fields: map[string]models.FieldRule{
				"title":          {Type: "string"},
				"date":           {Type: "date"},
				"featured_image": {Type: "url"},
				"status":         {Type: "enum", Values: []string{"publish", "draft"}},
				"weight":         {Type: "int"},
			},
		},
	}
}

// TestCheckPageSchemaValid: a conforming post yields no violations.
func TestCheckPageSchemaValid(t *testing.T) {
	g := newTestGen(t, "")
	g.config.ContentSchemas = postSchema()
	p := models.Page{
		Type: "post", Title: "Hello", Slug: "hello", Author: 1,
		Date:          time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		FeaturedImage: "/media/hero.jpg", Status: "publish",
		Extra: map[string]interface{}{"weight": 5},
	}
	if v := g.checkPageSchema(p); len(v) != 0 {
		t.Errorf("valid post reported violations: %v", v)
	}
}

// TestCheckPageSchemaViolations: missing required + malformed fields are caught,
// each with file/field context.
func TestCheckPageSchemaViolations(t *testing.T) {
	g := newTestGen(t, "")
	g.config.ContentSchemas = postSchema()
	p := models.Page{
		Type: "post", Slug: "bad", SourceFile: "posts/bad.md",
		// title + date + author missing; bad enum, url, int
		Status:        "archived",                                // not in enum
		FeaturedImage: "not a url",                               // invalid url
		Extra:         map[string]interface{}{"weight": "heavy"}, // not int
	}
	v := g.checkPageSchema(p)
	joined := strings.Join(v, "\n")
	for _, want := range []string{
		`missing required field "title"`,
		`missing required field "date"`,
		`missing required field "author"`,
		`field "status" must be one of`,
		`field "featured_image" must be a url`,
		`field "weight" must be a int`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected violation %q in:\n%s", want, joined)
		}
	}
	if !strings.Contains(joined, "posts/bad.md") {
		t.Errorf("violations must carry the source file: %s", joined)
	}
}

// TestValidateContentSchemasStrict: strict turns violations into a build error;
// warn mode returns nil.
func TestValidateContentSchemasStrict(t *testing.T) {
	base := func() *Generator {
		g := newTestGen(t, "")
		g.config.ContentSchemas = postSchema()
		g.siteData.Posts = []models.Page{{Type: "post", Slug: "x", SourceFile: "x.md"}} // missing required
		// A page validated against the post schema exercises the Pages loop too.
		g.siteData.Pages = []models.Page{{Type: "post", Slug: "p", SourceFile: "p.md"}}
		return g
	}
	if err := base().validateContentSchemas(); err != nil {
		t.Errorf("warn mode must not fail the build: %v", err)
	}
	g := base()
	g.config.Strict = true
	if err := g.validateContentSchemas(); err == nil {
		t.Errorf("strict mode must fail the build on violations")
	}

	// No schemas configured → always a no-op.
	clean := newTestGen(t, "")
	clean.config.Strict = true
	clean.siteData.Posts = []models.Page{{Type: "post"}}
	if err := clean.validateContentSchemas(); err != nil {
		t.Errorf("no schemas configured must be a no-op: %v", err)
	}
}

// TestPageFieldValue covers the field accessor: known fields, Extra fallback,
// author int/raw, and presence flags.
func TestPageFieldValue(t *testing.T) {
	p := models.Page{
		Title: "T", Slug: "s", Status: "publish", Type: "post", Keywords: "k",
		FeaturedImage: "/i.jpg", Category: "News", Series: "Saga",
		Tags: []string{"go"}, Categories: []int{1}, AuthorRaw: "Jane",
		Date:  time.Now().UTC(),
		Extra: map[string]interface{}{"custom": "v"},
	}
	cases := []struct {
		field   string
		present bool
	}{
		{"title", true}, {"slug", true}, {"status", true}, {"type", true},
		{"keywords", true}, {"featured_image", true}, {"category", true},
		{"series", true}, {"date", true}, {"modified", false},
		{"tags", true}, {"categories", true}, {"author", true},
		{"description", false}, {"custom", true}, {"missing", false},
	}
	for _, c := range cases {
		if _, present := pageFieldValue(p, c.field); present != c.present {
			t.Errorf("pageFieldValue(%q) present = %v, want %v", c.field, present, c.present)
		}
	}
	if v, _ := pageFieldValue(p, "author"); v != "Jane" {
		t.Errorf("author fallback to AuthorRaw = %v, want Jane", v)
	}
}

// TestCheckFieldRuleTypes exercises each supported type positively and negatively.
func TestCheckFieldRuleTypes(t *testing.T) {
	ok := []struct {
		rule  models.FieldRule
		value interface{}
	}{
		{models.FieldRule{Type: "string"}, "x"},
		{models.FieldRule{Type: ""}, "default-is-string"},
		{models.FieldRule{Type: "int"}, 3},
		{models.FieldRule{Type: "int"}, int64(7)},
		{models.FieldRule{Type: "int"}, float64(4)},
		{models.FieldRule{Type: "int"}, "42"},
		{models.FieldRule{Type: "bool"}, true},
		{models.FieldRule{Type: "date"}, "2026-01-02"},
		{models.FieldRule{Type: "date"}, time.Now().UTC()},
		{models.FieldRule{Type: "url"}, "https://x.test/a"},
		{models.FieldRule{Type: "url"}, "/root-relative/"},
		{models.FieldRule{Type: "list"}, []string{"a"}},
		{models.FieldRule{Type: "enum", Values: []string{"a", "b"}}, "b"},
	}
	for _, c := range ok {
		if msg := checkFieldRule("f", c.value, c.rule); msg != "" {
			t.Errorf("%s value %v should pass, got: %s", c.rule.Type, c.value, msg)
		}
	}
	bad := []struct {
		rule  models.FieldRule
		value interface{}
	}{
		{models.FieldRule{Type: "int"}, "nope"},
		{models.FieldRule{Type: "int"}, 5.5},  // non-whole float is not an int
		{models.FieldRule{Type: "int"}, true}, // wrong kind entirely
		{models.FieldRule{Type: "string"}, 7}, // non-string
		{models.FieldRule{Type: "date"}, 5},   // non-string, non-time
		{models.FieldRule{Type: "bool"}, "true"},
		{models.FieldRule{Type: "date"}, "yesterday"},
		{models.FieldRule{Type: "url"}, "not a url"},
		{models.FieldRule{Type: "list"}, "scalar"},
		{models.FieldRule{Type: "enum", Values: []string{"a"}}, "z"},
		{models.FieldRule{Type: "mystery"}, "x"},
	}
	for _, c := range bad {
		if msg := checkFieldRule("f", c.value, c.rule); msg == "" {
			t.Errorf("%s value %v should fail", c.rule.Type, c.value)
		}
	}
}
