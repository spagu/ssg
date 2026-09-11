package generator

// A content page that renders its own listing can paginate it (#267).
//
// Every generated listing paginated and no authored page could: `.Pager` was
// not in a page's context, and nothing wrote a second file, so a hand-written
// /blog/ that put an aggregated feed above the site's own posts could only page
// in the browser. The pieces were all here — `.Pager`, the /page/N/ URL shape,
// paginateTerm — and a page was simply not allowed to ask.
//
// Page 1 is the page itself, at every path page_format gives it, rendered with
// `.Pager` and `.Posts` added to its ordinary context. Pages 2..N are rendered
// through the same layout at /<url>/page/N/index.html, each from a copy of the
// page whose Link names that address — so the canonical, og:url and JSON-LD the
// SEO pass derives from the page describe page N, not page 1. Only page 1 gets
// the derivative outputs (.md/.json) and the sitemap entry: the tail is a slice
// of one document, not more documents.

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// defaultPageSize applies when neither the page nor the site says how many.
// A page that wrote `paginate:` wants pages, so "one page" is not an answer.
const defaultPageSize = 10

// pageChunks returns the chunks a page splits into, or nil for an ordinary
// page. A page whose URL names a file (link: /blog.html) has nowhere to put a
// /page/2/ beneath it and stays whole, with a warning that says so.
func (g *Generator) pageChunks(page models.Page) []termChunk {
	if page.Paginate == nil {
		return nil
	}
	base := page.GetURL()
	if models.HasPageExtension(base) {
		fmt.Printf("   ⚠️  %s: paginate needs a directory-style URL, and %s names a file — rendering unpaginated\n",
			page.SourceFile, base)
		return nil
	}
	base = "/" + strings.Trim(base, "/") + "/"
	if base == "//" {
		base = "/"
	}
	return paginateTerm(g.paginateCollection(page, *page.Paginate), g.pageSize(*page.Paginate), base)
}

// pageSize is the page's own size, else the site's `paginate`, else the default.
func (g *Generator) pageSize(spec models.PaginateSpec) int {
	switch {
	case spec.Size > 0:
		return spec.Size
	case g.config.Paginate > 0:
		return g.config.Paginate
	}
	return defaultPageSize
}

// paginateCollection selects what the page pages over: posts (in the page's
// language under i18n) or pages, optionally narrowed to one content root the
// way feeds: and sitemaps: narrow theirs. A page paging over pages never lists
// itself. An unknown collection name is a loud empty, not a silent one.
func (g *Generator) paginateCollection(page models.Page, spec models.PaginateSpec) []models.Page {
	var pool []models.Page
	switch spec.Collection() {
	case "posts":
		pool = g.siteData.Posts
	case "pages":
		pool = g.siteData.Pages
	default:
		fmt.Printf("   ⚠️  %s: paginate.over %q is not a collection — use posts or pages\n",
			page.SourceFile, spec.Over)
		return nil
	}
	if g.config.I18n.Enabled {
		pool = languagePages(pool, page.Lang)
	}
	out := make([]models.Page, 0, len(pool))
	for _, p := range pool {
		if spec.Collection() == "pages" && p.GetOutputPath() == page.GetOutputPath() {
			continue
		}
		if s := strings.TrimSpace(spec.Source); s != "" && !sourceDirMatches(p.SourceDir, s) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// applyChunk puts one page's slice and pager into a rendered context.
func applyChunk(data map[string]interface{}, chunk termChunk) {
	data["Pager"] = chunk.Pager
	data["Posts"] = chunk.Posts
}

// renderPagedTail writes pages 2..N. Each renders from a copy of the page whose
// Link is that page's address, so everything derived from the page — the
// canonical in context, and the og:url and JSON-LD the SEO pass injects —
// names page N. The ordinary page path is left to the caller, which has
// already written page 1 there.
func (g *Generator) renderPagedTail(page models.Page, chunks []termChunk, templateName string) error {
	for _, chunk := range chunks {
		if chunk.Pager.Current == 1 {
			continue
		}
		url := chunk.Pager.Pages[chunk.Pager.Current-1].URL
		paged := page
		paged.Link = url
		paged.Aliases = nil // aliases redirect to page 1; the tail must not claim them

		outputPath := filepath.Join(g.config.OutputDir, filepath.FromSlash(strings.Trim(url, "/")), indexHTMLName)
		if err := g.ensureWithinOutput(outputPath); err != nil {
			return err
		}
		if err := g.ensureParent(outputPath); err != nil {
			return err
		}
		data := g.pageToTemplateData(paged, false)
		applyChunk(data, chunk)
		if err := g.renderPageTemplate(templateName, outputPath, data, &paged, false); err != nil {
			if strings.Contains(err.Error(), "no such template") || strings.Contains(err.Error(), "is undefined") {
				err = g.renderPageTemplate(pageHTMLName, outputPath, data, &paged, false)
			}
			if err != nil {
				return fmt.Errorf("rendering %s: %w", url, err)
			}
		}
	}
	return nil
}
