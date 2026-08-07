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

	"github.com/spagu/ssg/internal/externalsource"
	"github.com/spagu/ssg/internal/models"
)

// feedFormat describes one output format: how it is rendered and how a browser
// or reader is told about it.
type feedFormat struct {
	mime   string
	render func(g *Generator, page feedPage) string
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

// feedPage is one rendered page of a feed: its metadata, its items, and the
// RFC 5005 links tying it to its neighbours when the feed is paginated.
type feedPage struct {
	title, selfURL, altURL string
	items                  []externalsource.FeedItem
	full                   bool
	prevURL, nextURL       string
	firstURL, lastURL      string
}

// writeDeclaredFeed collects a feed's items and writes it, in pages when the
// spec asks for them.
func (g *Generator) writeDeclaredFeed(spec models.FeedSpec) error {
	rel := models.SanitizeRelPath(strings.TrimSpace(spec.Path))
	if rel == "" {
		return fmt.Errorf("feeds: path is required")
	}
	format, _, err := feedFormatOf(spec)
	if err != nil {
		return err
	}

	full := g.config.FeedFullContent
	if spec.FullContent != nil {
		full = *spec.FullContent
	}
	items := g.feedItemsFor(spec, full)

	limit := g.config.FeedItems
	if limit <= 0 {
		limit = 20
	}
	if spec.Items != nil && *spec.Items > 0 {
		limit = *spec.Items
	}
	if len(items) > limit {
		items = items[:limit]
	}

	title := strings.TrimSpace(spec.Title)
	if title == "" {
		title = g.config.Domain
	}
	base := httpsScheme + g.config.Domain
	altURL := base + "/" + strings.TrimPrefix(feedAltPath(spec), "/")

	// Pagination splits the archive rather than shipping one huge document; the
	// pages are linked with RFC 5005 rel="next"/"prev" so a reader can walk back
	// through them instead of only seeing the newest slice.
	per := spec.Paginate
	if per <= 0 || per >= len(items) {
		page := feedPage{title: title, selfURL: base + "/" + filepath.ToSlash(rel),
			altURL: altURL, items: items, full: full}
		return g.writeFeedFile(rel, format.render(g, page), len(items))
	}
	total := (len(items) + per - 1) / per
	for n := 1; n <= total; n++ {
		lo := (n - 1) * per
		hi := lo + per
		if hi > len(items) {
			hi = len(items)
		}
		page := feedPage{
			title: title, altURL: altURL, items: items[lo:hi], full: full,
			selfURL:  base + "/" + filepath.ToSlash(feedPagePath(rel, n)),
			firstURL: base + "/" + filepath.ToSlash(feedPagePath(rel, 1)),
			lastURL:  base + "/" + filepath.ToSlash(feedPagePath(rel, total)),
		}
		if n > 1 {
			page.prevURL = base + "/" + filepath.ToSlash(feedPagePath(rel, n-1))
		}
		if n < total {
			page.nextURL = base + "/" + filepath.ToSlash(feedPagePath(rel, n+1))
		}
		if err := g.writeFeedFile(feedPagePath(rel, n), format.render(g, page), hi-lo); err != nil {
			return err
		}
	}
	return nil
}

// feedItemsFor returns a feed's items: an aggregate of several inputs, or this
// site's own posts when no aggregation is declared.
func (g *Generator) feedItemsFor(spec models.FeedSpec, full bool) []externalsource.FeedItem {
	if len(spec.Aggregate) > 0 {
		return g.aggregateItems(spec)
	}
	posts := sortPostsByDate(g.selectFeedPosts(spec))
	out := make([]externalsource.FeedItem, 0, len(posts))
	for _, p := range posts {
		htmlBody, summary := g.feedBody(p, full)
		canonical := p.GetCanonical(g.config.Domain)
		out = append(out, externalsource.FeedItem{
			ID: canonical, URL: canonical, Title: p.Title, Summary: summary,
			ContentHTML: htmlBody, Published: p.Date, Updated: g.lastModFor(p), Tags: p.Tags,
		})
	}
	return out
}

// feedPagePath is the file for page n: page 1 keeps the declared path, so the
// advertised URL never moves as the archive grows.
func feedPagePath(rel string, n int) string {
	if n <= 1 {
		return rel
	}
	ext := path.Ext(rel)
	return strings.TrimSuffix(rel, ext) + fmt.Sprintf("-%d", n) + ext
}

// writeFeedFile writes one rendered feed page.
func (g *Generator) writeFeedFile(rel, body string, count int) error {
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
		fmt.Printf("   📰 %s (%d item(s))\n", filepath.ToSlash(rel), count)
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

// renderAtomFeed renders Atom 1.0.
func renderAtomFeed(g *Generator, p feedPage) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom">` + "\n")
	fmt.Fprintf(&sb, "  <title>%s</title>\n", stdhtml.EscapeString(p.title))
	fmt.Fprintf(&sb, "  <link href=%q rel=\"alternate\"/>\n", p.altURL)
	fmt.Fprintf(&sb, "  <link href=%q rel=\"self\"/>\n", p.selfURL)
	// RFC 5005 paging, so a reader can walk the whole archive rather than only
	// the newest page.
	for rel, href := range map[string]string{"first": p.firstURL, "last": p.lastURL, "previous": p.prevURL, "next": p.nextURL} {
		if href != "" {
			fmt.Fprintf(&sb, "  <link href=%q rel=%q/>\n", href, rel)
		}
	}
	fmt.Fprintf(&sb, "  <id>%s</id>\n", p.altURL)
	if u := newestItemUpdate(p.items); !u.IsZero() {
		fmt.Fprintf(&sb, "  <updated>%s</updated>\n", u.UTC().Format(time.RFC3339))
	}
	for _, it := range p.items {
		sb.WriteString("  <entry>\n")
		fmt.Fprintf(&sb, "    <title>%s</title>\n", stdhtml.EscapeString(it.Title))
		fmt.Fprintf(&sb, "    <link href=%q/>\n", it.URL)
		fmt.Fprintf(&sb, "    <id>%s</id>\n", stdhtml.EscapeString(it.ID))
		if !it.Published.IsZero() {
			fmt.Fprintf(&sb, "    <published>%s</published>\n", it.Published.UTC().Format(time.RFC3339))
		}
		if !it.Updated.IsZero() {
			fmt.Fprintf(&sb, "    <updated>%s</updated>\n", it.Updated.UTC().Format(time.RFC3339))
		}
		if it.Author != "" {
			fmt.Fprintf(&sb, "    <author><name>%s</name></author>\n", stdhtml.EscapeString(it.Author))
		}
		for _, t := range feedItemTerms(it) {
			fmt.Fprintf(&sb, "    <category term=%q/>\n", stdhtml.EscapeString(t))
		}
		if p.full && it.ContentHTML != "" {
			fmt.Fprintf(&sb, "    <content type=\"html\">%s</content>\n", stdhtml.EscapeString(it.ContentHTML))
		} else {
			fmt.Fprintf(&sb, "    <summary>%s</summary>\n", stdhtml.EscapeString(it.Summary))
		}
		sb.WriteString("  </entry>\n")
	}
	sb.WriteString("</feed>\n")
	return sb.String()
}

// renderRSSFeed renders RSS 2.0. Dates are RFC 1123Z, which the format requires —
// an RFC 3339 timestamp is silently ignored by strict readers.
func renderRSSFeed(g *Generator, p feedPage) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">` + "\n")
	sb.WriteString("  <channel>\n")
	fmt.Fprintf(&sb, "    <title>%s</title>\n", stdhtml.EscapeString(p.title))
	fmt.Fprintf(&sb, "    <link>%s</link>\n", stdhtml.EscapeString(p.altURL))
	fmt.Fprintf(&sb, "    <description>%s</description>\n", stdhtml.EscapeString(p.title))
	fmt.Fprintf(&sb, "    <atom:link href=%q rel=\"self\" type=\"application/rss+xml\"/>\n", p.selfURL)
	for rel, href := range map[string]string{"first": p.firstURL, "last": p.lastURL, "previous": p.prevURL, "next": p.nextURL} {
		if href != "" {
			fmt.Fprintf(&sb, "    <atom:link href=%q rel=%q/>\n", href, rel)
		}
	}
	if u := newestItemUpdate(p.items); !u.IsZero() {
		fmt.Fprintf(&sb, "    <lastBuildDate>%s</lastBuildDate>\n", u.UTC().Format(time.RFC1123Z))
	}
	for _, it := range p.items {
		body := it.Summary
		if p.full && it.ContentHTML != "" {
			body = it.ContentHTML
		}
		sb.WriteString("    <item>\n")
		fmt.Fprintf(&sb, "      <title>%s</title>\n", stdhtml.EscapeString(it.Title))
		fmt.Fprintf(&sb, "      <link>%s</link>\n", stdhtml.EscapeString(it.URL))
		fmt.Fprintf(&sb, "      <guid isPermaLink=\"false\">%s</guid>\n", stdhtml.EscapeString(it.ID))
		if !it.Published.IsZero() {
			fmt.Fprintf(&sb, "      <pubDate>%s</pubDate>\n", it.Published.UTC().Format(time.RFC1123Z))
		}
		for _, t := range feedItemTerms(it) {
			fmt.Fprintf(&sb, "      <category>%s</category>\n", stdhtml.EscapeString(t))
		}
		fmt.Fprintf(&sb, "      <description>%s</description>\n", stdhtml.EscapeString(body))
		sb.WriteString("    </item>\n")
	}
	sb.WriteString("  </channel>\n</rss>\n")
	return sb.String()
}

// renderJSONFeed renders JSON Feed 1.1, built through encoding/json so escaping
// is the encoder's problem rather than something to get wrong by hand.
func renderJSONFeed(g *Generator, p feedPage) string {
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
		Author        *struct {
			Name string `json:"name"`
		} `json:"author,omitempty"`
	}
	doc := struct {
		Version     string     `json:"version"`
		Title       string     `json:"title"`
		HomePageURL string     `json:"home_page_url"`
		FeedURL     string     `json:"feed_url"`
		NextURL     string     `json:"next_url,omitempty"`
		Items       []jsonItem `json:"items"`
	}{
		Version: "https://jsonfeed.org/version/1.1", Title: p.title,
		HomePageURL: p.altURL, FeedURL: p.selfURL, NextURL: p.nextURL,
		Items: make([]jsonItem, 0, len(p.items)),
	}
	for _, it := range p.items {
		ji := jsonItem{ID: it.ID, URL: it.URL, Title: it.Title, Summary: it.Summary, Tags: feedItemTerms(it)}
		if p.full && it.ContentHTML != "" {
			ji.ContentHTML = it.ContentHTML
		} else {
			ji.ContentText = it.Summary
		}
		if it.Author != "" {
			ji.Author = &struct {
				Name string `json:"name"`
			}{Name: it.Author}
		}
		if !it.Published.IsZero() {
			ji.DatePublished = it.Published.UTC().Format(time.RFC3339)
		}
		if !it.Updated.IsZero() {
			ji.DateModified = it.Updated.UTC().Format(time.RFC3339)
		}
		doc.Items = append(doc.Items, ji)
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b) + "\n"
}

// feedItemTerms are the item's tags plus its provenance label, so a reader or a
// downstream template can group an aggregate by where each item came from —
// which is the whole reason the label is carried through collection.
func feedItemTerms(it externalsource.FeedItem) []string {
	terms := append([]string(nil), it.Tags...)
	if it.Label != "" {
		terms = append(terms, it.Label)
	}
	return terms
}

// newestItemUpdate is the most recent timestamp across a page's items.
func newestItemUpdate(items []externalsource.FeedItem) time.Time {
	var newest time.Time
	for _, it := range items {
		t := it.Updated
		if t.IsZero() {
			t = it.Published
		}
		if t.After(newest) {
			newest = t
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
	// feed_autodiscovery: false hands the links to the theme, for a site that
	// wants control over their order, titles or which feeds are advertised.
	if g.config.FeedAutodiscovery != nil && !*g.config.FeedAutodiscovery {
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
