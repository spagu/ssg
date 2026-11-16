package generator

import (
	"path/filepath"
	"strings"
	"testing"
)

// buildDocsNavSite builds a three-guide site with ssgtheme and the given
// variables, and returns the HTML of each guide by slug.
func buildDocsNavSite(t *testing.T, vars map[string]interface{}) map[string]string {
	t.Helper()
	tmp := t.TempDir()
	content := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(content, "metadata.json"), `{}`)
	slugs := []string{"install", "workers", "workers-secrets"}
	titles := []string{"SSG Installation Guide", "Workers, modules and services", "Worker secrets and variables"}
	for i, slug := range slugs {
		mustWrite(t, filepath.Join(content, "pages", slug+".md"),
			"---\ntitle: "+titles[i]+"\nslug: "+slug+"\nstatus: publish\ntype: page\n---\n\nText.\n")
	}
	out := filepath.Join(tmp, "output")
	gen, err := New(Config{Source: "site", Template: "ssgtheme", Domain: "example.com",
		ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join("..", "..", "templates"),
		OutputDir: out, Quiet: true, Variables: vars})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	pages := map[string]string{}
	for _, slug := range slugs {
		pages[slug] = readFileString(t, filepath.Join(out, slug, "index.html"))
	}
	return pages
}

// sidebar cuts the docs navigation out of a page.
func sidebar(t *testing.T, page string) string {
	t.Helper()
	start := strings.Index(page, `<nav class="docs-nav`)
	if start < 0 {
		t.Fatal("page has no docs navigation")
	}
	end := strings.Index(page[start:], "</nav>")
	return page[start : start+end]
}

func docsNavGroups() []interface{} {
	return []interface{}{
		map[string]interface{}{"title": "Core guides", "pages": []interface{}{"install"}},
		map[string]interface{}{
			"title":    "Workers & modules",
			"pages":    []interface{}{"workers", "workers-secrets"},
			"headings": map[string]interface{}{"workers": "Basics", "workers-secrets": "Secrets"},
		},
	}
}

// TestSSGThemeDocsNavHeadingsAndLabels: a group's headings split its list under
// small headings, docs_nav_labels shortens a guide's name in the sidebar only,
// and every list the sidebar opens is closed.
func TestSSGThemeDocsNavHeadingsAndLabels(t *testing.T) {
	pages := buildDocsNavSite(t, map[string]interface{}{
		"docs_nav_groups": docsNavGroups(),
		"docs_nav_labels": map[string]interface{}{"workers-secrets": "Secrets and variables", "install": "Installation"},
	})
	nav := sidebar(t, pages["workers"])
	for _, want := range []string{
		`<p class="docs-nav__part" id="docs-nav-part-workers">Basics</p>`,
		`<p class="docs-nav__part" id="docs-nav-part-workers-secrets">Secrets</p>`,
		`aria-labelledby="docs-nav-part-workers-secrets"`,
		`>Secrets and variables</a>`,
		`>Workers, modules and services</a>`,
		`Core guides →`,
	} {
		if !strings.Contains(nav, want) {
			t.Errorf("workers sidebar lacks %s", want)
		}
	}
	if open, closed := strings.Count(nav, "<ul"), strings.Count(nav, "</ul>"); open != closed {
		t.Errorf("sidebar opens %d lists and closes %d", open, closed)
	}
	// The heading on the first guide replaces the group's own list, so no
	// empty list labelled by the group title is left in front of it.
	if strings.Contains(nav, `aria-labelledby="docs-nav-group"`) {
		t.Error("a group starting with a heading must not open an unlabelled empty list first")
	}
	// The page keeps its full title as its heading.
	if !strings.Contains(pages["workers-secrets"], "<h1") || !strings.Contains(pages["workers-secrets"], "Worker secrets and variables</h1>") {
		t.Error("the label must not replace the page's own title")
	}
	core := sidebar(t, pages["install"])
	if !strings.Contains(core, `>Installation</a>`) || !strings.Contains(core, `aria-labelledby="docs-nav-group"`) {
		t.Error("a group without headings keeps one list under the group title, with labels applied")
	}
}

// TestSSGThemeDocsNavWithoutLabels: with neither headings nor labels the
// sidebar shows full titles in one list per group, as before.
func TestSSGThemeDocsNavWithoutLabels(t *testing.T) {
	groups := docsNavGroups()
	delete(groups[1].(map[string]interface{}), "headings")
	nav := sidebar(t, buildDocsNavSite(t, map[string]interface{}{"docs_nav_groups": groups})["workers"])
	if strings.Contains(nav, "docs-nav__part") {
		t.Error("no headings configured, none rendered")
	}
	if !strings.Contains(nav, `>Worker secrets and variables</a>`) {
		t.Error("without labels the sidebar shows the full title")
	}
}
