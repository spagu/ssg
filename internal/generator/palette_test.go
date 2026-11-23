package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func TestApplySiteIdentity_ConfigWinsExportFillsIn(t *testing.T) {
	meta := models.Metadata{
		Site:      models.SiteInfo{Name: "Exported Name", Description: "Exported tagline"},
		Marketing: models.Marketing{Colors: map[string]string{"primary": "#7b2ff7"}},
	}

	// Nothing configured: the export supplies everything.
	site := &models.SiteData{}
	applySiteIdentity(site, meta, Config{})
	if site.Title != "Exported Name" || site.Description != "Exported tagline" {
		t.Errorf("export should fill an empty config: %+v", site)
	}
	if site.Colors["primary"] != "#7b2ff7" {
		t.Errorf("export palette should apply: %+v", site.Colors)
	}

	// Configured: the author's values win, key by key.
	site = &models.SiteData{}
	applySiteIdentity(site, meta, Config{
		Title:  "My Title",
		Colors: map[string]string{"primary": "#000000"},
	})
	if site.Title != "My Title" {
		t.Errorf("config title must win, got %q", site.Title)
	}
	if site.Description != "Exported tagline" {
		t.Errorf("an unset key still falls back to the export, got %q", site.Description)
	}
	if site.Colors["primary"] != "#000000" {
		t.Errorf("config palette must win, got %+v", site.Colors)
	}
}

// identityTemplates prints the site identity on the home page and gives every
// other page a usable title and description, so check_meta: strict judges the
// home page alone.
func identityTemplates(name string) string {
	if name == "index.html" {
		return `<html><head><title>The example project home page</title>` +
			`<meta name="description" content="{{.Description}}">` +
			`<meta name="x-site" content="{{.Site.Title}}|{{.Site.Description}}"></head><body>home</body></html>`
	}
	return `<html><head><title>{{if .Page}}{{.Page.Title}}{{end}} on the example project site</title>` +
		`<meta name="description" content="{{if .Page}}{{.Page.Description}}{{end}}"></head><body>x</body></html>`
}

// buildIdentitySite builds the identity fixture and returns the home page.
func buildIdentitySite(t *testing.T, cfg Config) string {
	t.Helper()
	cfg.CheckMeta = "strict"
	buildSiteFixture(t, cfg)
	home, err := os.ReadFile(filepath.Join(cfg.OutputDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(home)
}

// TestSiteIdentityReachesHomeOnEveryPath — #440. `title:` and `description:`
// were applied only while reading metadata.json, so a site built from
// content_sources alone (which never reads that file) rendered an empty
// .Description and .Site.Description and failed check_meta: strict.
func TestSiteIdentityReachesHomeOnEveryPath(t *testing.T) {
	const doc = "---\ntitle: Getting started with the example project\n" +
		"description: How to install and run the example project, step by step, for new users.\n" +
		"status: publish\n---\n\nHello.\n"
	const exported = `{"site":{"name":"Exported Name","description":"Exported tagline from the migration of the example site."}}`
	cases := []struct {
		name, metadata, title, desc string
		sourcesOnly                 bool
		wantTitle, wantDesc         string
	}{
		{name: "content_sources only, no metadata.json", metadata: "{}", sourcesOnly: true,
			title: "Example Docs", desc: "Guides for the example project, kept short and current.",
			wantTitle: "Example Docs", wantDesc: "Guides for the example project, kept short and current."},
		{name: "config wins over metadata.json", metadata: exported,
			title: "Example Docs", desc: "Guides for the example project, kept short and current.",
			wantTitle: "Example Docs", wantDesc: "Guides for the example project, kept short and current."},
		{name: "metadata.json fills an empty config", metadata: exported,
			wantTitle: "Exported Name", wantDesc: "Exported tagline from the migration of the example site."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newSiteFixture(t, tc.metadata, map[string]string{"pages/guide.md": doc}, identityTemplates)
			cfg.Title, cfg.Description = tc.title, tc.desc
			if tc.sourcesOnly {
				cfg.ContentSources = []ContentSource{{Path: filepath.Join(cfg.ContentDir, "site", "pages")}}
				cfg.Source = ""
			}
			home := buildIdentitySite(t, cfg)
			if want := `<meta name="description" content="` + tc.wantDesc + `">`; !strings.Contains(home, want) {
				t.Errorf(".Description: want %s in\n%s", want, home)
			}
			if want := `content="` + tc.wantTitle + `|` + tc.wantDesc + `"`; !strings.Contains(home, want) {
				t.Errorf(".Site.Title/.Site.Description: want %s in\n%s", want, home)
			}
		})
	}
}

// TestSiteIdentityFromConfigWithMddb — #440: the mddb path reads no
// metadata.json either, and must still carry the configured identity.
func TestSiteIdentityFromConfigWithMddb(t *testing.T) {
	srv := mockMddbServer(t)
	g := newTestGen(t, "")
	g.config.Title, g.config.Description = "Example Docs", "Guides for the example project."
	g.config.Mddb = MddbConfig{Enabled: true, URL: srv.URL, Protocol: "http", Collection: "content", Timeout: 5}
	if err := g.loadContent(); err != nil {
		t.Fatalf("loadContent (mddb): %v", err)
	}
	if g.siteData.Title != "Example Docs" || g.siteData.Description != "Guides for the example project." {
		t.Errorf("site identity = %q / %q, want the configured values", g.siteData.Title, g.siteData.Description)
	}
}

func TestBuildPaletteHead(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Colors = map[string]string{
		"text":    "#111111",
		"primary": "#7b2ff7",
		"brand-x": "rgb(1, 2, 3)",
	}

	out := g.buildPaletteHead("<html><head></head></html>")

	if !strings.Contains(out, "--ssg-color-primary:#7b2ff7;") {
		t.Errorf("primary colour missing:\n%s", out)
	}
	// Known roles come first in a fixed order, theme-specific ones after, so
	// the output is byte-stable between builds.
	if i, j := strings.Index(out, "primary"), strings.Index(out, "text"); i > j {
		t.Errorf("known roles should be emitted in palette order:\n%s", out)
	}
	if !strings.Contains(out, "--ssg-color-brand-x:rgb(1, 2, 3);") {
		t.Errorf("theme-specific role missing:\n%s", out)
	}
	// No explicit theme-color anywhere: the primary colour stands in.
	if !strings.Contains(out, `<meta name="theme-color" content="#7b2ff7">`) {
		t.Errorf("theme-color fallback missing:\n%s", out)
	}
}

func TestBuildPaletteHead_SkipsWhenNotNeeded(t *testing.T) {
	g := newTestGen(t, "")

	if out := g.buildPaletteHead(""); out != "" {
		t.Errorf("no palette should emit nothing, got %q", out)
	}

	g.siteData.Colors = map[string]string{"primary": "#7b2ff7"}
	if out := g.buildPaletteHead("<style>:root{--ssg-color-primary:#abc}</style>"); out != "" {
		t.Errorf("a theme that already declares the variables wins, got %q", out)
	}

	// An export's theme-color is the site's own decision; the palette does not
	// second-guess it.
	g.siteData.Marketing.ThemeColor = "#123456"
	out := g.buildPaletteHead("")
	if strings.Contains(out, "theme-color") {
		t.Errorf("marketing theme-color must not be duplicated:\n%s", out)
	}
	g.siteData.Marketing.ThemeColor = ""
	if out := g.buildPaletteHead(`<meta name="theme-color" content="#abcdef">`); strings.Contains(out, "theme-color") {
		t.Errorf("a theme's own theme-color must not be duplicated:\n%s", out)
	}
}

// TestBuildPaletteHead_RejectsUnsafeValues: the palette is crawled from someone
// else's site, so a value that could close the declaration or the element is
// dropped rather than escaped into something meaningless.
func TestBuildPaletteHead_RejectsUnsafeValues(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Colors = map[string]string{
		"primary": "#fff}</style><script>alert(1)</script>",
		"accent":  strings.Repeat("a", 100),
		"text":    "   ",
	}

	if out := g.buildPaletteHead(""); out != "" {
		t.Errorf("no safe value survives, so nothing should be emitted, got:\n%s", out)
	}

	g.siteData.Colors["secondary"] = "#00ff00"
	out := g.buildPaletteHead("")
	if strings.Contains(out, "script") || strings.Contains(out, "}</style><") {
		t.Errorf("unsafe value leaked:\n%s", out)
	}
	if !strings.Contains(out, "--ssg-color-secondary:#00ff00;") {
		t.Errorf("the safe value should still be emitted:\n%s", out)
	}
}

func TestCSSIdent(t *testing.T) {
	cases := map[string]string{
		"primary":     "primary",
		"Brand Color": "brand-color",
		"brand_x":     "brand-x",
		"a!@#b":       "ab",
	}
	for in, want := range cases {
		if got := cssIdent(in); got != want {
			t.Errorf("cssIdent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPaletteMetaThemeColor(t *testing.T) {
	if got := paletteMetaThemeColor(map[string]string{"primary": "#abc"}); got != "#abc" {
		t.Errorf("expected the primary colour, got %q", got)
	}
	if got := paletteMetaThemeColor(map[string]string{}); got != "" {
		t.Errorf("no primary colour should yield nothing, got %q", got)
	}
	if got := paletteMetaThemeColor(map[string]string{"primary": `"><script>`}); got != "" {
		t.Errorf("an unsafe value should yield nothing, got %q", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "third"); got != "third" {
		t.Errorf("expected the first non-empty value, got %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("all-empty should yield empty, got %q", got)
	}
}
