package generator

// slug_transliterate (#426): off, every URL is what it was; on, label slugs
// fold diacritics and each moved archive answers its old URL with a 301.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/externalsource"
	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/taxonomy"
)

// buildTranslitSite builds one post with a Polish tag, a French series, a
// German technology term, a Russian tag and an author named in Polish.
func buildTranslitSite(t *testing.T, on bool) Config {
	t.Helper()
	files := map[string]string{
		"posts/news/one.md": "---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2026-01-01\nauthor: 1\n" +
			"tags: [\"Zażółć gęślą\", Go, Привет]\nseries: Crème brûlée\ntechnology: [Straße]\n---\n\nBody.\n",
	}
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[{"id":1,"name":"Łukasz Żak"}]}`, files, nil)
	cfg.SlugTransliterate = on
	cfg.Taxonomies = map[string]taxonomy.DefinitionConfig{"technology": {}}
	buildSiteFixture(t, cfg)
	return cfg
}

// moved is each archive: its plain slug and its transliterated one.
var moved = []struct{ base, old, now string }{
	{"tag", "za-g-l", "zazolc-gesla"},
	{"series", "cr-me-br-l-e", "creme-brulee"},
	{"technology", "stra-e", "strasse"},
	{"author", "ukasz-ak", "lukasz-zak"},
}

func exists(cfg Config, rel string) bool {
	_, err := os.Stat(filepath.Join(cfg.OutputDir, filepath.FromSlash(rel), indexHTMLName))
	return err == nil
}

func TestSlugTransliterateOffKeepsURLs(t *testing.T) {
	cfg := buildTranslitSite(t, false)
	redirects := mustRead(t, filepath.Join(cfg.OutputDir, "_redirects"))
	for _, m := range moved {
		if !exists(cfg, m.base+"/"+m.old) || exists(cfg, m.base+"/"+m.now) {
			t.Errorf("/%s/: want the plain slug %q only", m.base, m.old)
		}
	}
	if redirects != redirectsFileHeader {
		t.Errorf("off must add no redirect:\n%s", redirects)
	}
}

func TestSlugTransliterateOnMovesAndRedirects(t *testing.T) {
	cfg := buildTranslitSite(t, true)
	redirects := mustRead(t, filepath.Join(cfg.OutputDir, "_redirects"))
	for _, m := range moved {
		if exists(cfg, m.base+"/"+m.old) || !exists(cfg, m.base+"/"+m.now) {
			t.Errorf("/%s/: want the folded slug %q only", m.base, m.now)
		}
		rule := "/" + m.base + "/" + m.old + "/ /" + m.base + "/" + m.now + "/ 301\n"
		if !strings.Contains(redirects, rule) {
			t.Errorf("missing %q in\n%s", rule, redirects)
		}
	}
	if !exists(cfg, "tag/go") || strings.Contains(redirects, "/tag/go/") {
		t.Error("an ASCII tag must neither move nor be redirected")
	}
	if n := strings.Count(redirects, " 301\n"); n != len(moved) {
		t.Errorf("%d rules, want %d (a Cyrillic tag has no slug either way):\n%s", n, len(moved), redirects)
	}
}

func TestRedirectMovedSlugsSkipsGuesses(t *testing.T) {
	g := &Generator{config: Config{SlugTransliterate: true}}
	g.redirectMovedSlugs("/tag/", map[string]string{
		"Łza":      "lza",    // moved: "za" → "lza"
		"Żaba":     "zaba",   // old "aba", but "aba" is live below
		"Aba":      "aba",    // ASCII, live
		"Ćma":      "cma",    // old "ma" …
		"Ńma":      "nma",    // … shared with this one: neither is redirected
		"Ósemka":   "custom", // slug not made from the label
		"Привет":   "",       // no slug at all
		"Straße 2": "strasse-2",
	})
	got := map[string]string{}
	for _, r := range g.aliasRedirects {
		got[r.From] = r.To
	}
	want := map[string]string{"/tag/za/": "/tag/lza/", "/tag/stra-e-2/": "/tag/strasse-2/"}
	if len(got) != len(want) {
		t.Fatalf("rules %v, want %v", got, want)
	}
	for from, to := range want {
		if got[from] != to {
			t.Errorf("%s → %q, want %q", from, got[from], to)
		}
	}
	off := &Generator{}
	off.redirectMovedSlugs("tag", map[string]string{"Łza": "lza"})
	if len(off.aliasRedirects) != 0 {
		t.Error("the switch off must record nothing")
	}
}

func TestDerivedTagSlugsDropsCanonicalSlugs(t *testing.T) {
	g := &Generator{siteData: &models.SiteData{TagSlugs: map[string]string{"café": "cafe"}}}
	got := g.derivedTagSlugs(map[string]string{"Café": "cafe", "Łza": "lza"})
	if len(got) != 1 || got["Łza"] != "lza" {
		t.Errorf("derivedTagSlugs = %v", got)
	}
}

func TestAliasLegacySlug(t *testing.T) {
	g := &Generator{config: Config{}}
	plain := models.Page{Slug: "strasse", Type: "page"}
	g.aliasLegacySlug(&plain, "")
	if len(plain.Aliases) != 0 {
		t.Error("a page without a legacy slug gets no alias")
	}

	p := models.Page{Slug: "strasse", LegacySlug: "stra-e", Type: "page"}
	g.aliasLegacySlug(&p, "")
	if len(p.Aliases) != 1 || p.Aliases[0] != "/stra-e/" {
		t.Errorf("aliases %v, want [/stra-e/]", p.Aliases)
	}

	post := models.Page{Slug: "strasse", LegacySlug: "stra-e", Type: "post"}
	g.aliasLegacySlug(&post, "/blog/:slug/")
	if len(post.Aliases) != 1 || post.Aliases[0] != "/blog/stra-e/" {
		t.Errorf("permalink aliases %v, want [/blog/stra-e/]", post.Aliases)
	}

	g.config.I18n.Enabled = true
	de := models.Page{Slug: "strasse", LegacySlug: "stra-e", Type: "page", LangPrefix: "de"}
	g.aliasLegacySlug(&de, "")
	if len(de.Aliases) != 1 || de.Aliases[0] != "/stra-e/" {
		t.Errorf("i18n aliases %v, want the path without the prefix writeAliasStubs adds", de.Aliases)
	}

	linked := models.Page{Slug: "strasse", LegacySlug: "stra-e", Type: "page", Link: "/fixed/"}
	g.aliasLegacySlug(&linked, "")
	if len(linked.Aliases) != 0 {
		t.Error("a page whose URL is its link did not move")
	}
}

// TestSlugTransliterateRecordPageKeepsItsOldURL: a page made from a record
// moves to the folded slug, and its old URL becomes an alias — a 301 in
// _redirects and a stub where the host does not read that file.
func TestSlugTransliterateRecordPageKeepsItsOldURL(t *testing.T) {
	cfg := mappedTaxonomySite(t, `[{"origin":"Crème brûlée","slug":"x","maintainerTerm":""}]`)
	src := cfg.ExternalSources.Sources["ports"]
	src.ContentMap = externalsource.ContentMap{Title: "origin", Type: "=page"}
	cfg.ExternalSources.Sources["ports"] = src
	cfg.SlugTransliterate = true
	buildSiteFixture(t, cfg)
	if !exists(cfg, "creme-brulee") {
		t.Fatal("the record page is not at its folded slug")
	}
	if !exists(cfg, "cr-me-br-l-e") {
		t.Error("no stub at the old URL")
	}
	redirects := mustRead(t, filepath.Join(cfg.OutputDir, "_redirects"))
	if !strings.Contains(redirects, "/cr-me-br-l-e /creme-brulee/ 301\n") {
		t.Errorf("no 301 from the old URL:\n%s", redirects)
	}
}
