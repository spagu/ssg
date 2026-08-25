package generator

// The generated 404 in the site's own language (#209).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
)

// polishSite writes a project with a Polish catalog covering the keys named.
func polishSite(t *testing.T, keys map[string]string) string {
	t.Helper()
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(contentDir, "metadata.json"), `{"categories":[],"media":[],"users":[]}`)
	mustWrite(t, filepath.Join(contentDir, "pages", "o-nas.md"),
		"---\ntitle: O nas\nslug: o-nas\nstatus: publish\ntype: page\nlang: pl\n---\n\nTresc.\n")

	if keys != nil {
		// Nested, because Lookup splits a dotted key and walks the map. Flat
		// keys with dots in them parse fine and match nothing — which is what
		// the first version of this fixture did.
		var b strings.Builder
		b.WriteString("not_found:\n")
		for _, k := range []string{"title", "body", "home"} {
			if v, ok := keys["not_found."+k]; ok {
				b.WriteString("  " + k + `: "` + v + `"` + "\n")
			}
		}
		mustWrite(t, filepath.Join(tmp, "i18n", "pl.yaml"), b.String())
	}
	return tmp
}

// buildPolish generates the site and returns the 404 it wrote.
func buildPolish(t *testing.T, tmp string, i18nOn bool) string {
	t.Helper()
	cfg := Config{
		Source: "site", Template: "scaffold404", Domain: "example.com",
		DefaultLanguage: "pl",
		ContentDir:      filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir: filepath.Join(tmp, "output"), Quiet: true,
	}
	if i18nOn {
		cfg.Languages = []string{"pl"}
		cfg.I18n = ssgi18n.Config{Enabled: true, TranslationsDir: filepath.Join(tmp, "i18n")}
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return mustRead(t, filepath.Join(tmp, "output", "404.html"))
}

// TestATranslated404SpeaksTheSiteLanguage — the reported case. This page is
// served to real visitors, on a static host, for every dead URL; after a
// migration, dead URLs are what old links produce.
func TestATranslated404SpeaksTheSiteLanguage(t *testing.T) {
	tmp := polishSite(t, map[string]string{
		"not_found.title": "404 — nie znaleziono strony",
		"not_found.body":  "Ta strona nie istnieje w serwisie {{site}}.",
		"not_found.home":  "Przejdź na stronę główną",
	})
	page := buildPolish(t, tmp, true)

	for _, want := range []string{
		`<html lang="pl">`,
		"nie znaleziono strony",
		"Ta strona nie istnieje w serwisie example.com.",
		"Przejdź na stronę główną",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the 404 is missing %q:\n%s", want, page)
		}
	}
	// The placeholder is substituted, not printed.
	if strings.Contains(page, "{{site}}") {
		t.Error("the site placeholder was left unresolved")
	}
	if strings.Contains(page, "page not found") {
		t.Error("English copy survived into a translated page")
	}
}

// TestAPartlyTranslated404StaysEnglish. Two translated lines, one English one
// and lang="pl" is worse than a wholly English page: the mislabelled part is
// exactly the part a screen reader gets wrong, and nobody proof-reads a 404.
func TestAPartlyTranslated404StaysEnglish(t *testing.T) {
	tmp := polishSite(t, map[string]string{
		"not_found.title": "404 — nie znaleziono strony",
		"not_found.body":  "Ta strona nie istnieje w serwisie {{site}}.",
		// not_found.home deliberately absent
	})
	page := buildPolish(t, tmp, true)

	if !strings.Contains(page, `<html lang="en">`) {
		t.Errorf("an incomplete translation must not relabel the page:\n%s", page)
	}
	if strings.Contains(page, "nie znaleziono") {
		t.Error("a partial translation must not be used at all")
	}
	if !strings.Contains(page, "Go to the home page") {
		t.Error("the English copy must be intact")
	}
}

// TestASiteWithNoCatalogIsUnchanged, silently — a warning per string, on every
// build, about copy the author never wrote, would be noise.
func TestASiteWithNoCatalogIsUnchanged(t *testing.T) {
	page := buildPolish(t, polishSite(t, nil), false)
	for _, want := range []string{`<html lang="en">`, "404 — page not found", "example.com", "Go to the home page"} {
		if !strings.Contains(page, want) {
			t.Errorf("the English 404 is missing %q:\n%s", want, page)
		}
	}
}

// TestTheSiteNameFallsBackWhenNoDomainIsConfigured, so a scaffolded project
// does not serve a sentence with a hole in it.
func TestTheSiteNameFallsBackWhenNoDomainIsConfigured(t *testing.T) {
	g, err := New(Config{DefaultLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if got := g.notFoundText().Body; !strings.Contains(got, "This site") {
		t.Errorf("body = %q", got)
	}
}

// TestASiteThatWritesItsOwn404IsLeftAlone — the escape hatch a theme has.
func TestASiteThatWritesItsOwn404IsLeftAlone(t *testing.T) {
	tmp := polishSite(t, nil)
	out := filepath.Join(tmp, "output")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(out, "404.html"), "<html>mine</html>")

	if page := buildPolish(t, tmp, false); page != "<html>mine</html>" {
		t.Errorf("an existing 404 must not be overwritten: %q", page)
	}
}

// TestTheCopyIsEscaped: the domain reaches the page as text, and a site name
// with a bracket in it must not become markup.
func TestTheCopyIsEscaped(t *testing.T) {
	got := renderNotFound(notFoundCopy{
		Title: `<script>alert(1)</script>`, Body: "b & c", Home: `"home"`, Lang: "en",
	})
	if strings.Contains(got, "<script>alert(1)</script>") {
		t.Errorf("the title was not escaped:\n%s", got)
	}
	if !strings.Contains(got, "b &amp; c") {
		t.Errorf("the body was not escaped:\n%s", got)
	}
}
