package generator

// A content page that renders its own listing can paginate it (#267): every
// generated listing could, and no page a person wrote.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// pagerTemplate renders what a paginating layout needs: the pager, the slice,
// and the canonical of THIS page of it. Guarded, because the same page.html
// renders ordinary pages where .Pager is nil.
func pagerTemplate(name string) string {
	if name == "page.html" {
		return `<html><head><title>{{ .Title }}</title><link rel="canonical" href="{{ .CanonicalURL }}"/></head><body>` +
			`{{ with .Pager }}<p id="pager">{{ .Current }}/{{ .Total }}|{{ .PrevURL }}|{{ .NextURL }}</p>{{ end }}` +
			`{{ range .Posts }}<li class="item">{{ .Slug }}</li>{{ end }}</body></html>`
	}
	return `<html><head><title>x</title></head><body><p>x</p></body></html>`
}

// buildPaginatedPageSite builds five posts and one page that pages over them
// two at a time, with the frontmatter the test wants.
func buildPaginatedPageSite(t *testing.T, paginate string, extraPages map[string]string) Config {
	t.Helper()
	files := map[string]string{
		"pages/blog.md": "---\ntitle: Blog\nslug: blog\nstatus: publish\ntype: page\n" + paginate + "\n---\n\nIntro.\n",
	}
	for i, n := range []string{"one", "two", "three", "four", "five"} {
		files["posts/news/"+n+".md"] = "---\ntitle: P\nslug: " + n + "\nstatus: publish\ntype: post\n" +
			"date: 2024-01-0" + string(rune('1'+i)) + "\n---\n\nBody.\n"
	}
	for k, v := range extraPages {
		files[k] = v
	}
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, pagerTemplate)
	buildSiteFixture(t, cfg)
	return cfg
}

func readOut(t *testing.T, cfg Config, rel string) string {
	t.Helper()
	return mustRead(t, filepath.Join(cfg.OutputDir, filepath.FromSlash(rel), indexHTMLName))
}

// TestPageCanPaginateItsOwnListing: the shorthand form writes the tail, gives
// every page its slice and pager, and the pager's links point at real files.
func TestPageCanPaginateItsOwnListing(t *testing.T) {
	cfg := buildPaginatedPageSite(t, "paginate: 2", nil)

	first := readOut(t, cfg, "blog")
	if !strings.Contains(first, `<p id="pager">1/3||/blog/page/2/</p>`) {
		t.Errorf("page 1 pager wrong:\n%s", first)
	}
	if n := strings.Count(first, `class="item"`); n != 2 {
		t.Errorf("page 1 lists %d items, want 2", n)
	}
	second := readOut(t, cfg, "blog/page/2")
	if !strings.Contains(second, `<p id="pager">2/3|/blog/|/blog/page/3/</p>`) {
		t.Errorf("page 2 pager wrong:\n%s", second)
	}
	third := readOut(t, cfg, "blog/page/3")
	if n := strings.Count(third, `class="item"`); n != 1 {
		t.Errorf("the last page lists %d items, want the remaining 1", n)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "blog", "page", "4", indexHTMLName)); err == nil {
		t.Error("a page past the end was written")
	}
	// Newest first, and no post appears twice across the set.
	seen := map[string]int{}
	for _, rel := range []string{"blog", "blog/page/2", "blog/page/3"} {
		for _, part := range strings.Split(readOut(t, cfg, rel), `class="item">`)[1:] {
			seen[part[:strings.Index(part, "<")]]++
		}
	}
	if len(seen) != 5 {
		t.Errorf("the pages list %d distinct posts, want 5: %v", len(seen), seen)
	}
	for slug, n := range seen {
		if n != 1 {
			t.Errorf("%s is listed %d times", slug, n)
		}
	}
	if !strings.Contains(first, `class="item">five<`) {
		t.Errorf("page 1 does not start with the newest post:\n%s", first)
	}
}

// TestPagedTailCanonicalisesToItself: page 2 names page 2, in the context AND
// in what the SEO pass derives from the page — the mistake #245 fixed for
// archives must not be reintroduced here.
func TestPagedTailCanonicalisesToItself(t *testing.T) {
	cfg := buildPaginatedPageSite(t, "paginate: 2", nil)
	cfg.SEO = true
	buildSiteFixture(t, cfg)

	if got := readOut(t, cfg, "blog"); !strings.Contains(got, `href="https://example.com/blog/"`) {
		t.Errorf("page 1 canonical wrong:\n%s", got)
	}
	second := readOut(t, cfg, "blog/page/2")
	if !strings.Contains(second, `href="https://example.com/blog/page/2/"`) {
		t.Errorf("page 2 canonical does not name page 2:\n%s", second)
	}
	if strings.Contains(second, `og:url" content="https://example.com/blog/"`) {
		t.Error("the SEO pass still describes page 1 on page 2")
	}
}

// TestPaginatedPageInSitemapOnce: the first page is the document; the tail is a
// slice of it and stays out, the rule every listing follows.
func TestPaginatedPageInSitemapOnce(t *testing.T) {
	cfg := buildPaginatedPageSite(t, "paginate: 2", nil)
	sitemap := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))

	if n := strings.Count(sitemap, "<loc>https://example.com/blog/</loc>"); n != 1 {
		t.Errorf("/blog/ listed %d times, want 1:\n%s", n, sitemap)
	}
	if strings.Contains(sitemap, "/blog/page/") {
		t.Errorf("the paginated tail reached the sitemap:\n%s", sitemap)
	}
}

// TestPaginateMappingFormNarrowsBySource: the long form names the collection
// and a content root, the way feeds: and sitemaps: do.
func TestPaginateMappingFormNarrowsBySource(t *testing.T) {
	cfg := buildPaginatedPageSite(t, "paginate:\n  over: posts\n  size: 10\n  source: docs", map[string]string{
		"posts/docs/guide.md": "---\ntitle: G\nslug: guide\nstatus: publish\ntype: post\ndate: 2024-02-01\n---\n\nBody.\n",
	})

	first := readOut(t, cfg, "blog")
	if n := strings.Count(first, `class="item"`); n != 1 || !strings.Contains(first, `>guide<`) {
		t.Errorf("source narrowing failed, got:\n%s", first)
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "blog", "page", "2", indexHTMLName)); err == nil {
		t.Error("one item under a size of ten should not produce a tail")
	}
}

// TestPaginateOverPagesExcludesItself: a page paging over pages does not list
// the page doing the paging.
func TestPaginateOverPagesExcludesItself(t *testing.T) {
	cfg := buildPaginatedPageSite(t, "paginate:\n  over: pages\n  size: 10", map[string]string{
		"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nA.\n",
	})

	first := readOut(t, cfg, "blog")
	if strings.Contains(first, `>blog<`) {
		t.Errorf("the paginating page lists itself:\n%s", first)
	}
	if !strings.Contains(first, `>about<`) {
		t.Errorf("the other page is missing:\n%s", first)
	}
}

// TestPaginateRefusesAFileURL: /blog.html has nowhere to put a /page/2/, so the
// page stays whole and the build says why instead of inventing a directory
// named "blog.html".
func TestPaginateRefusesAFileURL(t *testing.T) {
	g := newTestGen(t, "")
	page := models.Page{Slug: "blog", Link: "/blog.html", SourceFile: "blog.md", Paginate: &models.PaginateSpec{Size: 2}}

	var chunks []termChunk
	out := captureBuildOutput(t, func() { chunks = g.pageChunks(page) })

	if chunks != nil {
		t.Errorf("a file-URL page was paginated: %v", chunks)
	}
	if !strings.Contains(out, "directory-style URL") {
		t.Errorf("no explanation:\n%s", out)
	}
	// An ordinary page is simply not paginated, silently.
	if got := g.pageChunks(models.Page{Slug: "about"}); got != nil {
		t.Errorf("a page without paginate produced chunks: %v", got)
	}
}

// TestPaginateUnknownCollectionIsLoud: a typo in `over:` is an empty listing
// with a reason, not an empty listing.
func TestPaginateUnknownCollectionIsLoud(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Posts = []models.Page{{Slug: "a"}}
	page := models.Page{Slug: "blog", SourceFile: "blog.md"}

	var got []models.Page
	out := captureBuildOutput(t, func() {
		got = g.paginateCollection(page, models.PaginateSpec{Over: "articles"})
	})
	if len(got) != 0 || !strings.Contains(out, `"articles" is not a collection`) {
		t.Errorf("got %v, output:\n%s", got, out)
	}
}

// TestPageSizePrecedence: the page's own size, then the site's paginate, then
// a default that is not "one page" — a page that asked to paginate wants pages.
func TestPageSizePrecedence(t *testing.T) {
	g := newTestGen(t, "")
	if got := g.pageSize(models.PaginateSpec{Size: 3}); got != 3 {
		t.Errorf("own size ignored: %d", got)
	}
	g.config.Paginate = 7
	if got := g.pageSize(models.PaginateSpec{}); got != 7 {
		t.Errorf("site paginate ignored: %d", got)
	}
	g.config.Paginate = 0
	if got := g.pageSize(models.PaginateSpec{}); got != defaultPageSize {
		t.Errorf("default = %d, want %d", got, defaultPageSize)
	}
}

// TestPaginateCollectionFollowsThePageLanguage: under i18n a Polish page pages
// over Polish posts.
func TestPaginateCollectionFollowsThePageLanguage(t *testing.T) {
	g := newTestGen(t, "")
	g.config.I18n.Enabled = true
	g.siteData.Posts = []models.Page{{Slug: "a", Lang: "pl"}, {Slug: "b", Lang: "en"}, {Slug: "c", Lang: "pl"}}

	got := g.paginateCollection(models.Page{Slug: "blog", Lang: "pl"}, models.PaginateSpec{})
	if len(got) != 2 || got[0].Slug != "a" || got[1].Slug != "c" {
		t.Errorf("language filter wrong: %v", got)
	}
}

// TestPagedTailFallsBackToPageHTML: a layout the theme does not have falls back
// to page.html for the tail exactly as it does for page 1, so a paginating
// page never half-renders.
func TestPagedTailFallsBackToPageHTML(t *testing.T) {
	cfg := buildPaginatedPageSite(t, "layout: no-such-layout\npaginate: 2", nil)

	if got := readOut(t, cfg, "blog/page/2"); !strings.Contains(got, `<p id="pager">2/3|`) {
		t.Errorf("the tail did not fall back to page.html:\n%s", got)
	}
}

// TestPagedTailRefusesToEscapeTheOutput: the tail path is checked like every
// other write — a page whose URL would put /page/2/ outside the output tree
// is an error, not a file somewhere else.
func TestPagedTailRefusesToEscapeTheOutput(t *testing.T) {
	g := newTestGen(t, "")
	mustWrite(t, filepath.Join(g.config.OutputDir, "blog", "page"), "a file where a directory must go")
	page := models.Page{Slug: "blog", Title: "B", SourceFile: "blog.md"}
	chunks := paginateTerm([]models.Page{{Slug: "a"}, {Slug: "b"}, {Slug: "c"}}, 2, "/blog/")

	if err := g.renderPagedTail(page, chunks, pageHTMLName); err == nil {
		t.Error("a blocked tail directory must fail the build, not vanish")
	}
}

// TestFrontPageCanPaginate: a page at the site root writes /page/2/, not //page/2/.
func TestFrontPageCanPaginate(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Posts = []models.Page{{Slug: "a"}, {Slug: "b"}, {Slug: "c"}}
	page := models.Page{Slug: "index", Link: "/", Paginate: &models.PaginateSpec{Size: 2}}

	chunks := g.pageChunks(page)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d, want 2", len(chunks))
	}
	if got := chunks[1].Pager.Pages[1].URL; got != "/page/2/" {
		t.Errorf("root tail URL = %q, want /page/2/", got)
	}
	// And the spec's collection default is spelled out once, in one place.
	if (models.PaginateSpec{}).Collection() != "posts" || (models.PaginateSpec{Over: "pages"}).Collection() != "pages" {
		t.Error("Collection() default or passthrough is wrong")
	}
}
