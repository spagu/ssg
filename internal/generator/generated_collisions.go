package generator

// A page and a file ssg generates at one address (SR-12 of the GO-183
// security review).
//
// A front-matter link: may name a file — /robots.txt, /sitemap.xml,
// /search-index.json — and sites use that on purpose: their own robots.txt
// with robots_off, a hand-written JSON document. But the pages are written
// first and every generated file after them, so when ssg also generated that
// file (or static/ held one), the page was replaced without a word: the
// build was green and the site served something its author never wrote. Like
// two documents at one address (#346), that now stops the build and names
// both sides.
//
// The check compares, rather than predicts: what each such page wrote is
// recorded once the pages are done, and compared after the last generated
// file. No list of generated names can fall behind a new generator, and a
// site whose page owns the name (robots_off: true, no search index) builds
// exactly as before. HTML pages are left out: the one generated .html file
// written after the pages, 404.html, already gives way to a page, and
// fingerprinting rewrites HTML on purpose.

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// claimedFile is what a page wrote at a file address.
type claimedFile struct {
	label string   // the page, as documentLabel names it
	sum   [32]byte // the file's content after the pages were written
}

// snapshotClaimedFiles records the non-HTML files the pages wrote through a
// link: that names a file. Runs right after the pages are rendered.
func (g *Generator) snapshotClaimedFiles() {
	g.claimedFiles = map[string]claimedFile{}
	for _, list := range [][]models.Page{g.siteData.Pages, g.siteData.Posts} {
		for _, p := range list {
			rel := p.GetOutputPath()
			ext := strings.ToLower(path.Ext(rel))
			if !models.HasPageExtension(rel) || ext == ".html" || ext == ".htm" {
				continue
			}
			sum, ok := fileSum(filepath.Join(g.config.OutputDir, filepath.FromSlash(rel)))
			if ok {
				g.claimedFiles[rel] = claimedFile{label: documentLabel(p), sum: sum}
			}
		}
	}
}

// checkClaimedFiles fails when something written after the pages replaced a
// file a page wrote. Runs after the last generated file.
func (g *Generator) checkClaimedFiles() error {
	rels := make([]string, 0, len(g.claimedFiles))
	for rel := range g.claimedFiles {
		rels = append(rels, rel)
	}
	sort.Strings(rels) // the same build reports the same collision first
	for _, rel := range rels {
		c := g.claimedFiles[rel]
		sum, ok := fileSum(filepath.Join(g.config.OutputDir, filepath.FromSlash(rel)))
		if !ok || sum == c.sum {
			continue
		}
		return fmt.Errorf("output collision at /%s: %s and %s are both written there, and the page was silently replaced — "+
			"set a different link: on the page, or turn the generated file off if the page is meant to be it",
			rel, c.label, g.laterWriter(rel))
	}
	return nil
}

// laterWriter names what replaced the page at rel: a file of the static
// directory, or one this build generates (robots.txt, a sitemap, a feed, the
// search index, …).
func (g *Generator) laterWriter(rel string) string {
	if g.config.StaticDir != "" {
		static := filepath.Join(g.config.StaticDir, filepath.FromSlash(rel))
		if _, err := os.Stat(static); err == nil {
			return filepath.ToSlash(static)
		}
	}
	return "the " + path.Base(rel) + " this build generates"
}

// fileSum hashes a file; false when it cannot be read.
func fileSum(name string) ([32]byte, bool) {
	data, err := os.ReadFile(name) // #nosec G304 -- a file this build wrote under its own output directory
	if err != nil {
		return [32]byte{}, false
	}
	return sha256.Sum256(data), true
}
