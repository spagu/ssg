package generator

import (
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
