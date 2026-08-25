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
	// Asserted against the shared skeleton, which is where the header now lives.
	// This used to check all four page templates, because the header was
	// repeated in all four and a button losing its id in three of them would
	// have left the check passing on the fourth. There is one copy now (#216),
	// so one place to look — which is the improvement, not a weakening.
	for _, id := range []string{"menu-toggle", "nav-links"} {
		if !strings.Contains(partialsTemplate, `id="`+id+`"`) {
			t.Fatalf("the scaffold skeleton no longer ships #%s — this test is guarding nothing", id)
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
	return []string{partialsTemplate, indexTemplate, pageTemplate, postTemplate, categoryTemplate}
}

// scaffoldPageTemplates returns only the four addressed as page templates —
// the ones that must no longer carry a document of their own (#216).
func scaffoldPageTemplates() []string {
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

// buildWithTheme renders a one-page site with the given theme and title.
func buildWithTheme(t *testing.T, theme, title string) string {
	t.Helper()
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(contentDir, "metadata.json"), `{"categories":[],"media":[],"users":[]}`)
	mustWrite(t, filepath.Join(contentDir, "pages", "about.md"),
		"---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nBody.\n")

	gen, err := New(Config{
		Source: "site", Template: theme, Domain: "example.com", Title: title,
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

// TestADefaultThemeShowsTheSiteName covers both themes a user can end up on
// without choosing one: `simple`, which `ssg init` writes into a fresh config,
// and the scaffold, which a theme with no local files falls back to.
//
// Making `title` settable over MCP achieves nothing if neither reads it (#212),
// and neither did: the setting wrote cleanly into the config and changed
// nothing on the page — the same trap as the base.html of #208 (#213).
func TestADefaultThemeShowsTheSiteName(t *testing.T) {
	for _, theme := range []string{"simple", "scaffoldname"} {
		t.Run(theme, func(t *testing.T) {
			named := buildWithTheme(t, theme, "Tradik")
			if !strings.Contains(named, "<title>Tradik") {
				t.Errorf("the configured name must reach the page:\n%s", firstTag(named, "<title>"))
			}
			// The host is still the host: neither a canonical URL nor a
			// copyright line is a title.
			if !strings.Contains(named, `href="https://example.com/"`) {
				t.Error("the canonical URL must keep the host")
			}
			if !strings.Contains(named, "&copy; example.com") {
				t.Error("the copyright line names the host, not the title")
			}

			// Unset, the domain stands in — which is why the golden corpora,
			// none of which configure a title, were unmoved by this change.
			if unnamed := buildWithTheme(t, theme, ""); !strings.Contains(unnamed, "<title>example.com") {
				t.Errorf("without a title the domain must stand in:\n%s", firstTag(unnamed, "<title>"))
			}
		})
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
	}

	// The language is resolved once, in the shared skeleton — the four page
	// templates no longer carry an <html> element of their own (#216), so
	// asserting on them would be asserting on a file that cannot fail.
	shared := mustRead(t, filepath.Join("..", "..", "templates", "simple", "partials.html"))
	if strings.Contains(shared, `<html lang="pl"`) || strings.Contains(shared, `<html lang="en"`) {
		t.Error("the shared skeleton hardcodes a document language")
	}
	if !strings.Contains(shared, "{{if .Ctx.Lang}}") {
		t.Error("the shared skeleton must resolve the document language")
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

// TestTheScaffoldDoesNotRepeatItsDocument: the scaffold carried four standalone
// documents too, which is the shape #208 removed a broken base.html for. It now
// ships a partials file that works — a skeleton the pages wrap themselves in,
// since Go templates cannot dispatch {{template}} on a computed name (#216).
func TestTheScaffoldDoesNotRepeatItsDocument(t *testing.T) {
	if n := strings.Count(partialsTemplate, `{{define "site-open"}}`); n != 1 {
		t.Fatalf("the skeleton must be defined once, found %d", n)
	}
	for _, tmpl := range scaffoldPageTemplates() {
		for _, repeated := range []string{"<!DOCTYPE html>", "<html lang", `<footer class="site-footer"`} {
			if strings.Contains(tmpl, repeated) {
				t.Errorf("a scaffold template carries its own %q instead of the shared skeleton", repeated)
			}
		}
		if !strings.Contains(tmpl, `{{template "site-open"`) || !strings.Contains(tmpl, `{{template "site-close"`) {
			t.Error("every scaffold template must wrap itself in the shared skeleton")
		}
	}

	// A post's canonical must come from the method that knows the URL format,
	// not from a hand-built path: the scaffold used to publish
	// https://site/<slug>/ for a post rendered at /2024/01/02/<slug>/, so every
	// post pointed search engines at a 404 (#217).
	if !strings.Contains(postTemplate, ".Post.GetCanonical") {
		t.Error("the post canonical must be derived, not hand-built")
	}
	if strings.Contains(postTemplate, `https://{{.Domain}}/{{.Post.Slug}}/`) {
		t.Error("the hand-built post canonical is back")
	}
}

// TestTheBundledThemeDoesNotRepeatItsDocument: the four page templates used to
// carry a whole standalone document each, so the header and footer existed in
// four copies and changing three of them was a silent inconsistency (#216).
func TestTheBundledThemeDoesNotRepeatItsDocument(t *testing.T) {
	shared := mustRead(t, filepath.Join("..", "..", "templates", "simple", "partials.html"))
	for _, block := range []string{"site-open", "site-close", "site-name", "site-nav"} {
		if !strings.Contains(shared, `{{define "`+block+`"}}`) {
			t.Errorf("partials.html no longer defines %q — this test is guarding nothing", block)
		}
	}

	for _, name := range []string{"index.html", "category.html", "page.html", "post.html"} {
		body := mustRead(t, filepath.Join("..", "..", "templates", "simple", name))
		// The skeleton lives in one place. A page template carrying its own
		// <html> or <footer> has drifted back to a copy.
		for _, repeated := range []string{"<!DOCTYPE html>", "<html lang", "<footer class=\"site-footer\"", "<script src=\"/js/main.js\">"} {
			if strings.Contains(body, repeated) {
				t.Errorf("%s carries its own %q instead of using the shared skeleton", name, repeated)
			}
		}
		if !strings.Contains(body, `{{template "site-open"`) || !strings.Contains(body, `{{template "site-close"`) {
			t.Errorf("%s must wrap itself in the shared skeleton", name)
		}
	}
}
