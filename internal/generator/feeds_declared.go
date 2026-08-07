package generator

// Declared syndication feeds (#86): several feeds, each choosing its own posts
// and its own format.
//
// The built-in `feed: true` output is all-or-nothing — every post, or one
// taxonomy term, always Atom, always named feed.xml. A site with more than one
// content root cannot offer "just the blog" or "just the guides", and "the three
// tags that mean release" needs three subscriptions. Everything needed to answer
// those is already in memory when the feeds are written; only the declaration was
// missing.

import (
	"encoding/json"
	"fmt"
	stdhtml "html"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// feedFormat describes one output format: how it is rendered and how a browser
// or reader is told about it.
type feedFormat struct {
	mime   string
	render func(g *Generator, f models.FeedSpec, title, selfURL, altURL string, posts []models.Page, full bool) string
}

// feedFormats is the supported set. Podcast and news shapes are deliberately
// absent: both carry namespace requirements that are easy to get subtly wrong,
// and a malformed one is rejected by the platform rather than degraded, so they
// are worth adding only against a real need (#86).
var feedFormats = map[string]feedFormat{
	"atom": {mime: "application/atom+xml", render: renderAtomFeed},
	"rss":  {mime: "application/rss+xml", render: renderRSSFeed},
	"json": {mime: "application/feed+json", render: renderJSONFeed},
}

// feedFormatOf resolves a spec's format, defaulting to atom.
func feedFormatOf(f models.FeedSpec) (feedFormat, string, error) {
	name := strings.ToLower(strings.TrimSpace(f.Format))
	if name == "" {
		name = "atom"
	}
	format, ok := feedFormats[name]
	if !ok {
		return feedFormat{}, name, fmt.Errorf("feeds: %q has unsupported format %q (supported: atom, rss, json)", f.Path, f.Format)
	}
	return format, name, nil
}

// generateDeclaredFeeds writes every feed in `feeds:`. It runs independently of
// `feed: true`, so a site can publish only the feeds it declares.
func (g *Generator) generateDeclaredFeeds() error {
	if len(g.config.Feeds) == 0 {
		return nil
	}
	g.log("📰 Generating declared feeds...")
	for _, spec := range g.config.Feeds {
		if err := g.writeDeclaredFeed(spec); err != nil {
			return err
		}
	}
	return nil
}

// writeDeclaredFeed selects the posts for one spec and writes it in its format.
func (g *Generator) writeDeclaredFeed(spec models.FeedSpec) error {
	rel := models.SanitizeRelPath(strings.TrimSpace(spec.Path))
	if rel == "" {
		return fmt.Errorf("feeds: path is required")
	}
	format, _, err := feedFormatOf(spec)
	if err != nil {
		return err
	}

	posts := g.selectFeedPosts(spec)
	limit := g.config.FeedItems
	if limit <= 0 {
		limit = 20
	}
	if spec.Items != nil && *spec.Items > 0 {
		limit = *spec.Items
	}
	ordered := sortPostsByDate(posts)
	if len(ordered) > limit {
		ordered = ordered[:limit]
	}

	title := strings.TrimSpace(spec.Title)
	if title == "" {
		title = g.config.Domain
	}
	full := g.config.FeedFullContent
	if spec.FullContent != nil {
		full = *spec.FullContent
	}

	base := httpsScheme + g.config.Domain
	selfURL := base + "/" + filepath.ToSlash(rel)
	// A feed for a section points at that section; a whole-site feed at the root.
	altURL := base + "/" + strings.TrimPrefix(feedAltPath(spec), "/")

	body := format.render(g, spec, title, selfURL, altURL, ordered, full)

	outPath := filepath.Join(g.config.OutputDir, filepath.FromSlash(rel))
	if err := g.ensureWithinOutput(outPath); err != nil {
		return err
	}
	// #nosec G301 -- Web content directories need to be world-traversable
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	// #nosec G306 -- Web content files need to be world-readable
	if err := os.WriteFile(outPath, []byte(body), 0644); err != nil {
		return err
	}
	if !g.config.Quiet {
		fmt.Printf("   📰 %s (%d item(s))\n", filepath.ToSlash(rel), len(ordered))
	}
	return nil
}

// feedAltPath is the human page a feed corresponds to — the directory it sits in,
// so /blog/feed.xml points readers at /blog/ rather than at the site root.
func feedAltPath(spec models.FeedSpec) string {
	dir := path.Dir("/" + strings.TrimPrefix(models.SanitizeRelPath(spec.Path), "/"))
	if dir == "." || dir == "/" {
		return "/"
	}
	return dir + "/"
}

// selectFeedPosts narrows the site's content to one feed's selection. Every
// criterion is optional and they combine with AND; a spec with none of them
// covers everything of the requested type.
func (g *Generator) selectFeedPosts(spec models.FeedSpec) []models.Page {
	pool := g.siteData.Posts
	if strings.EqualFold(strings.TrimSpace(spec.Type), "page") {
		pool = g.siteData.Pages
	}
	source := strings.TrimSpace(spec.Source)
	cats := lowerSet(spec.Categories)
	tags := lowerSet(spec.Tags)
	if source == "" && len(cats) == 0 && len(tags) == 0 {
		return pool
	}
	out := make([]models.Page, 0, len(pool))
	for _, p := range pool {
		if source != "" && !pageInSource(p, source) {
			continue
		}
		if len(cats) > 0 && !g.pageInCategories(p, cats) {
			continue
		}
		if len(tags) > 0 && !pageHasAnyTag(p, tags) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// pageInSource reports whether a page came from a content root, matching the
// directory itself and anything beneath it, so `source: blog` covers
// blog/2026/post.md as well as blog/post.md.
func pageInSource(p models.Page, source string) bool {
	dir := filepath.ToSlash(strings.TrimSuffix(p.SourceDir, "/"))
	want := filepath.ToSlash(strings.Trim(source, "/"))
	if dir == "" || want == "" {
		return false
	}
	return dir == want || strings.HasSuffix(dir, "/"+want) ||
		strings.HasPrefix(dir, want+"/") || strings.Contains(dir, "/"+want+"/")
}

// pageInCategories matches a page against category names or slugs, so a config
// may use whichever form reads better.
func (g *Generator) pageInCategories(p models.Page, want map[string]bool) bool {
	if want[strings.ToLower(strings.TrimSpace(p.Category))] {
		return true
	}
	for _, id := range p.Categories {
		if cat, ok := g.siteData.Categories[id]; ok {
			if want[strings.ToLower(cat.Name)] || want[strings.ToLower(cat.Slug)] {
				return true
			}
		}
	}
	return false
}

func pageHasAnyTag(p models.Page, want map[string]bool) bool {
	for _, t := range p.Tags {
		if want[strings.ToLower(strings.TrimSpace(t))] {
			return true
		}
	}
	return false
}

func lowerSet(items []string) map[string]bool {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]bool, len(items))
	for _, s := range items {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			out[s] = true
		}
	}
	return out
}

// feedBody returns the rendered body of one entry, honouring full-content mode,
// and the plain-text summary used by formats that want both.
func (g *Generator) feedBody(p models.Page, full bool) (htmlBody, summary string) {
	if full {
		// Feed readers render this HTML — sanitize like page output (SEC-014).
		htmlBody = g.sanitizeHTML(g.convertMarkdownToHTML(p.Content))
	}
	summary = p.Excerpt
	if summary == "" {
		summary = truncateRunes(tmplStripHTML(g.convertMarkdownToHTML(p.Content)), 300)
	}
	return htmlBody, summary
}

// truncateRunes cuts by runes, never bytes: slicing bytes can split a multibyte
// character and emit invalid UTF-8 into a feed (GO-021).
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// renderAtomFeed renders Atom 1.0 — the same shape the built-in feeds use.
func renderAtomFeed(g *Generator, _ models.FeedSpec, title, selfURL, altURL string, posts []models.Page, full bool) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom"`)
	if len(posts) > 0 && posts[0].Locale != "" {
		fmt.Fprintf(&sb, ` xml:lang="%s"`, stdhtml.EscapeString(posts[0].Locale))
	}
	sb.WriteString(">\n")
	fmt.Fprintf(&sb, "  <title>%s</title>\n", stdhtml.EscapeString(title))
	fmt.Fprintf(&sb, "  <link href=%q rel=\"alternate\"/>\n", altURL)
	fmt.Fprintf(&sb, "  <link href=%q rel=\"self\"/>\n", selfURL)
	fmt.Fprintf(&sb, "  <id>%s</id>\n", altURL)
	if u := g.newestUpdate(posts); !u.IsZero() {
		fmt.Fprintf(&sb, "  <updated>%s</updated>\n", u.UTC().Format(time.RFC3339))
	}
	for _, p := range posts {
		canonical := p.GetCanonical(g.config.Domain)
		htmlBody, summary := g.feedBody(p, full)
		sb.WriteString("  <entry>\n")
		fmt.Fprintf(&sb, "    <title>%s</title>\n", stdhtml.EscapeString(p.Title))
		fmt.Fprintf(&sb, "    <link href=%q/>\n", canonical)
		fmt.Fprintf(&sb, "    <id>%s</id>\n", canonical)
		if !p.Date.IsZero() {
			fmt.Fprintf(&sb, "    <published>%s</published>\n", p.Date.UTC().Format(time.RFC3339))
		}
		if m := g.lastModFor(p); !m.IsZero() {
			fmt.Fprintf(&sb, "    <updated>%s</updated>\n", m.UTC().Format(time.RFC3339))
		}
		if full {
			fmt.Fprintf(&sb, "    <content type=\"html\">%s</content>\n", stdhtml.EscapeString(htmlBody))
		} else {
			fmt.Fprintf(&sb, "    <summary>%s</summary>\n", stdhtml.EscapeString(summary))
		}
		sb.WriteString("  </entry>\n")
	}
	sb.WriteString("</feed>\n")
	return sb.String()
}

// renderRSSFeed renders RSS 2.0. Dates are RFC 1123Z, which is what the format
// requires — an RFC 3339 timestamp is silently ignored by strict readers.
func renderRSSFeed(g *Generator, _ models.FeedSpec, title, selfURL, altURL string, posts []models.Page, full bool) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">` + "\n")
	sb.WriteString("  <channel>\n")
	fmt.Fprintf(&sb, "    <title>%s</title>\n", stdhtml.EscapeString(title))
	fmt.Fprintf(&sb, "    <link>%s</link>\n", stdhtml.EscapeString(altURL))
	fmt.Fprintf(&sb, "    <description>%s</description>\n", stdhtml.EscapeString(title))
	fmt.Fprintf(&sb, "    <atom:link href=%q rel=\"self\" type=\"application/rss+xml\"/>\n", selfURL)
	if len(posts) > 0 && posts[0].Locale != "" {
		fmt.Fprintf(&sb, "    <language>%s</language>\n", stdhtml.EscapeString(posts[0].Locale))
	}
	if u := g.newestUpdate(posts); !u.IsZero() {
		fmt.Fprintf(&sb, "    <lastBuildDate>%s</lastBuildDate>\n", u.UTC().Format(time.RFC1123Z))
	}
	for _, p := range posts {
		canonical := p.GetCanonical(g.config.Domain)
		htmlBody, summary := g.feedBody(p, full)
		body := summary
		if full {
			body = htmlBody
		}
		sb.WriteString("    <item>\n")
		fmt.Fprintf(&sb, "      <title>%s</title>\n", stdhtml.EscapeString(p.Title))
		fmt.Fprintf(&sb, "      <link>%s</link>\n", stdhtml.EscapeString(canonical))
		// isPermaLink=false: the guid is an identifier, and saying otherwise makes
		// a reader re-fetch it as a URL.
		fmt.Fprintf(&sb, "      <guid isPermaLink=\"false\">%s</guid>\n", stdhtml.EscapeString(canonical))
		if !p.Date.IsZero() {
			fmt.Fprintf(&sb, "      <pubDate>%s</pubDate>\n", p.Date.UTC().Format(time.RFC1123Z))
		}
		for _, t := range p.Tags {
			fmt.Fprintf(&sb, "      <category>%s</category>\n", stdhtml.EscapeString(t))
		}
		fmt.Fprintf(&sb, "      <description>%s</description>\n", stdhtml.EscapeString(body))
		sb.WriteString("    </item>\n")
	}
	sb.WriteString("  </channel>\n</rss>\n")
	return sb.String()
}

// renderJSONFeed renders JSON Feed 1.1. Built with encoding/json rather than
// string concatenation, so escaping is the encoder's problem and cannot be got
// wrong by hand.
func renderJSONFeed(g *Generator, _ models.FeedSpec, title, selfURL, altURL string, posts []models.Page, full bool) string {
	type jsonItem struct {
		ID            string   `json:"id"`
		URL           string   `json:"url"`
		Title         string   `json:"title"`
		ContentHTML   string   `json:"content_html,omitempty"`
		ContentText   string   `json:"content_text,omitempty"`
		Summary       string   `json:"summary,omitempty"`
		DatePublished string   `json:"date_published,omitempty"`
		DateModified  string   `json:"date_modified,omitempty"`
		Tags          []string `json:"tags,omitempty"`
	}
	doc := struct {
		Version     string     `json:"version"`
		Title       string     `json:"title"`
		HomePageURL string     `json:"home_page_url"`
		FeedURL     string     `json:"feed_url"`
		Items       []jsonItem `json:"items"`
	}{
		Version:     "https://jsonfeed.org/version/1.1",
		Title:       title,
		HomePageURL: altURL,
		FeedURL:     selfURL,
		Items:       make([]jsonItem, 0, len(posts)),
	}
	for _, p := range posts {
		htmlBody, summary := g.feedBody(p, full)
		canonical := p.GetCanonical(g.config.Domain)
		it := jsonItem{ID: canonical, URL: canonical, Title: p.Title, Summary: summary, Tags: p.Tags}
		if full {
			it.ContentHTML = htmlBody
		} else {
			it.ContentText = summary
		}
		if !p.Date.IsZero() {
			it.DatePublished = p.Date.UTC().Format(time.RFC3339)
		}
		if m := g.lastModFor(p); !m.IsZero() {
			it.DateModified = m.UTC().Format(time.RFC3339)
		}
		doc.Items = append(doc.Items, it)
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b) + "\n"
}

// newestUpdate is the most recent modification across a feed's posts.
func (g *Generator) newestUpdate(posts []models.Page) time.Time {
	var newest time.Time
	for _, p := range posts {
		if m := g.lastModFor(p); m.After(newest) {
			newest = m
		}
	}
	return newest
}

// feedAutodiscoveryLinks renders one <link rel="alternate"> per feed the site
// publishes, each with its own MIME type and title.
//
// A reader that offers a choice reads exactly these links, so publishing four
// feeds behind a single Atom <link> hides three of them. The built-in feed is
// included when `feed: true`, and a declared feed whose format is unknown is
// skipped rather than advertised with a wrong type.
func (g *Generator) feedAutodiscoveryLinks() string {
	var sb strings.Builder
	if g.config.Feed {
		fmt.Fprintf(&sb, `<link rel="alternate" type="application/atom+xml" title=%q href="/%s">`+"\n",
			stdhtml.EscapeString(g.config.Domain), feedFileName)
	}
	for _, spec := range g.config.Feeds {
		rel := models.SanitizeRelPath(strings.TrimSpace(spec.Path))
		if rel == "" {
			continue
		}
		format, _, err := feedFormatOf(spec)
		if err != nil {
			continue
		}
		title := strings.TrimSpace(spec.Title)
		if title == "" {
			title = g.config.Domain
		}
		fmt.Fprintf(&sb, `<link rel="alternate" type=%q title=%q href="/%s">`+"\n",
			format.mime, stdhtml.EscapeString(title), filepath.ToSlash(rel))
	}
	return sb.String()
}

// injectFeedLinks adds the autodiscovery <link> elements to a rendered page,
// unless the page already advertises a feed of its own. Applied to every HTML
// page — including the homepage, which carries no page context and so was
// skipped by the SEO block that used to own this (#86).
func (g *Generator) injectFeedLinks(s string) string {
	if !g.config.Feed && len(g.config.Feeds) == 0 {
		return s
	}
	if strings.Contains(s, `rel="alternate" type="application/atom+xml"`) ||
		strings.Contains(s, `rel="alternate" type="application/rss+xml"`) ||
		strings.Contains(s, `rel="alternate" type="application/feed+json"`) {
		return s // the theme provides its own
	}
	links := g.feedAutodiscoveryLinks()
	if links == "" {
		return s
	}
	if i := strings.LastIndex(s, "</head>"); i >= 0 {
		return s[:i] + links + s[i:]
	}
	return s
}
