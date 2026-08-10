package generator

// #110: structured-data defaults per content section.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// sectionGen builds a generator whose content root is dir.
func sectionGen(dir string, defaults map[string]map[string]interface{}) *Generator {
	return &Generator{
		config: Config{
			Domain: "food.example", ContentDir: dir, Source: "food",
			SchemaDefaults: defaults, Quiet: true,
		},
		siteData: &models.SiteData{},
	}
}

// TestSectionSchemaAppliesToItsSection: without this, an @type had to be
// repeated in every file — a hundred recipes meant a hundred copies, and
// site-wide `schema:` could not carry it because it would apply to posts too.
func TestSectionSchemaAppliesToItsSection(t *testing.T) {
	root := t.TempDir()
	g := sectionGen(root, map[string]map[string]interface{}{
		"pages/recipes": {"@type": "Recipe", "recipeCuisine": "Polish"},
	})

	recipe := models.Page{Slug: "pierogi", Type: "page",
		SourceDir: filepath.Join(root, "food", "pages", "recipes")}
	got := g.sectionSchema(recipe)
	if got == nil || got["@type"] != "Recipe" {
		t.Fatalf("recipe section: got %v", got)
	}

	// A page outside the section keeps the derived type.
	other := models.Page{Slug: "about", Type: "page",
		SourceDir: filepath.Join(root, "food", "pages")}
	if g.sectionSchema(other) != nil {
		t.Error("a page outside the section picked up its defaults")
	}
}

// TestSectionSchemaLongestPrefixWins mirrors link_rewrites, so the project has
// one prefix rule rather than two.
func TestSectionSchemaLongestPrefixWins(t *testing.T) {
	root := t.TempDir()
	g := sectionGen(root, map[string]map[string]interface{}{
		"pages":         {"@type": "WebPage", "inLanguage": "en"},
		"pages/recipes": {"@type": "Recipe"},
	})
	page := models.Page{Slug: "pierogi", Type: "page",
		SourceDir: filepath.Join(root, "food", "pages", "recipes")}
	if got := g.sectionSchema(page); got["@type"] != "Recipe" {
		t.Errorf("longest prefix should win: got %v", got)
	}
}

// TestSectionSchemaHomeKey: the home page is the only page that can carry a
// site-level @type without claiming it for every other page.
func TestSectionSchemaHomeKey(t *testing.T) {
	root := t.TempDir()
	g := sectionGen(root, map[string]map[string]interface{}{
		"home": {"@type": "SoftwareApplication", "applicationCategory": "DeveloperApplication"},
	})
	home := g.indexPageContext(filepath.Join("/out", indexHTMLName))
	got := g.sectionSchema(*home)
	if got == nil || got["@type"] != "SoftwareApplication" {
		t.Fatalf("home: got %v", got)
	}
	// "home" must not leak onto ordinary pages.
	other := models.Page{Slug: "about", Type: "page",
		SourceDir: filepath.Join(root, "food", "pages")}
	if g.sectionSchema(other) != nil {
		t.Error("the home defaults reached an ordinary page")
	}
}

// TestSectionSchemaPrecedence pins the order the feature depends on: section
// defaults outrank the derived @type — that is what they are for — while a
// page's own schema still wins over its section.
func TestSectionSchemaPrecedence(t *testing.T) {
	root := t.TempDir()
	g := sectionGen(root, map[string]map[string]interface{}{
		"pages/recipes": {"@type": "Recipe", "recipeCuisine": "Polish"},
	})
	g.config.Schema = map[string]interface{}{
		"publisher": map[string]interface{}{"@type": "Organization", "name": "Food"},
	}
	page := models.Page{
		Title: "Pierogi", Slug: "pierogi", Type: "page",
		SourceDir: filepath.Join(root, "food", "pages", "recipes"),
		Schema:    map[string]interface{}{"cookTime": "PT20M", "recipeCuisine": "Silesian"},
	}

	out := g.buildJSONLD(page, false)
	for _, want := range []string{
		`"@type":"Recipe"`,     // section beats the derived WebPage
		`"cookTime":"PT20M"`,   // the page's own field survives
		`"recipeCuisine":"Sil`, // the page overrides its section
		`"name":"Food"`,        // site-wide publisher still reaches it
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
}

// TestSectionSchemaIgnoredWhenUnconfigured keeps the feature free for sites
// that do not use it.
func TestSectionSchemaIgnoredWhenUnconfigured(t *testing.T) {
	g := sectionGen(t.TempDir(), nil)
	if got := g.sectionSchema(models.Page{Slug: "x", SourceDir: "/nowhere"}); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}
