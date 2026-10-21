package generator

import (
	stdhtml "html"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestAbsoluteFeedHTML: every kind of URL a post's HTML can hold, resolved
// against the post's address or left alone (#347).
func TestAbsoluteFeedHTML(t *testing.T) {
	post := "https://example.com/blog/x/"
	cases := map[string]string{
		`<img src="/images/a.svg">`:                      `<img src="https://example.com/images/a.svg">`,
		`<img src="img/b.png">`:                          `<img src="https://example.com/blog/x/img/b.png">`,
		`<a href="../y/">`:                               `<a href="https://example.com/blog/y/">`,
		`<a href="#step-2">`:                             `<a href="https://example.com/blog/x/#step-2">`,
		`<a href="/stats/?q=1&amp;p=2">`:                 `<a href="https://example.com/stats/?q=1&amp;p=2">`,
		`<img srcset="/a.png 1x, b.png 2x">`:             `<img srcset="https://example.com/a.png 1x, https://example.com/blog/x/b.png 2x">`,
		`<video poster=/p.jpg>`:                          `<video poster=https://example.com/p.jpg>`,
		`<div style="background:url(/bg.png)">`:          `<div style="background:url(https://example.com/bg.png)">`,
		`<a href="https://other.example.org/z">`:         `<a href="https://other.example.org/z">`,
		`<img src="//cdn.example.com/c.png">`:            `<img src="//cdn.example.com/c.png">`,
		`<a href="mailto:a@example.com">`:                `<a href="mailto:a@example.com">`,
		`<img src="data:image/png;base64,AAAA">`:         `<img src="data:image/png;base64,AAAA">`,
		`<pre><code>src="/usr/bin" href=/x</code></pre>`: `<pre><code>src="/usr/bin" href=/x</code></pre>`,
	}
	for in, want := range cases {
		if got := absoluteFeedHTML(in, post); got != want {
			t.Errorf("absoluteFeedHTML(%s)\n got %s\nwant %s", in, got, want)
		}
	}
	// Without an absolute address to resolve against, nothing changes.
	for _, base := range []string{"", "/blog/x/", "::bad"} {
		if got := absoluteFeedHTML(`<img src="/a.png">`, base); got != `<img src="/a.png">` {
			t.Errorf("base %q: %s", base, got)
		}
	}
}

// TestFullContentFeedsHaveAbsoluteURLs: RSS and Atom built end to end, a post
// with a root-relative image, a document-relative image and a link.
func TestFullContentFeedsHaveAbsoluteURLs(t *testing.T) {
	tmp := t.TempDir()
	writeTaxonomyMeta(t, tmp)
	mustWrite(t, filepath.Join(tmp, "content", "site", "posts", "news", "pipeline.md"),
		"---\ntitle: Pipeline\nslug: pipeline\nstatus: publish\ntype: post\ndate: 2025-01-02\n---\n\n"+
			"![diagram](/images/blog/pipeline.svg)\n\n![local](img/step.png)\n\nSee [the stats](/stats/).\n")
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	cfg := taxonomyTestConfig(tmp)
	cfg.FeedFullContent = true
	on := true
	cfg.Feeds = []models.FeedSpec{{Path: "/blog/rss.xml", Format: "rss", FullContent: &on}}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	postURL := "https://example.com/2025/01/02/pipeline/"
	for _, feed := range []string{"blog/rss.xml", "feed.xml"} {
		body := stdhtml.UnescapeString(mustRead(t, filepath.Join(cfg.OutputDir, feed)))
		for _, want := range []string{
			`src="https://example.com/images/blog/pipeline.svg"`,
			`src="` + postURL + `img/step.png"`,
			`href="https://example.com/stats/"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s lacks %s", feed, want)
			}
		}
		if strings.Contains(body, `src="/images`) || strings.Contains(body, `href="/stats/"`) {
			t.Errorf("%s still carries a relative URL", feed)
		}
	}
}
