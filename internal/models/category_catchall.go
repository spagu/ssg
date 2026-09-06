package models

import "strings"

// The catch-all category — WordPress's "Uncategorized", "Bez kategorii" in a
// Polish export — is the term an exporter assigns to a post that was filed
// nowhere. It is not a real subject, so the sitemap and the per-category feeds
// leave it out.
//
// Recognising it by **id 1** was wrong twice over (#243): the id is whatever the
// source's metadata.json happens to number its first category, so a site whose
// id 1 is a populated, linked archive had that archive silently dropped from
// sitemap.xml and given no feed — while a catch-all numbered anything else was
// never recognised at all. The convention lives in WordPress's database, not in
// the exported data.
//
// The term names itself instead. One rule, in one place: the sitemap, the feeds
// and the template helpers all ask here, so they cannot drift apart again.
var catchAllCategoryKeys = map[string]bool{
	"uncategorized":       true, // WordPress, en_US
	"uncategorised":       true, // WordPress, en_GB
	"bez-kategorii":       true, // WordPress, pl_PL
	"nicht-kategorisiert": true, // WordPress, de_DE
	"sans-categorie":      true, // WordPress, fr_FR
	"sans-catégorie":      true, // …and the accented display name it ships with
	"sin-categoria":       true, // WordPress, es_ES
	"sin-categoría":       true,
	"senza-categoria":     true, // WordPress, it_IT
}

// IsCatchAllCategory reports whether cat is the exporter's "no category" term.
//
// Both the slug and the display name are tested, because an export that
// translated one may have left the other in English, and either spelling
// identifies the same term. An empty category is not a catch-all: it is nothing
// at all, and callers that resolve an unknown id already skip it.
func IsCatchAllCategory(cat Category) bool {
	return catchAllCategoryKey(cat.Slug) || catchAllCategoryKey(cat.Name)
}

// catchAllCategoryKey normalises one label to the slug shape the table uses, so
// "Bez kategorii" and "bez-kategorii" are the same key.
func catchAllCategoryKey(label string) bool {
	key := strings.ToLower(strings.TrimSpace(label))
	if key == "" {
		return false
	}
	key = strings.ReplaceAll(key, " ", "-")
	key = strings.ReplaceAll(key, "_", "-")
	return catchAllCategoryKeys[key]
}

// HasCategoriesOtherThanCatchAll reports whether p is filed under at least one
// real category, resolving each id against the site's category table.
//
// cats is the site's categories; a nil or partial map means the ids cannot be
// resolved, and an unresolvable id counts as real — the alternative is to hide
// a category the caller simply could not look up.
func (p Page) HasCategoriesOtherThanCatchAll(cats map[int]Category) bool {
	for _, catID := range p.Categories {
		cat, ok := cats[catID]
		if !ok || !IsCatchAllCategory(cat) {
			return true
		}
	}
	return false
}
