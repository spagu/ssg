package generator

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// validateContentSchemas enforces the per-type frontmatter contracts declared in
// content_schemas (#62): every post and page is checked against the schema for
// its type. Violations are reported with file, field and reason. Under `strict`
// (or --strict) the build fails; otherwise they are warnings so a site can adopt
// schemas incrementally. A no-op when no schemas are configured.
func (g *Generator) validateContentSchemas() error {
	if len(g.config.ContentSchemas) == 0 {
		return nil
	}
	var violations []string
	for _, p := range g.siteData.Posts {
		violations = append(violations, g.checkPageSchema(p)...)
	}
	for _, p := range g.siteData.Pages {
		violations = append(violations, g.checkPageSchema(p)...)
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	for _, v := range violations {
		fmt.Printf("   ⚠️  content schema: %s\n", v)
	}
	if g.config.Strict {
		return fmt.Errorf("content schema validation failed: %d violation(s); see ssg --help for content_schemas", len(violations))
	}
	return nil
}

// checkPageSchema validates one page against the schema for its content type,
// returning a violation string per problem (empty when the page conforms or has
// no schema). A page with no explicit type is validated as a "page".
func (g *Generator) checkPageSchema(p models.Page) []string {
	typ := p.Type
	if typ == "" {
		typ = "page"
	}
	schema, ok := g.config.ContentSchemas[typ]
	if !ok {
		return nil
	}
	where := p.SourceFile
	if where == "" {
		where = p.Slug
	}
	var out []string
	for _, field := range schema.Required {
		if _, present := pageFieldValue(p, field); !present {
			out = append(out, fmt.Sprintf("%s: missing required field %q (type %s)", where, field, typ))
		}
	}
	for _, field := range sortedKeys(schema.Fields) {
		value, present := pageFieldValue(p, field)
		if !present {
			continue // optional fields are only type-checked when supplied
		}
		if msg := checkFieldRule(field, value, schema.Fields[field]); msg != "" {
			out = append(out, where+": "+msg)
		}
	}
	return out
}

// pageFieldValue reads a frontmatter field by name, mapping the well-known model
// fields and falling back to Extra for custom keys. The second return reports
// presence (a required field is "missing" when this is false).
func pageFieldValue(p models.Page, name string) (interface{}, bool) {
	switch name {
	case "title":
		return p.Title, p.Title != ""
	case "description":
		return p.Description, p.Description != ""
	case "slug":
		return p.Slug, p.Slug != ""
	case "status":
		return p.Status, p.Status != ""
	case "type":
		return p.Type, p.Type != ""
	case "keywords":
		return p.Keywords, p.Keywords != ""
	case "featured_image":
		return p.FeaturedImage, p.FeaturedImage != ""
	case "category":
		return p.Category, p.Category != ""
	case "series":
		return p.Series, p.Series != ""
	case "date":
		return p.Date, !p.Date.IsZero()
	case "modified":
		return p.Modified, !p.Modified.IsZero()
	case "tags":
		return p.Tags, len(p.Tags) > 0
	case "categories":
		return p.Categories, len(p.Categories) > 0 || len(p.CategoriesRaw) > 0
	case "author":
		if p.Author != 0 {
			return p.Author, true
		}
		return p.AuthorRaw, p.AuthorRaw != nil
	default:
		v, ok := p.Extra[name]
		return v, ok
	}
}

// checkFieldRule validates one present field value against its declared rule,
// returning an empty string when it conforms or a human-readable reason when not.
func checkFieldRule(field string, value interface{}, rule models.FieldRule) string {
	switch rule.Type {
	case "", "string":
		if _, ok := value.(string); !ok {
			return typeMsg(field, "string", value)
		}
	case "int":
		if !isInt(value) {
			return typeMsg(field, "int", value)
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return typeMsg(field, "bool", value)
		}
	case "date":
		if !isDate(value) {
			return typeMsg(field, "date", value)
		}
	case "url":
		if s, ok := value.(string); !ok || !isURLValue(s) {
			return typeMsg(field, "url", value)
		}
	case "list":
		if !isList(value) {
			return typeMsg(field, "list", value)
		}
	case "enum":
		s, ok := value.(string)
		if !ok || !containsString(rule.Values, s) {
			return fmt.Sprintf("field %q must be one of %v, got %v", field, rule.Values, value)
		}
	default:
		return fmt.Sprintf("field %q declares unknown schema type %q", field, rule.Type)
	}
	return ""
}

func typeMsg(field, want string, value interface{}) string {
	return fmt.Sprintf("field %q must be a %s, got %T (%v)", field, want, value, value)
}

// isInt reports whether value is an integer, including whole floats from JSON and
// numeric strings from YAML scalars.
func isInt(value interface{}) bool {
	switch v := value.(type) {
	case int, int64:
		return true
	case float64:
		return v == float64(int64(v))
	case string:
		_, err := strconv.Atoi(strings.TrimSpace(v))
		return err == nil
	}
	return false
}

// isDate reports whether value is a time.Time or a string in one of the accepted
// frontmatter date formats.
func isDate(value interface{}) bool {
	if _, ok := value.(time.Time); ok {
		return true
	}
	s, ok := value.(string)
	if !ok {
		return false
	}
	for _, f := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02", "2006/01/02"} {
		if _, err := time.Parse(f, strings.TrimSpace(s)); err == nil {
			return true
		}
	}
	return false
}

// isURLValue accepts absolute URLs (with a scheme) and site-root-relative paths.
func isURLValue(s string) bool {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "/") {
		return true
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func isList(value interface{}) bool {
	switch value.(type) {
	case []string, []interface{}, []int:
		return true
	}
	return false
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
