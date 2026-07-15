package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHeadingIDFromVisibleText (#26): heading ids derive from the visible
// text — a Markdown link's href must not leak into the id; duplicates get -N
// suffixes and the TOC references the same ids.
func TestHeadingIDFromVisibleText(t *testing.T) {
	g, err := New(Config{Domain: "example.com", TOC: true})
	if err != nil {
		t.Fatal(err)
	}
	md := "### [Ian Zane](/authors/ian-zane/) — Generalist\n\n## Our Team\n\n## Our Team\n"
	html := g.convertMarkdownToHTML(md)
	wantContains(t, "headings", html,
		`<h3 id="ian-zane-generalist">`, `<h2 id="our-team">`, `<h2 id="our-team-1">`)
	if strings.Contains(html, "authorsian-zane") {
		t.Fatalf("href leaked into heading id: %s", html)
	}
	toc := string(g.tocHTML(md))
	wantContains(t, "toc", toc, `href="#ian-zane-generalist"`, `href="#our-team"`, `href="#our-team-1"`)

	// Empty-text heading falls back to a stable id.
	html = g.convertMarkdownToHTML("## ![](/img/logo.png)\n")
	wantContains(t, "empty heading", html, `<h2 id="heading">`)
}

// TestNumericTagIDsResolve (#27): numeric WordPress tag ids in frontmatter
// resolve via metadata.json `tags` (name + canonical slug), like author ids
// resolve via `users`.
func TestNumericTagIDsResolve(t *testing.T) {
	tmp := t.TempDir()
	mustWrite(t, filepath.Join(tmp, "content", "site", "metadata.json"),
		`{"categories":[],"exported_at":"","media":[],
		  "users":[{"id":101,"name":"Ian Zane","slug":"ian-zane"}],
		  "tags":[{"id":1691,"name":"eSports Betting","slug":"esports-betting"},
		          {"id":1700,"name":"Śląsk News","slug":"slask-news"}]}`)
	mustWrite(t, filepath.Join(tmp, "content", "site", "posts", "news", "one.md"),
		"---\ntitle: Numeric tags\nslug: numeric-tags\nstatus: publish\ntype: post\ndate: 2026-07-10\nauthor: 101\ntags:\n  - 1691\n  - 1700\n  - 9999\n  - plain-tag\n---\n\nBody.\n")
	writeSimpleTemplates(t, filepath.Join(tmp, "templates", "simple"))
	gen, err := New(Config{Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir: filepath.Join(tmp, "output"), Quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := filepath.Join(tmp, "output")
	// Resolved names on the post.
	post := gen.siteData.Posts[0]
	if got := strings.Join(post.Tags, "|"); got != "eSports Betting|Śląsk News|9999|plain-tag" {
		t.Fatalf("tags = %q", got)
	}
	// Archives under metadata slugs (canonical WordPress slug wins over
	// derived slugify — "Śląsk News" would otherwise mangle).
	wantFiles(t,
		filepath.Join(out, "tag", "esports-betting", "index.html"),
		filepath.Join(out, "tag", "slask-news", "index.html"),
		filepath.Join(out, "tag", "plain-tag", "index.html"),
		filepath.Join(out, "author", "ian-zane", "index.html"))
	// No raw-id archives for resolved tags; unknown ids pass through.
	if _, err := os.Stat(filepath.Join(out, "tag", "1691", "index.html")); err == nil {
		t.Fatal("raw numeric tag archive must not exist for resolved ids")
	}
	if _, err := os.Stat(filepath.Join(out, "tag", "9999", "index.html")); err != nil {
		t.Fatal("unknown numeric ids must pass through unchanged")
	}
	wantContains(t, "sitemap", mustRead(t, filepath.Join(out, "sitemap.xml")),
		"/tag/esports-betting/", "/tag/slask-news/")
}
