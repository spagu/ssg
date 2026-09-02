package generator

// The front page may be written by exactly one document (#234). GO-023 guarded
// posts; #129 removed the page-side guard so a page could BE the front page —
// and with it went the protection against a second page landing on the root.

import (
	"github.com/spagu/ssg/internal/models"
	"path/filepath"
	"strings"
	"testing"
)

func buildFrontPageSite(t *testing.T, pages map[string]string) (outDir string) {
	t.Helper()
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content")
	tmplDir := filepath.Join(tmp, "templates")
	outDir = filepath.Join(tmp, "output")

	mustWrite(t, filepath.Join(contentDir, "site", "metadata.json"),
		`{"categories":[],"media":[],"users":[]}`)
	for name, body := range pages {
		mustWrite(t, filepath.Join(contentDir, "site", "pages", name), body)
	}
	for _, name := range []string{"base.html", "index.html", "post.html", "page.html",
		"category.html", "tag.html", "taxonomy.html"} {
		mustWrite(t, filepath.Join(tmplDir, "simple", name),
			`{{define "`+name+`"}}<html><body>{{.Content}}</body></html>{{end}}`)
	}
	gen, err := New(Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: contentDir, TemplatesDir: tmplDir, OutputDir: outDir, Quiet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return outDir
}

// TestQueryLinkedPagesDoNotOverwriteTheFrontPage is the issue's reproduction:
// a real front page plus two WordPress custom-type exports whose links carry
// their meaning in the query string. Before the fix the build finished green
// and the site led with a stray gallery.
func TestQueryLinkedPagesDoNotOverwriteTheFrontPage(t *testing.T) {
	out := buildFrontPageSite(t, map[string]string{
		"home.md": "---\ntitle: Welcome\nslug: home\nstatus: publish\nlink: \"/\"\n---\n\nThe real front page.\n",
		"g1.md":   "---\ntitle: lifeatead\nslug: \"1289\"\nstatus: publish\nlink: \"/?modula-gallery=1289\"\n---\n\nGallery one.\n",
		"g2.md":   "---\ntitle: teamday\nslug: \"1876\"\nstatus: publish\nlink: \"/?modula-gallery=1876\"\n---\n\nGallery two.\n",
	})

	front := mustRead(t, filepath.Join(out, indexHTMLName))
	if !strings.Contains(front, "The real front page.") {
		t.Errorf("index.html does not hold the front page; it holds: %.120s", front)
	}
	if strings.Contains(front, "Gallery") {
		t.Error("a query-linked page overwrote the front page")
	}
	// The galleries are content somebody migrated; they must land somewhere,
	// not vanish — the slug-derived address is that somewhere.
	for slug, want := range map[string]string{"1289": "Gallery one.", "1876": "Gallery two."} {
		doc := mustRead(t, filepath.Join(out, slug, indexHTMLName))
		if !strings.Contains(doc, want) {
			t.Errorf("/%s/ = %.80q, want %q", slug, doc, want)
		}
	}
}

// TestSecondRootClaimantIsSkippedNotSilentlyLast: two pages both saying
// link: "/" is an authoring mistake; the build must pick the same page the
// front-page report names (the first) and refuse the other loudly — not let
// render order decide which document a site leads with.
func TestSecondRootClaimantIsSkippedNotSilentlyLast(t *testing.T) {
	out := buildFrontPageSite(t, map[string]string{
		"aaa-home.md":  "---\ntitle: Real\nslug: home\nstatus: publish\nlink: \"/\"\n---\n\nThe designated front.\n",
		"zzz-later.md": "---\ntitle: Impostor\nslug: later\nstatus: publish\nlink: \"/\"\n---\n\nThe impostor.\n",
	})
	front := mustRead(t, filepath.Join(out, indexHTMLName))
	if !strings.Contains(front, "The designated front.") {
		t.Errorf("index.html = %.120q, want the first claimant — the one the build reported", front)
	}
	if strings.Contains(front, "Impostor") || strings.Contains(front, "impostor") {
		t.Error("the later claimant overwrote the designated front page")
	}
}

// TestDesignationIsPerLanguage: with i18n on, each language has its own front
// page, and a root claim in one language must not disqualify another's.
func TestDesignationIsPerLanguage(t *testing.T) {
	en := models.Page{Title: "Home", Slug: "home", Link: "/", Lang: "en", SourceFile: "en/home.md"}
	pl := models.Page{Title: "Dom", Slug: "dom", Link: "/", Lang: "pl", SourceFile: "pl/dom.md"}
	extra := models.Page{Title: "Extra", Slug: "extra", Link: "/", Lang: "en", SourceFile: "en/extra.md"}
	g := &Generator{siteData: &models.SiteData{Pages: []models.Page{en, pl, extra}}}
	g.config.I18n.Enabled = true

	if !g.isDesignatedFrontPage(en) {
		t.Error("the first English claimant is the English front page")
	}
	if !g.isDesignatedFrontPage(pl) {
		t.Error("the first Polish claimant is the Polish front page")
	}
	if g.isDesignatedFrontPage(extra) {
		t.Error("a second English claimant must not be designated")
	}
}
