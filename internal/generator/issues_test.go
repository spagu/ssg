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

	// BACKWARD COMPAT: plain headings keep goldmark's ids bit-for-bit —
	// punctuation quirks ("foo--bar") included — so anchors on pre-1.8.6
	// sites stay valid; only link-bearing headings are recomputed.
	html = g.convertMarkdownToHTML("## Foo & Bar\n\n## my_heading\n")
	wantContains(t, "plain headings unchanged", html,
		`<h2 id="foo--bar">`, `<h2 id="my-heading">`)
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
		          {"id":1700,"name":"Śląsk News","slug":"slask-news"},
		          {"id":1800,"name":"Hand Written","slug":"custom-hand"}]}`)
	mustWrite(t, filepath.Join(tmp, "content", "site", "posts", "news", "one.md"),
		"---\ntitle: Numeric tags\nslug: numeric-tags\nstatus: publish\ntype: post\ndate: 2026-07-10\nauthor: 101\ntags:\n  - 1691\n  - 1700\n  - 9999\n  - plain-tag\n  - Hand Written\n---\n\nBody.\n")
	writeSimpleTemplates(t, filepath.Join(tmp, "templates", "simple"))
	gen, out := buildSite(t, tmp)
	// Resolved names on the post.
	post := gen.siteData.Posts[0]
	if got := strings.Join(post.Tags, "|"); got != "eSports Betting|Śląsk News|9999|plain-tag|Hand Written" {
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
	// BACKWARD COMPAT: hand-written tag names keep their historical DERIVED
	// slug even when metadata carries a different canonical one — pre-1.8.6
	// tag URLs never change; canonical slugs apply only to id-resolved tags.
	if _, err := os.Stat(filepath.Join(out, "tag", "hand-written", "index.html")); err != nil {
		t.Fatal("hand-written tag must keep its derived slug")
	}
	if _, err := os.Stat(filepath.Join(out, "tag", "custom-hand", "index.html")); err == nil {
		t.Fatal("canonical slug must not apply to hand-written tag names")
	}
	wantContains(t, "sitemap", mustRead(t, filepath.Join(out, "sitemap.xml")),
		"/tag/esports-betting/", "/tag/slask-news/")
}

func TestIssue31YamlComment(t *testing.T) {
	yamlContent := `requests:
  - id: test-entry
    answer: some text mentioning issue #24-71 here
      continued on next line.
    status: open`

	tmp := t.TempDir()
	mustWrite(t, filepath.Join(tmp, "content", "metadata.json"), `{"categories":[],"exported_at":"","media":[],"users":[],"tags":[]}`)
	mustWrite(t, filepath.Join(tmp, "content", "posts", "news", "one.md"), "---\ntitle: Numeric tags\nslug: numeric-tags\nstatus: publish\ntype: post\ndate: 2026-07-10\n---\n\nBody.\n")
	mustWrite(t, filepath.Join(tmp, "data", "repro.yaml"), yamlContent)
	writeSimpleTemplates(t, filepath.Join(tmp, "templates"))
	
	gen, err := New(Config{
		Domain:       "example.com",
		ContentDir:   filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"),
		DataDir:      filepath.Join(tmp, "data"),
		OutputDir:    filepath.Join(tmp, "output"),
		Quiet:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = gen.Generate()
	if err == nil {
		t.Fatal("expected error due to invalid yaml syntax, but got none")
	}
	expectedHint := "Line 3: \"answer: some text mentioning issue #24-71 here\" contains ' #' (space followed by hash)"
	if !strings.Contains(err.Error(), expectedHint) {
		t.Fatalf("expected error to contain hint %q, but got: %v", expectedHint, err)
	}
}

func TestDataParserCoverage(t *testing.T) {
	g, err := New(Config{Domain: "example.com", Quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	data := make(map[string]interface{})

	// 1. JSON parse error
	jsonFile := filepath.Join(tmp, "invalid.json")
	if err := os.WriteFile(jsonFile, []byte(`{"key": "value"`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := g.parseDataFile(jsonFile, ".json", tmp, data); err == nil {
		t.Error("expected error parsing invalid JSON")
	}

	// 2. YAML parse error without space+#, but with a comment line
	yamlFile := filepath.Join(tmp, "invalid.yaml")
	if err := os.WriteFile(yamlFile, []byte("# comment line\nkey: val\nkey2"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := g.parseDataFile(yamlFile, ".yaml", tmp, data); err == nil {
		t.Error("expected error parsing invalid YAML")
	}

	// Clean up invalid files before walking
	_ = os.Remove(jsonFile)
	_ = os.Remove(yamlFile)

	// 3. Skip non-yaml/json files
	txtFile := filepath.Join(tmp, "skipped.txt")
	if err := os.WriteFile(txtFile, []byte("text"), 0644); err != nil {
		t.Fatal(err)
	}
	g.config.DataDir = tmp
	if err := g.loadData(); err != nil {
		t.Errorf("expected no error with skipped file, got %v", err)
	}

	// 4. Non-existent data directory
	g.config.DataDir = filepath.Join(tmp, "non-existent-dir")
	if err := g.loadData(); err != nil {
		t.Errorf("expected no error with non-existent data dir, got %v", err)
	}
}



