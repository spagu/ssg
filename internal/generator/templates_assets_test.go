package generator

// The scaffold writes what it links (#172).
//
// Every scaffolded template carried a stylesheet and a script link, and nothing
// wrote either file: the site built, the pages were right, and a browser
// rendered unstyled HTML with two 404s in its console while the build reported
// success.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryScaffoldedReferenceIsWritten: the assertion that would have caught
// the report. Each asset the templates name must be a file the scaffold writes.
func TestEveryScaffoldedReferenceIsWritten(t *testing.T) {
	dir := t.TempDir()
	if err := writeScaffoldAssets(dir); err != nil {
		t.Fatal(err)
	}
	templates := strings.Join(scaffoldTemplates(), "\n")

	for _, ref := range []string{"/css/style.css", "/js/main.js"} {
		if !strings.Contains(templates, ref) {
			t.Fatalf("the templates no longer reference %s — this test is guarding nothing", ref)
		}
		path := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(ref, "/")))
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("templates link %s and the scaffold does not write it: %v", ref, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty, which renders the same as missing", ref)
		}
	}
}

// TestScaffoldAssetsAreNotOverwritten: a theme being edited must not have its
// stylesheet replaced by the next build.
func TestScaffoldAssetsAreNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	css := filepath.Join(dir, "css", "style.css")
	if err := os.MkdirAll(filepath.Dir(css), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := "body { color: rebeccapurple }"
	if err := os.WriteFile(css, []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeScaffoldAssets(dir); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, css); got != mine {
		t.Errorf("an existing stylesheet must be left alone, got:\n%s", got)
	}
	// The one that was missing is still written.
	if _, err := os.Stat(filepath.Join(dir, "js", "main.js")); err != nil {
		t.Errorf("the missing asset must still be created: %v", err)
	}
}

// TestScaffoldStylesheetIsSelfContained: no external reference, because a
// scaffold that fetches a font from a CDN makes a site slower and less private
// than the one it replaced (FE-011).
func TestScaffoldStylesheetIsSelfContained(t *testing.T) {
	for _, forbidden := range []string{"http://", "https://", "@import", "//fonts."} {
		if strings.Contains(scaffoldStylesheet, forbidden) {
			t.Errorf("the scaffold stylesheet must not contain %q", forbidden)
		}
	}
	// The elements the templates actually ship must all be styled — a rule set
	// that misses the nav leaves the page looking broken in exactly one place.
	for _, sel := range []string{
		".container", ".site-header", ".main-nav", ".nav-links", ".menu-toggle",
		".skip-link", ".main-content", ".site-footer", ".hero", ".post-list", ".pagination",
	} {
		if !strings.Contains(scaffoldStylesheet, sel) {
			t.Errorf("the templates ship %s and the stylesheet does not style it", sel)
		}
	}
}

// TestScaffoldScriptTargetsWhatTheTemplatesShip: the templates carry a menu
// button, and a button that does nothing is worse than no button.
func TestScaffoldScriptTargetsWhatTheTemplatesShip(t *testing.T) {
	// Asserted against every template the scaffold writes, not one of them:
	// the header is repeated in all four, so a button losing its id in three
	// of them would leave this passing on the fourth (#208).
	for _, id := range []string{"menu-toggle", "nav-links"} {
		for _, tmpl := range scaffoldTemplates() {
			if !strings.Contains(tmpl, `id="`+id+`"`) {
				t.Fatalf("a scaffold template no longer ships #%s — this test is guarding nothing", id)
			}
		}
		if !strings.Contains(scaffoldScript, id) {
			t.Errorf("the script must drive #%s", id)
		}
	}
	// It must survive a theme that kept the script and replaced the markup.
	if !strings.Contains(scaffoldScript, "if (!toggle || !links) return") {
		t.Error("the script must do nothing when its elements are absent, not throw")
	}
	// And say what it did, for a screen reader.
	if !strings.Contains(scaffoldScript, "aria-expanded") {
		t.Error("the toggle must report its state")
	}
}

// TestWriteScaffoldAssetsReportsRealFailures: a scaffold that silently fails to
// write its stylesheet leaves exactly the defect this feature exists to fix.
func TestWriteScaffoldAssetsReportsRealFailures(t *testing.T) {
	root := t.TempDir()
	// A file where the css/ directory needs to be.
	if err := os.WriteFile(filepath.Join(root, "css"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := writeScaffoldAssets(root)
	if err == nil {
		t.Fatal("a blocked directory must be reported")
	}
	if !strings.Contains(err.Error(), "css") {
		t.Errorf("the error must name what it could not create: %v", err)
	}
}

// TestScaffoldAssetsCreateTheirDirectories, since a fresh theme has neither.
func TestScaffoldAssetsCreateTheirDirectories(t *testing.T) {
	root := t.TempDir()
	if err := writeScaffoldAssets(root); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"css", "js"} {
		info, err := os.Stat(filepath.Join(root, dir))
		if err != nil || !info.IsDir() {
			t.Errorf("%s/ must be created: %v", dir, err)
		}
	}
}

// TestScaffoldAssetWriteFailureIsReported: a directory that exists but cannot
// be written into. The scaffold must say so rather than leave the site linking
// a file it never wrote — which is the whole defect this closes.
func TestScaffoldAssetWriteFailureIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test relies on")
	}
	root := t.TempDir()
	css := filepath.Join(root, "css")
	if err := os.MkdirAll(css, 0o555); err != nil { // read-only
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(css, 0o755) })

	err := writeScaffoldAssets(root)
	if err == nil {
		t.Fatal("an unwritable asset directory must be reported")
	}
	if !strings.Contains(err.Error(), "style.css") {
		t.Errorf("the error must name the file: %v", err)
	}
}

// scaffoldTemplates returns every template the scaffold writes to disk. Named
// once so a test cannot assert against a template that is no longer shipped —
// which is how base.html kept its guarantees long after nothing included it.
func scaffoldTemplates() []string {
	return []string{indexTemplate, pageTemplate, postTemplate, categoryTemplate}
}

// TestTheScaffoldDeclaresTheDocumentLanguage drives a real build, because the
// four template contexts are not one shape: pages and archives get a map, the
// front page and taxonomy indexes get anonymous structs. A field missing from a
// struct is not an empty value in a Go template — it is a hard error, on the
// most linked document the site has. That is how #186 nearly shipped, and #208
// is the same surface (#208).
func TestTheScaffoldDeclaresTheDocumentLanguage(t *testing.T) {
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(contentDir, "metadata.json"),
		`{"categories":[{"id":1,"name":"Nowosci","slug":"nowosci"}],"media":[],"users":[]}`)
	mustWrite(t, filepath.Join(contentDir, "pages", "o-nas.md"),
		"---\ntitle: O nas\nslug: o-nas\nstatus: publish\ntype: page\nlang: pl\n---\n\nTresc.\n")
	mustWrite(t, filepath.Join(contentDir, "posts", "news", "wpis.md"),
		"---\ntitle: Wpis\nslug: wpis\nstatus: publish\ntype: post\ndate: 2024-01-02\ncategories: [Nowosci]\nlang: pl\n---\n\nTresc.\n")

	gen, err := New(Config{
		Source: "site", Template: "scaffoldpl", Domain: "example.com",
		DefaultLanguage: "pl",
		ContentDir:      filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir: filepath.Join(tmp, "output"), Quiet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// The scaffold no longer writes a layout nothing includes.
	if _, err := os.Stat(filepath.Join(tmp, "templates", "scaffoldpl", "base.html")); err == nil {
		t.Error("base.html must not be written")
	}

	// Every document the theme rendered says pl — the front page included,
	// which is the one that renders from a struct.
	var checked int
	err = filepath.WalkDir(filepath.Join(tmp, "output"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".html" || filepath.Base(p) == "404.html" {
			return nil //nolint:nilerr // the generated 404 has its own English copy (#209)
		}
		checked++
		if body := mustRead(t, p); !strings.Contains(body, `<html lang="pl">`) {
			rel, _ := filepath.Rel(tmp, p)
			t.Errorf("%s does not declare pl", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 4 {
		t.Fatalf("only %d document(s) checked — the fixture proves too little", checked)
	}
}

// TestTheScaffoldFallsBackToEnglishWhenNothingSaysOtherwise, so a site that
// declares no language is unchanged.
func TestTheScaffoldFallsBackToEnglishWhenNothingSaysOtherwise(t *testing.T) {
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(contentDir, "metadata.json"), `{"categories":[],"media":[],"users":[]}`)
	mustWrite(t, filepath.Join(contentDir, "pages", "about.md"),
		"---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nBody.\n")

	gen, err := New(Config{
		Source: "site", Template: "scaffolden", Domain: "example.com",
		ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir: filepath.Join(tmp, "output"), Quiet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if body := mustRead(t, filepath.Join(tmp, "output", "about", indexHTMLName)); !strings.Contains(body, `<html lang="en">`) {
		t.Error("with no language anywhere the scaffold must still say en")
	}
}

// TestTheScaffoldShowsTheSiteName: making `title` settable over MCP (#212)
// achieves nothing if no default theme reads it. A key that writes cleanly and
// changes nothing visible is the same trap as the base.html of #208 — it looks
// like it worked.
func TestTheScaffoldShowsTheSiteName(t *testing.T) {
	build := func(title string) string {
		tmp := t.TempDir()
		contentDir := filepath.Join(tmp, "content", "site")
		mustWrite(t, filepath.Join(contentDir, "metadata.json"), `{"categories":[],"media":[],"users":[]}`)
		mustWrite(t, filepath.Join(contentDir, "pages", "about.md"),
			"---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nBody.\n")
		gen, err := New(Config{
			Source: "site", Template: "scaffoldname", Domain: "example.com", Title: title,
			ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
			OutputDir: filepath.Join(tmp, "output"), Quiet: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := gen.Generate(); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		return mustRead(t, filepath.Join(tmp, "output", indexHTMLName))
	}

	named := build("Tradik")
	if !strings.Contains(named, "<title>Tradik - Home</title>") {
		t.Errorf("the configured name must reach the page:\n%s", firstTag(named, "<title>"))
	}
	// The host is still the host: a canonical URL is not a title.
	if !strings.Contains(named, `href="https://example.com/"`) {
		t.Error("the canonical URL must keep the domain")
	}

	// With no name configured, the domain stands in — an existing site that
	// never set `title` looks exactly as it did.
	unnamed := build("")
	if !strings.Contains(unnamed, "<title>example.com - Home</title>") {
		t.Errorf("without a title the domain must stand in:\n%s", firstTag(unnamed, "<title>"))
	}
}

// firstTag returns the line carrying tag, for a readable failure.
func firstTag(body, tag string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, tag) {
			return strings.TrimSpace(line)
		}
	}
	return "(not found)"
}

// TestTheBundledThemeShowsTheSiteName: `simple` is what `ssg init` writes, so
// it is the theme most people meet first — and it printed the host wherever it
// meant the name, which made `title` a setting that wrote cleanly and changed
// nothing (#213).
func TestTheBundledThemeShowsTheSiteName(t *testing.T) {
	build := func(title string) string {
		tmp := t.TempDir()
		contentDir := filepath.Join(tmp, "content", "site")
		mustWrite(t, filepath.Join(contentDir, "metadata.json"), `{"categories":[],"media":[],"users":[]}`)
		mustWrite(t, filepath.Join(contentDir, "pages", "about.md"),
			"---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nBody.\n")
		gen, err := New(Config{
			Source: "site", Template: "simple", Domain: "example.com", Title: title,
			ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
			OutputDir: filepath.Join(tmp, "output"), Quiet: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := gen.Generate(); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		return mustRead(t, filepath.Join(tmp, "output", indexHTMLName))
	}

	named := build("Tradik")
	if !strings.Contains(named, "<title>Tradik") {
		t.Errorf("the configured name must reach the page:\n%s", firstTag(named, "<title>"))
	}
	if !strings.Contains(named, `href="https://example.com/"`) {
		t.Error("the canonical URL must keep the host")
	}
	if !strings.Contains(named, "&copy; example.com") {
		t.Error("the copyright line names the host, not the title")
	}

	// Unset, the domain stands in — which is why the golden corpora, none of
	// which configure a title, are byte-identical across this change.
	if unnamed := build(""); !strings.Contains(unnamed, "<title>example.com") {
		t.Errorf("without a title the domain must stand in:\n%s", firstTag(unnamed, "<title>"))
	}
}

// TestTheBundledThemeIsNeutral: `simple` is what `ssg init` writes, so a fresh
// site anywhere in the world used to greet visitors in Polish and declare
// itself Polish to screen readers, whatever language it was actually in (#213).
func TestTheBundledThemeIsNeutral(t *testing.T) {
	for _, name := range []string{"index.html", "category.html", "page.html", "post.html"} {
		body := mustRead(t, filepath.Join("..", "..", "templates", "simple", name))

		// No Polish diacritics: the copy is English now, and a stray one would
		// mean a phrase was missed.
		if i := strings.IndexAny(body, "ąćęłńóśźżĄĆĘŁŃÓŚŹŻ"); i >= 0 {
			t.Errorf("%s still carries Polish copy near %q", name, body[max(0, i-40):min(len(body), i+40)])
		}
		// And it must not hardcode a language, for the same reason the scaffold
		// must not (#208): the export knows what language the site is in.
		if strings.Contains(body, `<html lang="pl"`) || strings.Contains(body, `<html lang="en"`) {
			t.Errorf("%s hardcodes a document language", name)
		}
		if !strings.Contains(body, "{{if .Lang}}") {
			t.Errorf("%s must resolve the document language", name)
		}
	}
}
