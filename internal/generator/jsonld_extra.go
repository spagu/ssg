package generator

// Structured data for pages ssg generates rather than renders from a file
// (1.8.72): the front page's Organization, an author's ProfilePage, a
// listing's ItemList. A template receives the blocks as .StructuredData — a
// theme that writes its own JSON-LD can include them with {{ toJSON }} — and,
// with seo: on, ssg injects them where the theme wrote no JSON-LD of its
// own, by the same rule as every other page (#158).

import (
	"reflect"
	"strings"
)

// structuredDataKey names the blocks in a template context.
const structuredDataKey = "StructuredData"

// structuredDataOf reads the blocks from a template context: a map or a
// struct with a StructuredData field. nil for anything else.
func structuredDataOf(data interface{}) []map[string]interface{} {
	if m, ok := data.(map[string]interface{}); ok {
		blocks, _ := m[structuredDataKey].([]map[string]interface{})
		return blocks
	}
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	f := v.FieldByName(structuredDataKey)
	if !f.IsValid() {
		return nil
	}
	blocks, _ := f.Interface().([]map[string]interface{})
	return blocks
}

// injectStructuredData adds the blocks before </head> when seo is on and the
// theme emitted no JSON-LD itself (themeLD, measured before ssg's own SEO pass
// added any).
func (g *Generator) injectStructuredData(html string, blocks []map[string]interface{}, themeLD bool) string {
	if !g.config.SEO || themeLD || len(blocks) == 0 {
		return html
	}
	var b strings.Builder
	for _, ld := range blocks {
		writeLDScript(&b, ld)
	}
	if i := strings.Index(strings.ToLower(html), "</head>"); i >= 0 {
		return html[:i] + b.String() + html[i:]
	}
	return html
}

// personLD is a schema.org Person: name, and whatever else is known — url,
// description, image, sameAs (profile links). Shared by author pages and
// the résumé, so a person reads the same wherever ssg describes one.
func personLD(name, url, description, image string, sameAs []string) map[string]interface{} {
	p := map[string]interface{}{"@type": "Person", "name": name}
	setNonEmpty(p, "url", url)
	setNonEmpty(p, "description", description)
	setNonEmpty(p, "image", image)
	if links := uniqueStrings(sameAs); len(links) > 0 {
		list := make([]interface{}, len(links))
		for i, l := range links {
			list[i] = l
		}
		p["sameAs"] = list
	}
	return p
}

// profilePageLD wraps a Person as the main entity of its profile page.
func profilePageLD(url string, person map[string]interface{}) map[string]interface{} {
	ld := map[string]interface{}{"@context": "https://schema.org", "@type": "ProfilePage", "mainEntity": person}
	setNonEmpty(ld, "url", url)
	return ld
}

// itemListLD is a listing as an ItemList of URLs, in the order shown: the
// summary-page form search engines read for carousels.
func itemListLD(urls []string) map[string]interface{} {
	if len(urls) == 0 {
		return nil
	}
	items := make([]interface{}, len(urls))
	for i, u := range urls {
		items[i] = map[string]interface{}{"@type": "ListItem", "position": i + 1, "url": u}
	}
	return map[string]interface{}{"@context": "https://schema.org", "@type": "ItemList", "itemListElement": items}
}

// setNonEmpty sets a key only to a non-blank value.
func setNonEmpty(m map[string]interface{}, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		m[key] = v
	}
}
