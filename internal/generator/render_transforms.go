// Render-time HTML transforms (PERF-005). Every per-file transformation — SEO
// block, KaTeX injection, relative links, prettify/minify — is applied to the
// rendered page while it is still in memory, so each HTML file is written to
// disk exactly once. Only genuinely global passes (asset fingerprinting, link
// checking, WebP reference rewriting) still walk the output tree. The legacy
// file-based helpers delegate to the string functions below, keeping one source
// of truth for each transformation.
package generator

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"os"
	"regexp"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// prettifyHTMLString removes blank lines and trailing whitespace, normalising
// line endings; output always ends with a single newline.
func prettifyHTMLString(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if strings.TrimSpace(trimmed) != "" {
			result = append(result, trimmed)
		}
	}
	return strings.Join(result, "\n") + "\n"
}

// minifyHTMLString collapses inter-tag whitespace and strips comments while
// preserving htmlmin:ignore blocks and whitespace-sensitive elements (GO-022).
func minifyHTMLString(s string) string {
	preservedBlocks := make(map[string]string)
	preserve := func(inner string) string {
		placeholder := fmt.Sprintf("__HTMLMIN_PRESERVE_%d__", len(preservedBlocks))
		preservedBlocks[placeholder] = inner
		return placeholder
	}
	s = minIgnoreBlockRe.ReplaceAllStringFunc(s, func(match string) string {
		if inner := minIgnoreBlockRe.FindStringSubmatch(match); len(inner) > 1 {
			return preserve(inner[1])
		}
		return match
	})
	s = minPreserveTagRe.ReplaceAllStringFunc(s, preserve)
	s = minHTMLCommentRe.ReplaceAllStringFunc(s, func(match string) string {
		if strings.HasPrefix(match, "<!--[if") {
			return match
		}
		return ""
	})
	s = minTagGapRe.ReplaceAllString(s, "><")
	s = minMultiSpaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	for placeholder, content := range preservedBlocks {
		s = strings.ReplaceAll(s, placeholder, content)
	}
	return s
}

// relativizeHTMLString rewrites absolute URLs pointing at domain into relative
// links (href/src/action attributes and url() in inline styles).
func relativizeHTMLString(s, domain string) string {
	baseDomain := strings.TrimPrefix(domain, "https://")
	baseDomain = strings.TrimPrefix(baseDomain, "http://")
	baseDomain = strings.TrimSuffix(baseDomain, "/")

	patterns := []string{"https://" + baseDomain, "http://" + baseDomain, "//" + baseDomain}
	for _, pattern := range patterns {
		for _, attr := range []string{"href", "src", "action"} {
			s = strings.ReplaceAll(s, attr+`="`+pattern+`"`, attr+`="/"`)
			s = strings.ReplaceAll(s, attr+`='`+pattern+`'`, attr+`='/'`)
			s = strings.ReplaceAll(s, attr+`="`+pattern+`/`, attr+`="/`)
			s = strings.ReplaceAll(s, attr+`='`+pattern+`/`, attr+`='/`)
		}
		s = strings.ReplaceAll(s, `url(`+pattern+`/`, `url(/`)
		s = strings.ReplaceAll(s, `url("`+pattern+`/`, `url("/`)
		s = strings.ReplaceAll(s, `url('`+pattern+`/`, `url('/`)
	}
	return s
}

// mathHTMLString wires KaTeX assets into pages that contain display math and
// are not already wired (AX-004).
func mathHTMLString(s string) string {
	if !strings.Contains(s, "$$") || strings.Contains(s, "katex.min.css") {
		return s
	}
	return injectKatexAssets(s)
}

// mermaidHTMLString wires the mermaid.js runtime into pages that contain a
// mermaid diagram and are not already wired (GO-073). The configured theme and
// background colour (GO-079) tune legibility on dark chrome.
func (g *Generator) mermaidHTMLString(s string) string {
	if !strings.Contains(s, `class="mermaid"`) || strings.Contains(s, "mermaid@") {
		return s
	}
	return injectMermaidAssets(s, g.config.MermaidTheme, g.config.MermaidBackground)
}

// seoHTMLString adds the generator-level SEO block (OpenGraph/Twitter/JSON-LD,
// feed alternate, hreflang) for the parts the theme did not provide (SEO-003).
// A no-op unless SEO injection is enabled (`seo`, opt-in since v1.8.2).
func (g *Generator) seoHTMLString(s string, page models.Page, isPost bool) string {
	if !g.config.SEO {
		return s
	}
	var b strings.Builder
	if !strings.Contains(s, "og:title") {
		b.WriteString(g.buildOpenGraph(page, isPost)) // includes the JSON-LD
	} else if !strings.Contains(s, "application/ld+json") {
		// The theme already provides OpenGraph tags but no structured data —
		// inject the JSON-LD on its own so AI-first Linked Data (#61) still reaches
		// themes that supply their own social meta (e.g. ssgtheme).
		b.WriteString(g.buildJSONLD(page, isPost))
	}
	if !strings.Contains(s, "hreflang") {
		b.WriteString(string(g.hreflangTags(page)))
	}
	// The site's own identity, discovered when the content was migrated:
	// icons, social defaults and verification tokens (GO-100 follow-up). Only
	// what the theme has not already emitted.
	b.WriteString(g.buildMarketingHead(s))
	// The site palette as CSS custom properties, so a theme can style against
	// the source site's colours instead of the author copying hex codes (#128).
	b.WriteString(g.buildPaletteHead(s))
	b.WriteString(g.analyticsSnippet(s))
	// Fall back to the front-matter description when the theme emitted no usable
	// one (#76). Nothing is invented here — the author already wrote it, it just
	// never reached the output because the template interpolated a different
	// field. An existing but EMPTY tag is rewritten in place rather than joined by
	// a second one: two description tags, the first of them blank, is worse than
	// the problem being fixed.
	s = fillMetaDescription(s, page.Description)
	if b.Len() == 0 {
		return s
	}
	if i := strings.LastIndex(s, "</head>"); i >= 0 {
		return s[:i] + b.String() + s[i:]
	}
	return b.String() + s
}

// transformHTMLPage applies every enabled per-file transform to a rendered page,
// in the same order the former tree-walks ran: SEO → math → relative links →
// prettify or minify. Pages pass their models.Page for SEO; page-less HTML
// (index, archives, alias stubs) passes nil.
func (g *Generator) transformHTMLPage(s string, page *models.Page, isPost bool) string {
	if g.config.I18n.Enabled {
		lang := g.currentLang
		if page != nil && page.Lang != "" {
			lang = page.Lang
		}
		if lang != "" {
			htmlLang := regexp.MustCompile(`(?i)<html([^>]*?)\s+lang=(?:"[^"]*"|'[^']*')`)
			if htmlLang.MatchString(s) {
				s = htmlLang.ReplaceAllString(s, `<html${1} lang="`+stdhtml.EscapeString(lang)+`"`)
			} else {
				s = strings.Replace(s, "<html", `<html lang="`+stdhtml.EscapeString(lang)+`"`, 1)
			}
		}
	}
	if page != nil {
		s = g.seoHTMLString(s, *page, isPost)
		// Point agents at the page's Markdown copy (GO-085). Only real source
		// pages get an index.md; the synthetic home/listing context carries no
		// Content, so it is skipped and never advertises a missing .md.
		if g.config.MarkdownPublish && page.Content != "" {
			s = injectMarkdownAlternate(s, markdownLeaf(page.GetURL()))
		}
	}
	// Feed autodiscovery is injected for every page, not only those with a page
	// context. The SEO block runs only for posts and pages, so the site homepage
	// — the first place a reader or a subscription tool looks — never advertised
	// a feed at all (#86).
	s = g.injectFeedLinks(s)
	if g.config.Math {
		s = mathHTMLString(s)
	}
	if g.config.Mermaid {
		s = g.mermaidHTMLString(s)
	}
	if g.config.RelativeLinks && g.config.Domain != "" {
		s = relativizeHTMLString(s, g.config.Domain)
	}
	if g.config.PrettyHTML && !g.config.MinifyHTML {
		s = prettifyHTMLString(s)
	}
	if g.config.MinifyHTML {
		s = minifyHTMLString(s)
	}
	// Last, so it sees the finished document: characters arrive from content,
	// from a template and from an injected block alike, and a pass that ran
	// earlier would miss whatever a later step added (#176).
	s = g.sanitizeHTMLString(s)
	return s
}

// renderPageTemplate renders a template into memory, applies the per-file HTML
// transforms and writes the result in a single write (PERF-005). page carries
// the SEO context for posts/pages; nil for listing pages.
func (g *Generator) renderPageTemplate(templateName, outputPath string, data interface{}, page *models.Page, isPost bool) error {
	if g.engine != nil {
		return g.renderWithEngine(templateName, outputPath, data, page, isPost)
	}
	var buf bytes.Buffer
	if err := g.tmpl.ExecuteTemplate(&buf, templateName, data); err != nil {
		return err
	}
	out := buf.String()
	data2 := []byte(out)
	if strings.HasSuffix(strings.ToLower(outputPath), ".html") {
		out = g.transformHTMLPage(out, page, isPost)
		// Re-encode the finished HTML to the page's output encoding, keeping the
		// declared <meta charset> in step with the bytes on disk (GO-087).
		enc := g.encodingFor(page)
		out = setHTMLCharset(out, enc)
		data2 = encodeText(out, enc)
	}
	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(outputPath, data2, 0644)
}
