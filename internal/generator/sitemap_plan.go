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
// entry should carry. A file whose entries carry no dates has none: Google
// uses lastmod only where it is consistently accurate, and the build time —
// new on every build of unchanged content — would teach it the site's dates
// cannot be trusted.
func (f sitemapFile) lastmod() time.Time {
	newest := time.Time{}
	for _, e := range f.entries {
		if e.lastmod.After(newest) {
			newest = e.lastmod
		}
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
	if len(f.entries) == 0 {
		return nil
	}
	chunks := chunkSitemap(f.entries, g.maxSitemapURLs(), sitemapByteLimit)
	if len(chunks) == 1 {
		return []sitemapFile{f}
	}
	stem := strings.TrimSuffix(f.path, ".xml")
	out := make([]sitemapFile, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, sitemapFile{path: fmt.Sprintf("%s-%d.xml", stem, len(out)+1), entries: c})
	}
	if !g.config.Quiet {
		fmt.Printf("   🗺️  %s exceeds %d URLs or %d MB and was split into %d files\n",
			f.path, g.maxSitemapURLs(), sitemapByteLimit>>20, len(out))
	}
	return out
}

// sitemapByteLimit is the protocol's other ceiling: 50 MB uncompressed per
// file. At ordinary URL lengths the 50,000-URL limit is reached first, but a
// site with long URLs and hreflang alternates can cross this one first (#339).
const sitemapByteLimit = 50 << 20

// sitemapIndexLimit is how many sitemaps one index may name (sitemaps.org).
// At 50,000 URLs each that is 2.5 billion URLs; the check is there so the
// limit is enforced rather than assumed.
const sitemapIndexLimit = 50000

// sitemapFrameBytes is what a file adds around its <url> blocks: the XML
// declaration and the <urlset> element with its namespaces.
const sitemapFrameBytes = 512

// chunkSitemap cuts entries into files of at most maxURLs entries and
// maxBytes rendered bytes each, in order.
func chunkSitemap(entries []sitemapEntry, maxURLs, maxBytes int) [][]sitemapEntry {
	var chunks [][]sitemapEntry
	start, size := 0, sitemapFrameBytes
	for i, e := range entries {
		var sb strings.Builder
		e.writeTo(&sb)
		n := sb.Len()
		if i > start && (i-start >= maxURLs || size+n > maxBytes) {
			chunks = append(chunks, entries[start:i])
			start, size = i, sitemapFrameBytes
		}
		size += n
	}
	return append(chunks, entries[start:])
}
