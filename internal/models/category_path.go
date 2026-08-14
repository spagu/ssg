package models

// Where a category archive lives (#138). WordPress nests categories and serves
// a child at /category/<parent>/<child>/; flattening it to /category/<child>/
// turns every surviving link — a menu copied from the old theme, a bookmark, a
// search result — into a 404 while the content sits one path away.
//
// The nesting is already in metadata.json (`parent`), so nothing has to be
// fetched: the information was being dropped at render time.

import "strings"

// CategoryPath returns the archive path for a category, relative to the
// /category/ root: "dzien-bociana" for a top-level term, "blog/dzien-bociana"
// for one nested under "blog".
//
// A parent that is not in the set (a partial export) is treated as absent
// rather than guessed at, and a parent chain that loops stops at the entry it
// revisits — a tangled taxonomy still renders every archive it has.
func CategoryPath(cat Category, all map[int]Category) string {
	segments := []string{SanitizeRelPath(cat.Slug)}
	seen := map[int]bool{cat.ID: true}

	for parentID := cat.Parent; parentID != 0; {
		parent, ok := all[parentID]
		if !ok || seen[parent.ID] {
			break
		}
		seen[parent.ID] = true
		if slug := SanitizeRelPath(parent.Slug); slug != "" {
			segments = append([]string{slug}, segments...)
		}
		parentID = parent.Parent
	}
	return strings.Trim(strings.Join(segments, "/"), "/")
}

// CategoryIsNested reports whether a category renders below another one, which
// is what makes its flat path worth a redirect.
func CategoryIsNested(cat Category, all map[int]Category) bool {
	return CategoryPath(cat, all) != SanitizeRelPath(cat.Slug)
}
