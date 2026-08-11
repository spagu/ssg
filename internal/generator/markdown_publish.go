// Markdown publishing (GO-085): with markdown_publish on, every page is written
// a second time as clean Markdown next to its index.html, linked from the page
// <head> as a text/markdown alternate, and indexed in a root llms.txt. SSG is
// Markdown-native, so the published copy is the authored source itself — not an
// HTML→Markdown round-trip — which is exactly what language models and agents
// want to consume instead of parsing rendered HTML.
package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// markdownAlternateTag is the discovery link injected into a page <head>; a
// relative href resolves against the page directory (…/ → …/index.md).
const markdownAlternateTag = `<link rel="alternate" type="text/markdown" href="index.md">`

// writeMarkdownOutput writes a page's Markdown copy when markdown_publish is
// enabled, in both agent-friendly locations: /section/index.md (mirrors
// index.html) and the flat sibling /section.md (for agents that append ".md" to
// a clean URL). The site root has no slug, so it gets only index.md. Content is
// re-encoded to the page's resolved output encoding. Mirrors writeJSONOutput.
func (g *Generator) writeMarkdownOutput(page models.Page, htmlPath string) {
	if !g.config.MarkdownPublish || !strings.HasSuffix(htmlPath, indexHTMLName) {
		return
	}
	data := encodeText(g.pageMarkdown(page), g.encodingFor(&page))
	dir := filepath.Dir(htmlPath)
	// #nosec G306 -- Web content files need to be world-readable
	_ = os.WriteFile(filepath.Join(dir, "index.md"), data, 0644)
	if filepath.Clean(dir) != filepath.Clean(g.config.OutputDir) {
		// #nosec G306 -- Web content files need to be world-readable
		_ = os.WriteFile(dir+".md", data, 0644)
	}
}

// pageMarkdown returns the clean Markdown document for a page: an H1 title
// (unless the body already opens with one) followed by the authored source.
// AI punctuation is normalised when clean_special_chars is on.
func (g *Generator) pageMarkdown(page models.Page) string {
	body := strings.TrimSpace(g.cleanSpecialChars(page.Content))
	var b strings.Builder
	if page.Title != "" && !startsWithH1(body) {
		b.WriteString("# " + g.cleanSpecialChars(page.Title) + "\n\n")
	}
	b.WriteString(body)
	b.WriteString("\n")
	return b.String()
}

// startsWithH1 reports whether the first non-empty line is a level-1 heading.
func startsWithH1(md string) bool {
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		return strings.HasPrefix(t, "# ")
	}
	return false
}

// injectMarkdownAlternate adds the text/markdown discovery link to a page
// <head>, unless one is already present.
func injectMarkdownAlternate(html string) string {
	if strings.Contains(html, `type="text/markdown"`) {
		return html
	}
	if i := strings.LastIndex(html, "</head>"); i >= 0 {
		return html[:i] + markdownAlternateTag + "\n" + html[i:]
	}
	return markdownAlternateTag + "\n" + html
}

// generateLLMsTxt writes /llms.txt: a plain-text index that points agents at the
// Markdown copy of every published page, following the llms.txt convention.
func (g *Generator) generateLLMsTxt() error {
	if !g.config.MarkdownPublish {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", g.config.Domain)
	b.WriteString("> Markdown copies of every page, for language models and agents.\n")
	writeSection(&b, "Documentation", g.siteData.Pages, g.config.Domain)
	writeSection(&b, "Posts", g.siteData.Posts, g.config.Domain)
	data := encodeText(g.cleanSpecialChars(b.String()), normalizeEncoding(g.config.OutputEncoding))
	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(filepath.Join(g.config.OutputDir, "llms.txt"), data, 0644)
}

// writeSection appends one "## Heading" block listing pages as links to their
// Markdown copies. Empty sections are skipped.
func writeSection(b *strings.Builder, heading string, pages []models.Page, domain string) {
	var lines []string
	for _, p := range pages {
		url := markdownURLFor(p, domain)
		if url == "" {
			continue
		}
		title := p.Title
		if title == "" {
			title = p.Slug
		}
		if p.Description != "" {
			lines = append(lines, fmt.Sprintf("- [%s](%s): %s", title, url, p.Description))
		} else {
			lines = append(lines, fmt.Sprintf("- [%s](%s)", title, url))
		}
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s\n\n", heading)
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n")
}

// effectiveHomeLimit resolves a home-card limit: 0 ⇒ the default of 6, a
// negative value ⇒ no limit (the whole list), otherwise the value itself,
// capped at the list length (GO-088).
func effectiveHomeLimit(cfg, total int) int {
	switch {
	case cfg == 0:
		if total < 6 {
			return total
		}
		return 6
	case cfg < 0:
		return total
	case cfg > total:
		return total
	default:
		return cfg
	}
}

// markdownURLFor returns the absolute URL of a page's Markdown copy, or "" for
// pages that are not published as directory-style index.html (no index.md).
func markdownURLFor(p models.Page, domain string) string {
	u := p.GetURL()
	if u == "" || !strings.HasSuffix(u, "/") {
		return "" // flat/other formats have no index.md counterpart
	}
	return httpsScheme + strings.TrimSuffix(domain, "/") + u + "index.md"
}
