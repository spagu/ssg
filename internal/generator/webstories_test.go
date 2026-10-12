package generator

// Web stories, piece by piece: the AMP shape, selection, captions, the
// references a picture can be written as, and the slot.

import (
	"regexp"
	"strings"
	"testing"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// ampBoilerplate is the exact style AMP requires in every document.
const ampBoilerplate = `<style amp-boilerplate>body{-webkit-animation:-amp-start 8s steps(1,end) 0s 1 normal both;-moz-animation:-amp-start 8s steps(1,end) 0s 1 normal both;-ms-animation:-amp-start 8s steps(1,end) 0s 1 normal both;animation:-amp-start 8s steps(1,end) 0s 1 normal both}@-webkit-keyframes -amp-start{from{visibility:hidden}to{visibility:visible}}@-moz-keyframes -amp-start{from{visibility:hidden}to{visibility:visible}}@-ms-keyframes -amp-start{from{visibility:hidden}to{visibility:visible}}@-o-keyframes -amp-start{from{visibility:hidden}to{visibility:visible}}@keyframes -amp-start{from{visibility:hidden}to{visibility:visible}}</style><noscript><style amp-boilerplate>body{-webkit-animation:none;-moz-animation:none;-ms-animation:none;animation:none}</style></noscript>`

var (
	storyScriptRe = regexp.MustCompile(`<script\b[^>]*>`)
	ampCustomRe   = regexp.MustCompile(`(?s)<style amp-custom>(.*?)</style>`)
	ampImgRe      = regexp.MustCompile(`<amp-img\b[^>]*>`)
	allowedAMPJS  = map[string]bool{
		`<script async src="https://cdn.ampproject.org/v0.js">`:                                          true,
		`<script async custom-element="amp-story" src="https://cdn.ampproject.org/v0/amp-story-1.0.js">`: true,
	}
)

// assertAMPStory checks what the AMP validator rejects most often.
func assertAMPStory(t *testing.T, s string) {
	t.Helper()
	for _, want := range []string{"<!doctype html>", `<html amp lang="`, `<meta charset="utf-8">`, ampBoilerplate,
		`<meta name="viewport" content="width=device-width`, `<link rel="canonical" href="`,
		`<amp-story standalone title="`, ` publisher="`, ` publisher-logo-src="`, ` poster-portrait-src="`,
		`<amp-story-page-outlink layout="nodisplay">`} {
		if !strings.Contains(s, want) {
			t.Errorf("AMP story lacks %q", want)
		}
	}
	for _, tag := range storyScriptRe.FindAllString(s, -1) {
		if !allowedAMPJS[tag] {
			t.Errorf("disallowed script %s", tag)
		}
	}
	if strings.Contains(s, " style=") {
		t.Error("inline style attribute in an AMP story")
	}
	styles := ampCustomRe.FindAllStringSubmatch(s, -1)
	if len(styles) != 1 || len(styles[0][1]) > 75000 {
		t.Errorf("amp-custom: %d block(s)", len(styles))
	}
	for _, img := range ampImgRe.FindAllString(s, -1) {
		for _, attr := range []string{" width=", " height=", ` layout="responsive"`, " alt="} {
			if !strings.Contains(img, attr) {
				t.Errorf("%s lacks %s", img, attr)
			}
		}
	}
}

// storyGen is a bare generator with web stories on.
func storyGen(t *testing.T) *Generator {
	t.Helper()
	g := newTestGen(t, "")
	g.config.WebStories = WebStoriesOptions{Enabled: true}
	return g
}

func TestStoryOptionDefaults(t *testing.T) {
	o := withStoryDefaults(WebStoriesOptions{})
	if o.URL != "/web-stories/" || o.Pages != 4 || o.MinImages != 2 {
		t.Errorf("defaults = %+v", o)
	}
	for in, want := range map[string]string{"stories": "/stories/", "/a/b": "/a/b/", "../../x": "/x/", " / ": "/web-stories/"} {
		if got := normalizeStoriesURL(in); got != want {
			t.Errorf("normalizeStoriesURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStoryCandidates(t *testing.T) {
	g := storyGen(t)
	g.config.ContentDir, g.config.Source = "content", "site"
	g.siteData.Posts = []models.Page{
		{Title: "Cake", Slug: "cake", Type: "post", URLFormat: "slug", SourceDir: "content/site/posts/recipes"},
		{Title: "News", Slug: "news", Type: "post", URLFormat: "slug", SourceDir: "content/site/posts/news"},
	}
	g.siteData.Pages = []models.Page{{Title: "About", Slug: "about", Type: "page"}}
	count := func(sections ...string) int { return len(g.storyCandidates(sections)) }
	if count() != 2 || count("pages") != 1 || count("posts/recipes") != 1 || count("posts", "posts/recipes", "page") != 3 || count("") != 0 {
		t.Errorf("selection: default=%d pages=%d recipes=%d all=%d", count(), count("pages"), count("posts/recipes"), count("posts", "posts/recipes", "page"))
	}
}

func TestStoryCaptions(t *testing.T) {
	g := storyGen(t)
	p := models.Page{Title: "Bread", Tags: []string{" ", "rye", "sourdough"}, Excerpt: "<p>A slow loaf &amp; a crisp crust.</p>",
		Schema: map[string]interface{}{"@type": []interface{}{"Recipe"}, "cookTime": "PT45M", "recipeYield": []interface{}{"makes 2 loaves"}}}
	got := strings.Join(g.storyCaptions(p, true, "en"), " | ")
	if got != "rye · sourdough | Cooks in 45 min | Makes 2 loaves | A slow loaf & a crisp crust." {
		t.Errorf("captions = %q", got)
	}
	if c := g.storyCaptions(models.Page{Title: "x"}, false, "en"); len(c) != 0 {
		t.Errorf("an empty page has captions %q", c)
	}
	for in, want := range map[interface{}]string{"PT1H25M": "1 hr 25 min", "pt0m": "", "soon": "", nil: "", "P1DT2H": "1 d 2 hr"} {
		if got := storyDuration("en", in); got != want {
			t.Errorf("storyDuration(%v) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[interface{}]string{"16": "Makes 16 servings", 8.0: "Makes 8 servings", "1 loaf": "Makes 1 loaf", "": ""} {
		if got := g.storyYield("en", in); got != want {
			t.Errorf("storyYield(%v) = %q, want %q", in, got, want)
		}
	}
	if g.storyYield("en", []interface{}{}) != "" {
		t.Error("an empty list yields nothing")
	}
}

func TestStoryTextAndLabels(t *testing.T) {
	g := storyGen(t)
	g.catalog = &ssgi18n.Catalog{Messages: map[string]map[string]any{"pl": {"web_stories": map[string]any{
		"makes": "Wychodzi {{yield}}", "cta": "Czytaj dalej", "hub_title": "Relacje"}}}}
	if got := g.storyYield("pl", "12 porcji"); got != "Wychodzi 12 porcji" {
		t.Errorf("catalog = %q", got)
	}
	if g.storyCTALabel("pl") != "Czytaj dalej" || g.storyCTALabel("en") != "Read the full article" {
		t.Error("CTA label from the catalog")
	}
	g.config.WebStories.CTA = "Get the recipe"
	if g.storyCTALabel("pl") != "Get the recipe" {
		t.Error("the configured CTA wins")
	}
	long := models.Page{Description: strings.Repeat("word ", 60)}
	if ex := storyExcerpt(long, 20); ex != "word word word word…" {
		t.Errorf("excerpt = %q", ex)
	}
	if g.storyDescription(models.Page{Title: "T"}) != "T" {
		t.Error("description falls back to the title")
	}
}

func TestStorySiteFacts(t *testing.T) {
	g := storyGen(t)
	if g.storyLang(models.Page{Lang: "pl"}) != "pl" || g.storyLang(models.Page{}) != "en" {
		t.Error("story language")
	}
	g.config.DefaultLanguage = "de"
	if g.storyLang(models.Page{}) != "de" {
		t.Error("default language")
	}
	g.siteData.Title = ""
	if g.storyPublisher() != "example.com" {
		t.Errorf("publisher = %q", g.storyPublisher())
	}
	g.config.Domain = ""
	if g.storyPublisher() != "ssg" {
		t.Error("last-resort publisher")
	}
	g.config.BasePath = "/blog"
	if g.storyHref("/x/") != "/blog/x/" || g.storyArticleURL(models.Page{Slug: "a", Type: "page"}) != "/blog/a/" {
		t.Errorf("hrefs without a domain: %q", g.storyHref("/x/"))
	}
	g.recordStorySitemap(&webStory{path: "/web-stories/a/"})
	if len(g.staticSitemap) != 0 {
		t.Error("no sitemap entry without a domain")
	}
	if storySourceFile(models.Page{}) != "" || storySourceFile(models.Page{SourceDir: "d", SourceFile: "f.md"}) == "" {
		t.Error("storySourceFile")
	}
	if p := g.storyPath(models.Page{Title: "Hello There", Slug: "/", LangPrefix: "pl"}); p != "/pl/web-stories/hello-there/" {
		t.Errorf("storyPath = %q", p)
	}
}

func TestStoryImageReferences(t *testing.T) {
	g := storyGen(t)
	g.config.Domain, g.config.BasePath = "www.example.com", "/blog"
	cases := map[string]string{"data:image/png;base64,x": "", "https://cdn.other.com/a.jpg": "", "": "",
		"https://example.com/blog/media/a.jpg?w=1#x": "/media/a.jpg", "pic.jpg": "pic.jpg", "%zz": "", "https://example.com": ""}
	for in, want := range cases {
		got, ok := g.localImagePath(in)
		if got != want || ok != (want != "") {
			t.Errorf("localImagePath(%q) = %q, %v", in, got, ok)
		}
	}
	g.config.ContentDir, g.config.Source = "content", "site"
	got := g.storyImageCandidates("shot-300x200.jpg", models.Page{SourceDir: "content/site/posts/x"})
	if strings.Join(got, ",") != "posts/x/shot.jpg,posts/x/shot-300x200.jpg,shot.jpg,shot-300x200.jpg" {
		t.Errorf("candidates = %v", got)
	}
	if g.storyImageCandidates("data:x", models.Page{}) != nil || g.storyImageSource("missing.png", models.Page{}) != "" {
		t.Error("nothing to resolve")
	}
}
