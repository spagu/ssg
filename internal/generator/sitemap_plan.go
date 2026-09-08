package generator

// Deciding which sitemap files a build writes.
//
// Two problems, one mechanism.
//
// **Size.** sitemaps.org caps one file at 50,000 URLs (and 50 MB uncompressed).
// A build over that limit used to write it anyway: a file every crawler
// rejects, with nothing said — the failure class this project has spent several
// releases removing. A set that does not fit is now split, and sitemap.xml
// becomes the <sitemapindex> naming the parts.
//
// **Structure.** A site may want its sections separated whatever the size —
// most usefully because Search Console reports indexing coverage per submitted
// sitemap, so "how much of the blog is indexed" is a question only a separate
// file can answer. `sitemaps:` declares them, mirroring the `feeds:` shape
// (#86): each entry chooses what goes in and where it is written.
//
// Selection is a partition, not a set of views: an entry lands in the FIRST
// declared sitemap that matches it, and whatever matches none goes to the
// default file. So order matters, like a routing table, and no URL is ever
// listed twice or lost.

import (
	"fmt"
	"strings"
	"time"
)

// sitemapURLLimit is the protocol's ceiling on one file. Configurable because a
// site may want smaller files (a slow generator, a cautious crawler budget),
// never larger: above 50,000 the file is invalid.
const sitemapURLLimit = 50000

// defaultSitemapName is the file the index points at for everything unclaimed,
// and the single file a small site writes.
// A split names its parts after the file it came from: sitemap.xml becomes
// sitemap-1.xml, sitemap-2.xml, and a declared sitemap-blog.xml becomes
// sitemap-blog-1.xml and so on.
const defaultSitemapName = "sitemap.xml"

// sitemapFile is one written file: its output-relative path and its entries.
type sitemapFile struct {
	path    string
	entries []sitemapEntry
}

// lastmod is the newest date among a file's entries, which is what an index
// entry should carry. A file whose entries carry no dates at all falls back to
// the build time, so the index never omits the field.
func (f sitemapFile) lastmod(built time.Time) time.Time {
	newest := time.Time{}
	for _, e := range f.entries {
		if e.lastmod.After(newest) {
			newest = e.lastmod
		}
	}
	if newest.IsZero() {
		return built
	}
	return newest
}

// maxSitemapURLs is the per-file ceiling this build applies.
func (g *Generator) maxSitemapURLs() int {
	n := g.config.SitemapMaxURLs
	if n <= 0 || n > sitemapURLLimit {
		return sitemapURLLimit
	}
	return n
}

// planSitemaps decides the files to write.
//
// The common case is deliberately unchanged: no declared sitemaps and a set that
// fits produces exactly one sitemap.xml, byte for byte what earlier releases
// wrote. An index appears only when there is more than one file to name.
func (g *Generator) planSitemaps(entries []sitemapEntry) (files []sitemapFile, index bool) {
	declared, rest := g.partitionSitemaps(entries)
	for _, d := range declared {
		files = append(files, g.splitSitemap(d)...)
	}
	// The default file keeps the plain name when it is the only one, so a small
	// site is untouched; alongside declared sitemaps it becomes a named part so
	// sitemap.xml can be the index.
	name := defaultSitemapName
	if len(files) > 0 {
		name = "sitemap-main.xml"
	}
	files = append(files, g.splitSitemap(sitemapFile{path: name, entries: rest})...)
	return files, len(files) > 1
}

// partitionSitemaps routes each entry to the first declared sitemap that claims
// it, returning those files plus everything unclaimed.
func (g *Generator) partitionSitemaps(entries []sitemapEntry) ([]sitemapFile, []sitemapEntry) {
	specs := g.config.Sitemaps
	if len(specs) == 0 {
		return nil, entries
	}
	files := make([]sitemapFile, 0, len(specs))
	for _, spec := range specs {
		files = append(files, sitemapFile{path: sitemapSpecPath(spec)})
	}
	var rest []sitemapEntry
	for _, e := range entries {
		placed := false
		for i, spec := range specs {
			if sitemapSpecMatches(spec, e) {
				files[i].entries = append(files[i].entries, e)
				placed = true
				break
			}
		}
		if !placed {
			rest = append(rest, e)
		}
	}
	// A declared sitemap that selected nothing is not written: an empty urlset
	// in an index is a file crawlers fetch to learn nothing.
	kept := files[:0]
	for _, f := range files {
		if len(f.entries) > 0 {
			kept = append(kept, f)
			continue
		}
		if !g.config.Quiet {
			fmt.Printf("   ⚠️  sitemaps: %s matched no URLs and was not written\n", f.path)
		}
	}
	return kept, rest
}

// splitSitemap breaks one file into as many as the URL ceiling requires.
func (g *Generator) splitSitemap(f sitemapFile) []sitemapFile {
	max := g.maxSitemapURLs()
	if len(f.entries) <= max {
		if len(f.entries) == 0 {
			return nil
		}
		return []sitemapFile{f}
	}
	stem := strings.TrimSuffix(f.path, ".xml")
	var out []sitemapFile
	for i := 0; i < len(f.entries); i += max {
		end := i + max
		if end > len(f.entries) {
			end = len(f.entries)
		}
		out = append(out, sitemapFile{
			path:    fmt.Sprintf("%s-%d.xml", stem, len(out)+1),
			entries: f.entries[i:end],
		})
	}
	if !g.config.Quiet {
		fmt.Printf("   🗺️  %s exceeds %d URLs and was split into %d files\n", f.path, max, len(out))
	}
	return out
}
