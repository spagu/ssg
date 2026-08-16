package generator

// One shape for every archive view (#145).
//
// category.html renders four different things — categories, tags, authors and
// series — and each used to arrive with its own anonymous struct. Two
// consequences, both bad:
//
//   - a field one kind carries and another does not (Pager, Series, Lang) is a
//     hard template error, and the generator loses the WHOLE archive over it,
//     leaving one warning per term in a long log. Six migrated sites lost every
//     category, tag and author archive that way.
//   - a theme cannot test whether a field exists, so "render pagination when
//     there is any" was not expressible.
//
// A map fixes both: Go templates resolve a missing key to nil rather than
// failing, so `{{if .Pager}}` works and an unknown field costs nothing. Every
// key below is present for every kind, so the difference between views is the
// VALUE, not the shape.

import "github.com/spagu/ssg/internal/models"

// archiveData builds the template context shared by every archive view.
//
// The keys are the ones the bundled themes and the documented contract already
// use; passing a map rather than a struct keeps every existing expression
// working (Go templates index maps and structs identically) while removing the
// class of failure above.
func (g *Generator) archiveData(kind, name string, cat models.Category,
	posts []models.Page, pager Pager, lang string) map[string]interface{} {
	return map[string]interface{}{
		"Site":     g.siteData,
		"Category": cat,
		"Kind":     kind,
		"Name":     name,
		// Series is the name under its old key, kept because series.html has
		// read it since the feature existed.
		"Series": name,
		"Posts":  posts,
		// Pager is present on every archive now, not only on the index and on
		// custom taxonomies — an archive whose template could not see its
		// pagination could only ever show its first page (#145).
		"Pager":        pager,
		"Lang":         lang,
		"Domain":       g.config.Domain,
		"Vars":         g.config.Variables,
		"Data":         g.data,
		"ExternalData": g.externalData,
	}
}

// singlePagePager describes an archive that was not paginated, so a template
// reading .Pager finds a truthful one-page pager instead of a zero value that
// renders as "page 0 of 0".
func singlePagePager(count int) Pager {
	p := Pager{Current: 1, Total: 1, PerPage: count}
	// One page, still a page: a numbered control renders "1" rather than
	// nothing, and .Pages is never nil for a template that ranges over it.
	return p.withPages(func(int) string { return "" })
}
