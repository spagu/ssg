package generator

// A document the generator COPIES cannot reach sitemap.xml (#255).
//
// generateSitemap writes the front page, the post listing (#244), pages, posts
// and the archives. A static_sources root is copied by the asset stage and
// never becomes a models.Page, so there is no branch it could be reached by —
// and neither is anything else a site publishes verbatim as HTML.
//
// schema-resume.org publishes a hand-authored CV editor at /editor/ that way.
// An Ahrefs crawl reported it under "indexable page not in sitemap" with 17
// internal inlinks: the most-linked page on the site after the home page, in the
// main nav of every page. This is the inverse of #244 — there the document was
// rendered and belonged to no collection; here it is a real document the
// generator never rendered at all — and it fails just as quietly: green build,
// clean check_links (the file is there), clean check_orphans (it is linked).
//
// The generator cannot tell a document from an asset by looking at a copied
// tree, so the site says which of the files it already declares is one:
//
//	static_sources:
//	  - { path: editor/index.html, dest: "editor/index.html", sitemap: true, priority: 0.8 }

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// staticSitemapDefaultPriority is what an ordinary page gets; a verbatim
// document is a page as far as a crawler is concerned.
const staticSitemapDefaultPriority = 0.8

// staticSitemapEntry is one copied document that asked to be listed.
type staticSitemapEntry struct {
	loc      string // absolute URL, host handling applied
	source   string // the file on disk, for <lastmod>
	priority float64
	rel      string // output-relative file, for the noindex check
}

// recordStaticSitemapEntry notes a copied file that `sitemap: true` asked for.
//
// dest is what copyStaticSources wrote: a file, or a directory whose index.html
// is the document. Anything else — a stylesheet, a directory with no index —
// cannot be a sitemap entry, and saying so is better than listing a URL that
// serves a file no crawler should index.
func (g *Generator) recordStaticSitemapEntry(src models.StaticSource, dest string) {
	if !src.Sitemap {
		return
	}
	file := dest
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		file = filepath.Join(dest, indexHTMLName)
	}
	if _, err := os.Stat(file); err != nil {
		fmt.Printf("   ⚠️  static_sources: %s asked for a sitemap entry but no HTML document was written there\n", src.Path)
		return
	}
	rel, err := filepath.Rel(g.config.OutputDir, file)
	if err != nil || !strings.EqualFold(filepath.Ext(file), ".html") {
		fmt.Printf("   ⚠️  static_sources: %s asked for a sitemap entry but is not an HTML document\n", src.Path)
		return
	}
	rel = filepath.ToSlash(rel)
	priority := src.Priority
	if priority <= 0 || priority > 1 {
		priority = staticSitemapDefaultPriority
	}
	// The source file, not the copy: `lastmod_from_git` reads the repository,
	// and the copy has no history of its own.
	source := src.Path
	switch info, err := os.Stat(src.Path); {
	case err != nil:
		// copyStaticSources skips a path it cannot stat, so this is unreachable
		// in a normal build — but leaving `source` pointing at nothing would
		// produce an entry with no date and no explanation (#260), which is the
		// shape of failure this whole entry exists to stop.
		fmt.Printf("   ⚠️  static_sources: %s is listed in the sitemap but cannot be read, so it carries no <lastmod>\n", src.Path)
		source = ""
	case info.IsDir():
		source = filepath.Join(src.Path, indexHTMLName)
	}
	g.staticSitemapMu.Lock()
	defer g.staticSitemapMu.Unlock()
	g.staticSitemap = append(g.staticSitemap, staticSitemapEntry{
		loc:      g.servedURL(httpsScheme + g.config.Domain + urlForOutputFile(rel)),
		source:   source,
		priority: priority,
		rel:      rel,
	})
}

// resetStaticSitemap clears the record at the start of a build, so a watch-mode
// rebuild lists what this build copied rather than accumulating.
func (g *Generator) resetStaticSitemap() {
	g.staticSitemapMu.Lock()
	defer g.staticSitemapMu.Unlock()
	g.staticSitemap = nil
}

// staticSourceEntries lists the verbatim documents that asked for it, in URL
// order so two builds of the same site produce the same file.
//
// They honour the rules every other entry does: a document already claimed by
// another section is not repeated, and one whose own HTML says noindex keeps
// itself out.
func (g *Generator) staticSourceEntries(claimed map[string]bool) []sitemapEntry {
	g.staticSitemapMu.Lock()
	entries := append([]staticSitemapEntry(nil), g.staticSitemap...)
	g.staticSitemapMu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].loc < entries[j].loc })

	out := make([]sitemapEntry, 0, len(entries))
	for _, e := range entries {
		if claimed[e.loc] || g.renderedExcludesItself(models.Page{}, e.rel) {
			continue
		}
		claimed[e.loc] = true
		out = append(out, sitemapEntry{
			loc:        e.loc,
			lastmod:    g.staticLastMod(e.source),
			changefreq: "monthly",
			priority:   fmt.Sprintf("%.1f", e.priority),
			kind:       kindStatic,
		})
	}
	return out
}

// staticLastMod dates a copied document (#260).
//
// 1.8.57 emitted <lastmod> only when `lastmod_from_git` was on AND git could
// answer, so a build where git says nothing produced an entry with no date at
// all — while the ordinary pages in the same sitemap kept theirs, because a
// page falls back to its frontmatter `modified`, then its `date`. A copied
// document has no frontmatter, so it looked like the git lookup was broken when
// what was missing was the fallback.
//
// The file's own modification time is the analogue of that frontmatter date: it
// is what the filesystem knows about the document, always available, and never
// worse than omitting the field.
func (g *Generator) staticLastMod(source string) time.Time {
	if source == "" {
		return time.Time{}
	}
	if g.config.LastmodFromGit {
		if t, ok := g.gitLastModForFile(source); ok {
			return t
		}
		g.warnGitLastModUnavailable(source)
	}
	if info, err := os.Stat(source); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

// warnGitLastModUnavailable says, once per build, that the setting the site
// asked for could not be honoured.
//
// The silence is what cost the reporter of #260 an afternoon: `lastmod_from_git`
// was on, the pages had dates, and there was no way to tell from the outside
// that git had answered nothing. The commonest cause is not a broken repository
// but an invisible git — a strictly confined snap sees only its own rootfs, and
// this snap bundles cwebp and avifenc for exactly that reason but not git.
func (g *Generator) warnGitLastModUnavailable(source string) {
	if g.config.Quiet {
		return
	}
	g.gitWarnOnce.Do(func() {
		fmt.Printf("   ⚠️  lastmod_from_git is on, but git could not date %s — using the file's modification time instead\n", source)
		fmt.Println("      git has to be on PATH; a strictly confined snap cannot see the host's copy.")
	})
}
