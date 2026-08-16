package generator

// Date archives — /YYYY/, /YYYY/MM/, /YYYY/MM/DD/ (#146).
//
// WordPress publishes them, and links to them sit in every post's byline, in
// widgets and in the sitemap: on one migrated site 60 of 477 sitemap URLs were
// date archives. SSG built category, tag, author and series archives; dates
// were not among them, and the address did not even 404 — posts with dated
// permalinks live in /2014/05/…, so the directory existed and the dev server
// answered with its automatic index: 776 bytes of file names where the source
// served a 32 KB archive.
//
// They are OPT-IN (`date_archives`). Generating them by default would add
// pages, sitemap entries and output files to every existing project that dates
// its posts — a site that never had these URLs should not grow them because it
// upgraded. `ssg migrate` turns them on, because a migrated site's own content
// links to them.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// dateArchiveDepth is how far down a date archive is generated: year, month,
// day. Anything deeper is a permalink, not an archive.
type dateArchiveDepth int

const (
	depthYear dateArchiveDepth = iota + 1
	depthMonth
	depthDay
)

// dateArchive is one archive page: the path it lives at, the label a theme
// titles it with, and the posts it lists.
type dateArchive struct {
	Path  string // "2014", "2014/05", "2014/05/12"
	Label string // "2014", "May 2014", "12 May 2014"
	Depth dateArchiveDepth
	Posts []models.Page
}

// generateDateArchives writes one page per year, month and day that has posts.
// A no-op unless date_archives is enabled.
func (g *Generator) generateDateArchives() error {
	if !g.config.DateArchives || len(g.siteData.Posts) == 0 {
		return nil
	}
	archives := collectDateArchives(g.siteData.Posts, g.dateArchiveDepth())
	written := 0
	for _, a := range archives {
		ok, err := g.writeDateArchive(a)
		if err != nil {
			return err
		}
		if ok {
			written++
		}
	}
	if written > 0 && !g.config.Quiet {
		fmt.Printf("   📅 Generated %d date archive(s)\n", written)
	}
	return nil
}

// dateArchiveDepth decides how deep the archives go. A site whose posts are
// addressed by date serves the same depth its permalinks imply; a slug-based
// site still publishes years and months, which is what WordPress does when the
// permalink structure carries no date.
func (g *Generator) dateArchiveDepth() dateArchiveDepth {
	if strings.EqualFold(strings.TrimSpace(g.config.PostURLFormat), "slug") {
		return depthMonth
	}
	return depthDay
}

// collectDateArchives groups posts by year, month and day, newest first within
// each archive. A post with no date belongs to no archive: inventing one would
// publish a page nobody linked.
func collectDateArchives(posts []models.Page, depth dateArchiveDepth) []dateArchive {
	byPath := map[string]*dateArchive{}

	add := func(path, label string, d dateArchiveDepth, post models.Page) {
		a, ok := byPath[path]
		if !ok {
			a = &dateArchive{Path: path, Label: label, Depth: d}
			byPath[path] = a
		}
		a.Posts = append(a.Posts, post)
	}

	for _, post := range posts {
		if post.Date.IsZero() || !strings.EqualFold(post.Type, "post") {
			continue
		}
		y, m, d := post.Date.Date()
		year := fmt.Sprintf("%04d", y)
		add(year, year, depthYear, post)
		if depth >= depthMonth {
			month := fmt.Sprintf("%s/%02d", year, int(m))
			add(month, fmt.Sprintf("%s %s", m.String(), year), depthMonth, post)
			if depth >= depthDay {
				add(fmt.Sprintf("%s/%02d", month, d),
					fmt.Sprintf("%d %s %s", d, m.String(), year), depthDay, post)
			}
		}
	}

	out := make([]dateArchive, 0, len(byPath))
	for _, a := range byPath {
		a.Posts = sortPostsByDate(a.Posts)
		out = append(out, *a)
	}
	// Deterministic order so a build is reproducible and the log reads the same
	// way twice.
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// writeDateArchive renders one archive, unless real content already owns that
// URL — an author's own /2014/ page outranks a generated listing.
func (g *Generator) writeDateArchive(a dateArchive) (bool, error) {
	if owner, taken := g.archiveURLOwner("", a.Path); taken {
		if !g.config.Quiet {
			fmt.Printf("   ⚠️  Skipping date archive /%s/: %s already owns that URL\n", a.Path, owner)
		}
		return false, nil
	}
	outputPath := filepath.Join(g.config.OutputDir, filepath.FromSlash(a.Path), indexHTMLName)
	if err := g.ensureWithinOutput(outputPath); err != nil {
		return false, nil // a date cannot escape the output dir, but the guard stays
	}
	// #nosec G301 -- web content directories need to be world-traversable
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return false, err
	}

	data := g.archiveData("date", a.Label,
		models.Category{Name: a.Label, Slug: strings.ReplaceAll(a.Path, "/", "-")},
		a.Posts, singlePagePager(len(a.Posts)), g.currentLang)
	// The path lets a theme link between years and months without re-deriving
	// them from the label.
	data["DatePath"] = a.Path

	if err := g.renderTemplate(categoryHTMLName, outputPath, data); err != nil {
		fmt.Printf("   ⚠️  Warning: failed to generate date archive /%s/: %v\n", a.Path, err)
		return false, nil
	}
	return true, nil
}
