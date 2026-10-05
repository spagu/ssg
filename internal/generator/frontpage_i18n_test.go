package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
)

// i18nFrontSite is a Hindi-first site with English and Polish under their
// prefixes, each leading with a page of its own (#319).
func i18nFrontSite(t *testing.T, postsPage string) Config {
	t.Helper()
	page := func(lang, link, title string) string {
		return "---\ntitle: " + title + "\nslug: home\nstatus: publish\ntype: page\nlang: " + lang +
			"\ntranslation_key: home\nlink: \"" + link + "\"\n---\n\n" + title + " body.\n"
	}
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"pages/home.hi.md": page("hi", "/", "Hindi home"),
		"pages/home.en.md": page("en", "/en/", "English home"),
		"pages/home.pl.md": page("pl", "/pl/", "Polish home"),
		"posts/news/hello.en.md": "---\ntitle: Hello\nslug: hello\nstatus: publish\ntype: post\nlang: en\n" +
			"date: 2026-10-01\n---\n\nHi.\n",
	}, func(name string) string {
		if name == "page.html" {
			return `<html><head><title>{{ .Page.Title }}</title></head><body>{{ .Page.Content | safeHTML }}</body></html>`
		}
		return `<html><head><title>listing</title></head><body><p>LISTING {{ len .Posts }}</p></body></html>`
	})
	cfg.DefaultLanguage = "hi"
	cfg.Languages = []string{"hi", "en", "pl"}
	cfg.LanguageConfigs = []ssgi18n.LanguageConfig{{Code: "hi", Name: "हिन्दी"}, {Code: "en", Name: "English"}, {Code: "pl", Name: "Polski"}}
	cfg.I18n = ssgi18n.Config{Enabled: true}
	cfg.PostsPage = postsPage
	return cfg
}

// TestEachLanguageLeadsWithItsPage: /en/ and /pl/ are the pages written for
// them, not the post listing; with posts_page the listing moves under each
// language's prefix.
func TestEachLanguageLeadsWithItsPage(t *testing.T) {
	for _, postsPage := range []string{"", "blog"} {
		cfg := i18nFrontSite(t, postsPage)
		buildSiteFixture(t, cfg)
		for dir, want := range map[string]string{"": "Hindi home", "en": "English home", "pl": "Polish home"} {
			got := mustRead(t, filepath.Join(cfg.OutputDir, dir, "index.html"))
			if !strings.Contains(got, want) || strings.Contains(got, "LISTING") {
				t.Errorf("posts_page=%q /%s/ = %s", postsPage, dir, got)
			}
		}
		listing := filepath.Join(cfg.OutputDir, "en", "blog", "index.html")
		_, err := os.Stat(listing)
		if postsPage == "blog" && err != nil {
			t.Errorf("the English listing moved to /en/blog/: %v", err)
		}
		if postsPage == "" && err == nil {
			t.Error("without posts_page there is no listing to move")
		}
	}
}

// TestTwoLanguagesAtTheSiteRoot: link: "/" on two languages is still a
// collision, and the error says how each names its own root.
func TestTwoLanguagesAtTheSiteRoot(t *testing.T) {
	cfg := i18nFrontSite(t, "")
	mustWrite(t, filepath.Join(cfg.ContentDir, "site", "pages", "home.en.md"),
		"---\ntitle: English home\nslug: home\nstatus: publish\ntype: page\nlang: en\nlink: \"/\"\n---\n\nX.\n")
	err := mustBuild(t, cfg)
	if err == nil || !strings.Contains(err.Error(), `link: "/en/"`) {
		t.Errorf("collision hint: %v", err)
	}
}

// TestListingKnowsItsLanguages (#321): a listing's context carries what a
// page's does — language, its own canonical per pager page, the listing in
// every language and the hreflang block built from them.
func TestListingKnowsItsLanguages(t *testing.T) {
	cfg := i18nFrontSite(t, "blog")
	mustWrite(t, filepath.Join(cfg.ContentDir, "site", "posts", "news", "second.en.md"),
		"---\ntitle: Second\nslug: second\nstatus: publish\ntype: post\nlang: en\ndate: 2026-10-02\n---\n\nTwo.\n")
	listing := `{{ .Lang }}|{{ .CanonicalURL }}|{{ .Description }}|` +
		`{{ range .Translations }}{{ .Lang }}={{ .URL }}{{ if .IsCurrent }}*{{ end }};{{ end }}|{{ .Hreflang }}`
	for _, name := range fixtureTemplates {
		if name == "index.html" {
			mustWrite(t, filepath.Join(cfg.TemplatesDir, cfg.Template, name),
				`{{define "index.html"}}<html><head><title>l</title></head><body><p>`+listing+`</p></body></html>{{end}}`)
		}
	}
	cfg.Paginate = 1
	buildSiteFixture(t, cfg)
	en := mustRead(t, filepath.Join(cfg.OutputDir, "en", "blog", "index.html"))
	for _, want := range []string{"<p>en|https://example.com/en/blog/|", "hi=/blog/;en=/en/blog/*;pl=/pl/blog/;",
		`hreflang="x-default" href="https://example.com/blog/"`, `hreflang="pl" href="https://example.com/pl/blog/"`} {
		if !strings.Contains(en, want) {
			t.Errorf("/en/blog/ lacks %s:\n%s", want, en)
		}
	}
	page2 := mustRead(t, filepath.Join(cfg.OutputDir, "en", "blog", "page", "2", "index.html"))
	if !strings.Contains(page2, "|https://example.com/en/blog/page/2/|") {
		t.Errorf("page 2 names itself:\n%s", page2)
	}
}
