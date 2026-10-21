package generator

// Two documents written to one address (#346).
//
// detectContentCollisions caught it, but ran only under i18n. Without i18n a
// page and an external-source record with the same link:, or two records, went
// to one index.html: one was written, the other silently gone, and link
// checking stayed green because the address existed. On the site that
// reported it, a content page lost its address to a category record, and six
// ports to six categories.
//
// Every build now stops on it and names both documents. Two cases already
// handled with a warning, where nothing is lost silently, keep their
// behaviour: a second page at the site root is skipped (#234), and an alias
// that meets a page is not written.

import (
	"fmt"

	"github.com/spagu/ssg/internal/models"
)

// detectPathCollisions fails when two pages, posts or records share an
// output path other than the root.
func (g *Generator) detectPathCollisions() error {
	seen := map[string]models.Page{}
	for _, list := range [][]models.Page{g.siteData.Pages, g.siteData.Posts} {
		for _, p := range list {
			path := p.GetOutputPath()
			if isRootOutputPath(path) {
				continue // the front page is chosen, with a warning, by #234
			}
			if prev, ok := seen[path]; ok {
				return fmt.Errorf("output collision at /%s/: %s and %s are both written there, and one would silently replace the other — set a distinct link: or slug: on one of them",
					path, documentLabel(prev), documentLabel(p))
			}
			seen[path] = p
		}
	}
	return nil
}

// documentLabel names a document in an error: its file, or for a record,
// which has none, its title.
func documentLabel(p models.Page) string {
	switch {
	case p.SourceFile != "":
		return p.SourceFile
	case p.FromRecord:
		return fmt.Sprintf("the external-source record %q", p.Title)
	}
	return fmt.Sprintf("%q", p.Title)
}
