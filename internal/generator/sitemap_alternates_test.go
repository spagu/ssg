package generator

// Language alternates for the front page and the post listing (#281).

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
)

// multilingualSitemap builds an en/pl/ro site whose listing lives at /blog/.
// withFrontPages adds a translated front page per language, each with the
// explicit link a language-prefixed front page needs.
func multilingualSitemap(t *testing.T, withFrontPages bool) string {
	t.Helper()
	files := map[string]string{}
	for _, lang := range []string{"en", "pl", "ro"} {
		files["posts/news/p."+lang+".md"] = "---\ntitle: P " + lang + "\nslug: p\nlang: " + lang +
			"\ntranslation_key: p\nstatus: publish\ntype: post\ndate: 2026-01-02\n---\n\nBody.\n"
		if withFrontPages {
			link := "/"
			if lang != "en" {
				link = "/" + lang + "/"
			}
			files["pages/home."+lang+".md"] = "---\ntitle: Home " + lang + "\nlang: " + lang +
				"\ntranslation_key: home\nlink: " + link + "\nstatus: publish\n---\n\nFront.\n"
		}
	}
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, nil)
	cfg.PostsPage = "blog"
	cfg.DefaultLanguage = "en"
	cfg.Languages = []string{"en", "pl", "ro"}
	cfg.I18n = ssgi18n.Config{Enabled: true}
	buildSiteFixture(t, cfg)
	return mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
}

// urlBlock returns the <url> element whose <loc> is loc.
func urlBlock(t *testing.T, sitemap, loc string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)<url>\s*<loc>` + regexp.QuoteMeta(loc) + `</loc>.*?</url>`)
	block := re.FindString(sitemap)
	if block == "" {
		t.Fatalf("no entry for %s in:\n%s", loc, sitemap)
	}
	return block
}

// wantAlternates checks one entry names all three languages and x-default.
func wantAlternates(t *testing.T, block string, hrefs map[string]string) {
	t.Helper()
	for lang, href := range hrefs {
		want := `hreflang="` + lang + `" href="` + href + `"`
		if !strings.Contains(block, want) {
			t.Errorf("missing %s in:\n%s", want, block)
		}
	}
	if !strings.Contains(block, `hreflang="x-default" href="`+hrefs["en"]+`"`) {
		t.Errorf("missing x-default in:\n%s", block)
	}
}

// TestFrontPagesCarryTheirAlternates: the sitemap says what the front page's
// HTML already said, for the page that resolved at each address.
func TestFrontPagesCarryTheirAlternates(t *testing.T) {
	homes := map[string]string{"en": "https://example.com/", "pl": "https://example.com/pl/", "ro": "https://example.com/ro/"}
	for _, withPages := range []bool{true, false} {
		sitemap := multilingualSitemap(t, withPages)
		for _, loc := range homes {
			wantAlternates(t, urlBlock(t, sitemap, loc), homes)
		}
	}
}

// TestPostListingsAreEachOthersAlternates: /blog/, /pl/blog/ and /ro/blog/ are
// generated rather than authored, but translations of one another all the same.
func TestPostListingsAreEachOthersAlternates(t *testing.T) {
	sitemap := multilingualSitemap(t, true)
	listings := map[string]string{"en": "https://example.com/blog/", "pl": "https://example.com/pl/blog/", "ro": "https://example.com/ro/blog/"}
	for _, loc := range listings {
		wantAlternates(t, urlBlock(t, sitemap, loc), listings)
	}
}

// TestAGroupOfOneHasNoAlternates: a lone listing or front page is not an
// alternate of itself, and a site without i18n gets no xhtml:link at all.
func TestAGroupOfOneHasNoAlternates(t *testing.T) {
	g := newTestGen(t, "")
	if got := g.groupAlternates([]alternateLink{{lang: "en", href: "https://example.com/"}}); got != "" {
		t.Errorf("without i18n: %q", got)
	}
	g.config.I18n.Enabled = true
	if got := g.groupAlternates([]alternateLink{{lang: "en", href: "https://example.com/"}}); got != "" {
		t.Errorf("a group of one: %q", got)
	}
	if got := g.homeAlternates(alternateLink{href: "https://example.com/"}, nil); got != "" {
		t.Errorf("no languages: %q", got)
	}
	g.config.I18n.Enabled = false
	if got := g.homeAlternates(alternateLink{href: "https://example.com/"}, nil); got != "" {
		t.Errorf("home without i18n: %q", got)
	}
}
