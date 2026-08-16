package generator

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/spagu/ssg/internal/models"
)

// buildJSONLD renders the JSON-LD structured-data script(s) for a page (#61):
// a main entity typed by content kind — BlogPosting for posts, WebSite for the
// home page, WebPage otherwise — enriched from existing frontmatter, plus a
// BreadcrumbList derived from the URL. It gives AI agents and answer engines
// machine-readable data without executing JavaScript. Site-wide config schema:
// supplies defaults (e.g. a publisher Organization); per-page frontmatter
// schema: overrides everything. json.Marshal HTML-escapes </script>, so
// untrusted titles cannot break out of the script element.
func (g *Generator) buildJSONLD(page models.Page, isPost bool) string {
	merged := g.mergedSchema(page, isPost)

	var b strings.Builder
	writeLDScript(&b, merged)
	if bc := g.breadcrumbLD(page); bc != nil {
		writeLDScript(&b, bc)
	}
	return b.String()
}

// mergedSchema returns the structured data ssg would emit for a page, already
// resolved in precedence order: site-wide < derived < section defaults <
// per-page schema (#110). Section defaults outrank the derived data on purpose —
// overriding the derived @type is what they exist for — while a page's own
// schema still wins over its section.
//
// It is also what templates receive as `.Schema` (#158), so a theme that has to
// write a JSON-LD block of its own can emit this one beside it rather than
// reimplementing the merge and losing the section's type.
func (g *Generator) mergedSchema(page models.Page, isPost bool) map[string]interface{} {
	merged := deepMergeLD(cloneLD(g.config.Schema), g.derivedLD(page, isPost, g.servedCanonical(page)))
	merged = deepMergeLD(merged, g.sectionSchema(page))
	merged = deepMergeLD(merged, page.Schema)
	if _, ok := merged["@context"]; !ok {
		merged["@context"] = "https://schema.org"
	}
	return merged
}

// derivedLD builds the main entity from a page's frontmatter with zero extra
// configuration: sensible Schema.org defaults every content type can rely on.
func (g *Generator) derivedLD(page models.Page, isPost bool, canonical string) map[string]interface{} {
	ldType := "WebPage"
	switch {
	case isPost:
		ldType = "BlogPosting"
	case strings.Trim(page.GetURL(), "/") == "": // home page (empty path)
		ldType = "WebSite"
	}
	ld := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    ldType,
		"name":     page.Title,
		"url":      canonical,
	}
	if page.Description != "" {
		ld["description"] = page.Description
	}
	if page.Locale != "" {
		ld["inLanguage"] = page.Locale
	}
	if page.FeaturedImage != "" {
		ld["image"] = page.FeaturedImage
	}
	if !isPost {
		return ld
	}
	// Article-family enrichment: headline, dates, author, keywords, main entity.
	ld["headline"] = page.Title
	if !page.Date.IsZero() {
		ld["datePublished"] = page.Date.UTC().Format(time.RFC3339)
	}
	mod := page.Modified
	if mod.IsZero() {
		mod = page.Date
	}
	if !mod.IsZero() {
		ld["dateModified"] = mod.UTC().Format(time.RFC3339)
	}
	if name := g.authorDisplayName(page); name != "" {
		ld["author"] = map[string]interface{}{"@type": "Person", "name": name}
	}
	if kw := keywordsOf(page); kw != "" {
		ld["keywords"] = kw
	}
	ld["mainEntityOfPage"] = map[string]interface{}{"@type": "WebPage", "@id": canonical}
	return ld
}

// breadcrumbLD derives a BreadcrumbList from the page's URL path so agents can
// place the page in the site hierarchy. Home (and any page whose URL is "/") has
// no trail and returns nil; a bare domain also skips it (item URLs need a host).
func (g *Generator) breadcrumbLD(page models.Page) map[string]interface{} {
	if g.config.Domain == "" {
		return nil
	}
	rel := strings.Trim(page.GetURL(), "/")
	if rel == "" {
		return nil
	}
	segs := strings.Split(rel, "/")
	items := make([]interface{}, 0, len(segs)+1)
	items = append(items, crumb(1, "Home", "https://"+g.config.Domain+"/"))
	acc := ""
	for i, s := range segs {
		acc += "/" + s
		name := titleize(s)
		itemURL := "https://" + g.config.Domain + acc + "/"
		if i == len(segs)-1 { // the page itself: use its real title and canonical URL
			if page.Title != "" {
				name = page.Title
			}
			itemURL = g.servedCanonical(page)
		}
		items = append(items, crumb(i+2, name, itemURL))
	}
	return map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": items,
	}
}

// crumb is one BreadcrumbList ListItem.
func crumb(pos int, name, item string) map[string]interface{} {
	return map[string]interface{}{
		"@type": "ListItem", "position": pos, "name": name, "item": item,
	}
}

// authorDisplayName resolves a page's author to a display name: a registered
// author's Name, otherwise the raw frontmatter string (a plain "author: Jane").
func (g *Generator) authorDisplayName(page models.Page) string {
	if page.Author != 0 {
		if a, ok := g.siteData.Authors[page.Author]; ok && a.Name != "" {
			return a.Name
		}
	}
	if s, ok := page.AuthorRaw.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// keywordsOf renders a page's tags (preferred) or explicit keywords as a
// comma-separated schema.org keywords string.
func keywordsOf(page models.Page) string {
	if len(page.Tags) > 0 {
		return strings.Join(page.Tags, ", ")
	}
	return strings.TrimSpace(page.Keywords)
}

// titleize turns a URL slug ("getting-started") into a readable crumb label
// ("Getting Started").
func titleize(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "-", " "), "_", " ")
	words := strings.Fields(s)
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// writeLDScript marshals a JSON-LD object into a <script> block. Map keys are
// emitted sorted (encoding/json), so output is deterministic for golden tests.
func writeLDScript(b *strings.Builder, ld map[string]interface{}) {
	if j, err := json.Marshal(ld); err == nil {
		b.WriteString(`<script type="application/ld+json">` + string(j) + "</script>\n")
	}
}

// cloneLD deep-copies a JSON-LD object so merging a shared site-wide default map
// never mutates it across pages.
func cloneLD(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = cloneVal(v)
	}
	return out
}

func cloneVal(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		return cloneLD(t)
	case []interface{}:
		s := make([]interface{}, len(t))
		for i, e := range t {
			s[i] = cloneVal(e)
		}
		return s
	default:
		return v
	}
}

// deepMergeLD overlays one JSON-LD object on another: nested objects merge
// recursively, every other value (including arrays) is replaced. overlay wins.
func deepMergeLD(base, overlay map[string]interface{}) map[string]interface{} {
	if base == nil {
		base = map[string]interface{}{}
	}
	for k, ov := range overlay {
		if bv, ok := base[k]; ok {
			if bm, ok1 := bv.(map[string]interface{}); ok1 {
				if om, ok2 := ov.(map[string]interface{}); ok2 {
					base[k] = deepMergeLD(bm, om)
					continue
				}
			}
		}
		base[k] = ov
	}
	return base
}

// sectionSchema returns the schema_defaults entry that applies to a page (#110).
//
// Keys match the page's content-relative directory by prefix, longest match
// first — the same rule link_rewrites uses, so there is one prefix convention in
// the project rather than two. "home" is reserved for the site root, which is
// the only page that can carry a site-level @type without claiming it for every
// other page too.
func (g *Generator) sectionSchema(page models.Page) map[string]interface{} {
	if len(g.config.SchemaDefaults) == 0 {
		return nil
	}
	if strings.Trim(page.GetURL(), "/") == "" {
		return g.config.SchemaDefaults["home"]
	}
	section := g.contentSection(page)
	if section == "" {
		return nil
	}
	best, bestLen := map[string]interface{}(nil), -1
	for prefix, ld := range g.config.SchemaDefaults {
		if prefix == "home" {
			continue
		}
		p := strings.Trim(prefix, "/")
		if p != "" && (section == p || strings.HasPrefix(section, p+"/")) && len(p) > bestLen {
			best, bestLen = ld, len(p)
		}
	}
	return best
}

// contentSection returns a page's directory path relative to the content root,
// slash-separated, or "" when it cannot be placed.
func (g *Generator) contentSection(page models.Page) string {
	if page.SourceDir == "" {
		return ""
	}
	// Relative to the *source* directory, not the content root, so a key reads
	// "pages/recipes" rather than repeating the source name in every entry.
	root := filepath.Join(g.config.ContentDir, g.config.Source)
	rel, err := filepath.Rel(root, page.SourceDir)
	if err != nil || strings.HasPrefix(rel, "..") {
		// Content that does not sit under the source (content_sources) still has
		// a place — fall back to the content root.
		rel, err = filepath.Rel(g.config.ContentDir, page.SourceDir)
		if err != nil || strings.HasPrefix(rel, "..") {
			return ""
		}
	}
	return strings.Trim(filepath.ToSlash(rel), "/")
}
