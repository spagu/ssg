package generator

// One <url> for the site root, however a document comes to claim it (#219).

import (
	"path/filepath"
	"strings"
	"testing"
)

// sitemapFor builds a site with the given front-page arrangement and returns
// its sitemap.
func sitemapFor(t *testing.T, frontMatter string) string {
	t.Helper()
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(contentDir, "metadata.json"), `{"categories":[],"media":[],"users":[]}`)
	mustWrite(t, filepath.Join(contentDir, "pages", "home.md"), frontMatter+"\nHello.\n")
	mustWrite(t, filepath.Join(contentDir, "pages", "about.md"),
		"---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout.\n")

	gen, err := New(Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir: filepath.Join(tmp, "output"), Quiet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return mustRead(t, filepath.Join(tmp, "output", "sitemap.xml"))
}

// TestAFrontPageIsListedOnce — the reported case. A page claiming the root with
// `link: "/"` appeared twice, and the two entries disagreed about the same URL:
// priority 1.0 from the front-page entry, 0.8 from the page's own.
func TestAFrontPageIsListedOnce(t *testing.T) {
	sitemap := sitemapFor(t, `---
title: Home
slug: home
status: publish
type: page
link: "/"
---`)

	if n := strings.Count(sitemap, "<loc>https://example.com/</loc>"); n != 1 {
		t.Errorf("the site root appears %d times, want 1:\n%s", n, sitemap)
	}
	// The one kept is the front-page entry, which is the one that says this is
	// the most important URL on the site.
	root := sitemap[strings.Index(sitemap, "<loc>https://example.com/</loc>"):]
	if !strings.Contains(root[:200], "<priority>1.0</priority>") {
		t.Errorf("the surviving entry must be the priority-1.0 one:\n%s", root[:200])
	}
	// Everything else is still listed.
	if !strings.Contains(sitemap, "https://example.com/about/") {
		t.Error("other pages must still be in the sitemap")
	}
}

// TestASlugIndexFrontPageIsAlsoListedOnce: the older way of claiming the root.
// The guard used to key on the slug, which is why `link: "/"` slipped past it;
// keying on the address covers both without knowing how the page got there.
func TestASlugIndexFrontPageIsAlsoListedOnce(t *testing.T) {
	sitemap := sitemapFor(t, `---
title: Home
slug: index
status: publish
type: page
---`)
	if n := strings.Count(sitemap, "<loc>https://example.com/</loc>"); n != 1 {
		t.Errorf("the site root appears %d times, want 1:\n%s", n, sitemap)
	}
}

// TestAnOrdinarySiteStillHasItsRoot: a site whose front page is generated
// rather than claimed by a document must keep the entry.
func TestAnOrdinarySiteStillHasItsRoot(t *testing.T) {
	sitemap := sitemapFor(t, `---
title: Home
slug: home
status: publish
type: page
---`)
	if n := strings.Count(sitemap, "<loc>https://example.com/</loc>"); n != 1 {
		t.Errorf("the site root appears %d times, want 1:\n%s", n, sitemap)
	}
	if !strings.Contains(sitemap, "https://example.com/home/") {
		t.Error("a page that does not claim the root keeps its own entry")
	}
}

// TestTheWorkaroundIsNoLongerNeeded: `sitemap: "no"` on the front page removed
// the duplicate, at the cost of reading like the home page was being excluded.
// With the duplicate gone, the flag must still mean what it says — suppress
// the document's entry — and the front-page entry stands on its own.
func TestTheWorkaroundIsNoLongerNeeded(t *testing.T) {
	sitemap := sitemapFor(t, `---
title: Home
slug: home
status: publish
type: page
link: "/"
sitemap: "no"
---`)
	if n := strings.Count(sitemap, "<loc>https://example.com/</loc>"); n != 1 {
		t.Errorf("the site root appears %d times, want 1:\n%s", n, sitemap)
	}
}
