package generator

import (
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// The author archive is generated outside the taxonomy registry — it is keyed on
// an author id rather than a frontmatter field (#44) — so `termURL`, which
// resolves registered taxonomies, cannot name it. A byline wanting to link the
// archive its own build published had to hardcode /author/<slug>/ and guess the
// slug rule, and a theme that did not guess left the archive an orphan:
// published, listed in the sitemap, linked from nowhere (#245).
//
// tmplAuthorURL applies the generator's own slug rule instead, so the link and
// the written archive cannot disagree.

// tmplAuthorURL returns the address of an author's archive, or "" when the value
// names no author.
//
// It accepts what a template actually holds: the author id a post carries, the
// post itself, an Author record, or a display name.
func (g *Generator) tmplAuthorURL(author any) string {
	slug := g.authorSlugForAny(author)
	if slug == "" {
		return ""
	}
	return "/author/" + slug + "/"
}

// authorSlugForAny resolves any of the shapes above to the slug the archive was
// written at.
func (g *Generator) authorSlugForAny(author any) string {
	switch v := author.(type) {
	case int:
		return g.authorSlugForID(v)
	case int64:
		return g.authorSlugForID(int(v))
	case float64: // a number arriving from data/JSON rather than frontmatter
		return g.authorSlugForID(int(v))
	case models.Author:
		return authorSlug(v)
	case string:
		return g.authorSlugForName(v)
	}
	if p, ok := pageFromAny(author); ok {
		return g.authorSlugForID(p.Author)
	}
	return ""
}

// authorSlugForID resolves an id through the same helper generateAuthors uses,
// so a link and the archive it points at are derived from one rule. Id 0 means
// "no author" and gets no link, rather than the "author-0" placeholder the
// unresolved-id fallback would invent.
func (g *Generator) authorSlugForID(id int) string {
	if id == 0 {
		return ""
	}
	_, slug := g.authorNameSlug(id)
	return slug
}

// authorSlugForName matches a display name or slug against the site's authors,
// falling back to slugifying the name — which is what generateAuthors does for
// an author record carrying no slug of its own.
func (g *Generator) authorSlugForName(name string) string {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return ""
	}
	for _, a := range g.siteData.Authors {
		if strings.ToLower(a.Name) == want || strings.ToLower(a.Slug) == want {
			return authorSlug(a)
		}
	}
	return slugify(name)
}

// authorSlug is the slug an author record is published under: its own slug when
// it has one, its name otherwise.
func authorSlug(a models.Author) string {
	if a.Slug != "" {
		return slugify(a.Slug)
	}
	return slugify(a.Name)
}
