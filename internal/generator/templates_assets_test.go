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
	templates := strings.Join([]string{
		baseTemplate, indexTemplate, pageTemplate, postTemplate, categoryTemplate,
	}, "\n")

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
	for _, id := range []string{"menu-toggle", "nav-links"} {
		if !strings.Contains(baseTemplate, `id="`+id+`"`) {
			t.Fatalf("base.html no longer ships #%s — this test is guarding nothing", id)
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
