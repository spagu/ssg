package models

// Where a category archive lives (#138). WordPress nests categories and serves
// a child at /category/<parent>/<child>/; flattening it to /category/<child>/
// turns every surviving link — a menu copied from the old theme, a bookmark, a
// search result — into a 404 while the content sits one path away.
//
// The nesting is already in metadata.json (`parent`), so nothing has to be
// fetched: the information was being dropped at render time.

import (
	"net/url"
	"strings"
)

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

// CategoryArchivePath returns the path a category archive is served at,
// relative to the site root and without slashes at either end
// ("category/blog/dzien-bociana", or "realizacje/adaptacje" for a site that
// dropped the /category/ base).
//
// The export states the address each term is actually served at, in `link`
// (#143). WordPress lets a site remove the category base, and many do, so
// deriving the path from a fixed "category/" prefix 404s every archive link in
// every post's meta line, in the menu and in the breadcrumbs — while the
// information was in hand all along.
//
// A term without a usable link keeps the built-in layout, which is what a
// hand-authored site has always had.
func CategoryArchivePath(cat Category, all map[int]Category, base string) string {
	if p := pathFromLink(cat.Link); p != "" {
		return p
	}
	return strings.Trim(base, "/") + "/" + CategoryPath(cat, all)
}

// pathFromLink extracts the path a term is served at from its absolute link,
// sanitised into output-safe segments. An unparseable or root link yields "",
// which sends the caller back to the built-in layout.
func pathFromLink(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	var segments []string
	for _, seg := range strings.Split(strings.Trim(u.Path, "/"), "/") {
		if clean := SanitizeRelPath(seg); clean != "" && clean != "." {
			segments = append(segments, clean)
		}
	}
	return strings.Join(segments, "/")
}
