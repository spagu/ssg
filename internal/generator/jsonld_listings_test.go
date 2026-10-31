package generator

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// ldOf returns every JSON-LD block of a page, decoded.
func ldOf(t *testing.T, html string) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	for _, raw := range strings.Split(html, `<script type="application/ld+json">`)[1:] {
		raw = raw[:strings.Index(raw, "</script>")]
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("invalid JSON-LD %q: %v", raw, err)
		}
		out = append(out, m)
	}
	return out
}

func typesOf(blocks []map[string]interface{}) string {
	var ts []string
	for _, b := range blocks {
		ts = append(ts, b["@type"].(string))
	}
	return strings.Join(ts, ",")
}

// TestListingStructuredData: with seo on, the front listing carries the
// Organization and an ItemList, an author archive is a ProfilePage with an
// ItemList, a category archive an ItemList; a post gets none of them.
func TestListingStructuredData(t *testing.T) {
	head := `<html><head><title>x</title></head><body><p>x</p></body></html>`
	cfg := newSiteFixture(t, `{"categories":[{"id":3,"name":"Cakes","slug":"cakes"}],"media":[],`+
		`"users":[{"id":1,"name":"Ann Baker","slug":"ann-baker"}]}`, map[string]string{
		"posts/news/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: post\ndate: 2026-07-01\nauthor: 1\ncategories: [3]\n---\n\nA.\n",
		"posts/news/b.md": "---\ntitle: B\nslug: b\nstatus: publish\ntype: post\ndate: 2026-07-02\nauthor: 1\ncategories: [3]\n---\n\nB.\n",
	}, func(name string) string {
		if name == "page.html" || name == "post.html" {
			return `<html><head><title>x</title><script type="application/ld+json">{"@type":"Thing"}</script></head><body>{{ .Content | safeHTML }}</body></html>`
		}
		return head
	})
	cfg.SEO = true
	cfg.Marketing = models.Marketing{Logo: "/logo.png", OGSiteName: "Bakery",
		SocialProfiles: map[string]string{"x": "https://x.example.com/bakery", "github": "https://github.example.com/bakery"}}
	buildSiteFixture(t, cfg)

	home := ldOf(t, mustRead(t, filepath.Join(cfg.OutputDir, "index.html")))
	if !strings.Contains(typesOf(home), "Organization") || !strings.Contains(typesOf(home), "ItemList") {
		t.Fatalf("home types = %s", typesOf(home))
	}
	for _, b := range home {
		switch b["@type"] {
		case "Organization":
			if b["name"] != "Bakery" || b["logo"] != "https://example.com/logo.png" || len(b["sameAs"].([]interface{})) != 2 {
				t.Errorf("organization = %v", b)
			}
		case "ItemList":
			if n := len(b["itemListElement"].([]interface{})); n != 2 {
				t.Errorf("home list has %d items", n)
			}
		}
	}
	author := ldOf(t, mustRead(t, filepath.Join(cfg.OutputDir, "author", "ann-baker", "index.html")))
	if typesOf(author) != "ProfilePage,ItemList" {
		t.Fatalf("author types = %s", typesOf(author))
	}
	person := author[0]["mainEntity"].(map[string]interface{})
	if person["name"] != "Ann Baker" || person["url"] != "https://example.com/author/ann-baker/" {
		t.Errorf("person = %v", person)
	}
	cat := ldOf(t, mustRead(t, filepath.Join(cfg.OutputDir, "category", "cakes", "index.html")))
	if typesOf(cat) != "ItemList" {
		t.Errorf("category types = %s", typesOf(cat))
	}
	post := mustRead(t, filepath.Join(cfg.OutputDir, "2026", "07", "01", "a", "index.html"))
	if strings.Contains(post, `"@type":"ItemList"`) || strings.Contains(post, `"@type":"Organization"`) {
		t.Error("a post gets no listing blocks")
	}
}

func TestStructuredDataHelpers(t *testing.T) {
	blocks := []map[string]interface{}{{"@type": "Thing"}}
	if structuredDataOf(map[string]interface{}{structuredDataKey: blocks}) == nil ||
		structuredDataOf(struct{ StructuredData []map[string]interface{} }{blocks}) == nil ||
		structuredDataOf(&struct{ StructuredData []map[string]interface{} }{blocks}) == nil ||
		structuredDataOf(struct{ Other int }{}) != nil || structuredDataOf(42) != nil {
		t.Error("structuredDataOf")
	}
	g := &Generator{config: Config{SEO: true}}
	html := "<html><head></head><body></body></html>"
	if got := g.injectStructuredData(html, blocks, false); !strings.Contains(got, `{"@type":"Thing"}</script>`+"\n</head>") {
		t.Errorf("inject: %s", got)
	}
	if g.injectStructuredData(html, blocks, true) != html || g.injectStructuredData("<p>no head</p>", blocks, false) != "<p>no head</p>" {
		t.Error("theme JSON-LD or no head: unchanged")
	}
	g.config.SEO = false
	if g.injectStructuredData(html, blocks, false) != html {
		t.Error("seo off: unchanged")
	}
	p := personLD("Ann", "", " ", "https://example.com/a.jpg", []string{"https://a.example.com", "https://a.example.com", ""})
	if p["image"] == nil || p["description"] != nil || len(p["sameAs"].([]interface{})) != 1 {
		t.Errorf("person = %v", p)
	}
	if itemListLD(nil) != nil || profilePageLD("", p)["url"] != nil {
		t.Error("empty list / url")
	}
	g = &Generator{config: Config{Domain: "example.com", BasePath: "/blog"}, siteData: &models.SiteData{}}
	for in, want := range map[string]string{"": "", "https://cdn.example.com/l.png": "https://cdn.example.com/l.png",
		"logo.png": "https://example.com/logo.png"} {
		if got := g.absoluteURL(in); got != want {
			t.Errorf("absoluteURL(%q) = %q", in, got)
		}
	}
	if (&Generator{config: Config{}}).absoluteURL("/x") != "" {
		t.Error("no domain, no absolute URL")
	}
	if g.organizationLD() != nil {
		t.Error("no marketing, no Organization")
	}
}

// TestThemeLDKeepsListingsOut: an archive whose theme writes its own JSON-LD
// gets no blocks injected — the theme can add .StructuredData itself.
func TestThemeLDKeepsListingsOut(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[{"id":3,"name":"Cakes","slug":"cakes"}],"media":[],"users":[]}`, map[string]string{
		"posts/news/a.md": "---\ntitle: A\nslug: a\nstatus: publish\ntype: post\ndate: 2026-07-01\ncategories: [3]\n---\n\nA.\n",
	}, func(name string) string {
		if name == "category.html" {
			return `<html><head><title>x</title><script type="application/ld+json">{"@type":"Thing"}</script></head><body>{{ len .StructuredData }}</body></html>`
		}
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	})
	cfg.SEO = true
	buildSiteFixture(t, cfg)
	got := mustRead(t, filepath.Join(cfg.OutputDir, "category", "cakes", "index.html"))
	if strings.Count(got, "application/ld+json") != 1 || !strings.Contains(got, "<body>1") {
		t.Errorf("theme JSON-LD wins, .StructuredData still offered:\n%s", got)
	}
}
