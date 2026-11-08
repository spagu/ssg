package generator

// markdown_publish and the pages a site keeps out of search (#393): llms.txt
// advertised noindex pages and the 404 page, each with a Markdown copy and an
// alternate link, to the agents the site meant to point at its real content.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestHiddenPagesGetNoMarkdownCopy builds a site with a public page, a
// noindex page, a sitemap: no page, the 404 page and a noindex page that asks
// for its copy itself.
func TestHiddenPagesGetNoMarkdownCopy(t *testing.T) {
	tmp := t.TempDir()
	pages := filepath.Join(tmp, "content", "site", "pages")
	page := func(slug, extra string) {
		mustWrite(t, filepath.Join(pages, slug+".md"),
			"---\ntitle: "+slug+"\nslug: "+slug+"\nstatus: publish\n"+extra+"---\n\nBody of "+slug+".\n")
	}
	page("about", "")
	page("archive", "robots: \"noindex, follow\"\n")
	page("thanks", "sitemap: \"no\"\n")
	page("404", "")
	page("backup", "robots: noindex\noutputs: [html, markdown]\n")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	cfg := taxonomyTestConfig(tmp)
	cfg.Domain = "example.com"
	cfg.MarkdownPublish = true
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := filepath.Join(tmp, "output")
	llms := mustRead(t, filepath.Join(out, "llms.txt"))

	if !strings.Contains(llms, "/about/index.md") {
		t.Errorf("the public page is missing from llms.txt:\n%s", llms)
	}
	if _, err := os.Stat(filepath.Join(out, "about", "index.md")); err != nil {
		t.Error("the public page lost its copy")
	}
	for _, hidden := range []string{"archive", "thanks", "404"} {
		if strings.Contains(llms, "/"+hidden+"/") || strings.Contains(llms, "/"+hidden+".md") {
			t.Errorf("llms.txt lists %s:\n%s", hidden, llms)
		}
		for _, name := range []string{filepath.Join(hidden, "index.md"), hidden + ".md"} {
			if _, err := os.Stat(filepath.Join(out, name)); err == nil {
				t.Errorf("%s was written", name)
			}
		}
	}
	if html := mustRead(t, filepath.Join(out, "archive", "index.html")); strings.Contains(html, "text/markdown") {
		t.Error("a noindex page advertises a Markdown copy")
	}
	// Its own outputs: list asked, so the copy is written; it is still not
	// advertised to agents, since the page asked crawlers to skip it.
	if _, err := os.Stat(filepath.Join(out, "backup", "index.md")); err != nil {
		t.Error("a page's own outputs: [markdown] was ignored")
	}
	if strings.Contains(llms, "/backup/") {
		t.Errorf("llms.txt lists a noindex page:\n%s", llms)
	}
}

// TestPublishesMarkdownWithoutRegistry: a bare Generator still applies the rule.
func TestPublishesMarkdownWithoutRegistry(t *testing.T) {
	g := &Generator{config: Config{MarkdownPublish: true}}
	if !g.publishesMarkdown(models.Page{Slug: "a"}) || g.publishesMarkdown(models.Page{Slug: "b", Robots: "noindex"}) {
		t.Error("bare generator")
	}
	g.config.MarkdownPublish = false
	if g.publishesMarkdown(models.Page{Slug: "a"}) {
		t.Error("markdown_publish off")
	}
}
