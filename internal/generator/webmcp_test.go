package generator

// WebMCP: the built site declaring its own tools to a browser agent (#224).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// webmcpSite writes a small site and returns its content/templates/output dirs.
// The templates are deliberately minimal: this feature is about what the build
// appends to a finished document, not about what a theme puts in it.
func webmcpSite(t *testing.T) (contentDir, tmplDir, outDir string) {
	t.Helper()
	tmp := t.TempDir()
	contentDir = filepath.Join(tmp, "content")
	tmplDir = filepath.Join(tmp, "templates")
	outDir = filepath.Join(tmp, "output")

	mustWrite(t, filepath.Join(contentDir, "site", "metadata.json"),
		`{"categories":[{"id":1,"name":"News","slug":"news"}],"media":[],"users":[]}`)
	mustWrite(t, filepath.Join(contentDir, "site", "posts", "news", "one.md"),
		"---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\ncategories: [News]\ntags: [go]\n---\n\nBody of one.\n")
	mustWrite(t, filepath.Join(contentDir, "site", "pages", "about.md"),
		"---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout this site.\n")

	for _, name := range []string{"base.html", "index.html", "post.html", "page.html",
		"category.html", "tag.html", "taxonomy.html"} {
		mustWrite(t, filepath.Join(tmplDir, "simple", name),
			`{{define "`+name+`"}}<html><body><p>page</p></body></html>{{end}}`)
	}
	return contentDir, tmplDir, outDir
}

func webmcpConfig(contentDir, tmplDir, outDir string) Config {
	return Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: contentDir, TemplatesDir: tmplDir, OutputDir: outDir, Quiet: true,
	}
}

// buildWebMCPSite runs a full build and returns every rendered HTML document.
func buildWebMCPSite(t *testing.T, cfg Config) map[string]string {
	t.Helper()
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	docs := map[string]string{}
	err = filepath.WalkDir(cfg.OutputDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".html" {
			return nil //nolint:nilerr // non-HTML output is not this test's business
		}
		rel, _ := filepath.Rel(cfg.OutputDir, p)
		docs[filepath.ToSlash(rel)] = mustRead(t, p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 {
		t.Fatal("the build produced no HTML")
	}
	return docs
}

// TestWebMCPIsAbsentUnlessAsked is the property that lets this ship at all.
// The API is a draft; a site that did not ask for it must be unchanged, byte
// for byte, and the golden baseline depends on exactly that.
func TestWebMCPIsAbsentUnlessAsked(t *testing.T) {
	contentDir, tmplDir, outDir := webmcpSite(t)
	for name, doc := range buildWebMCPSite(t, webmcpConfig(contentDir, tmplDir, outDir)) {
		if strings.Contains(doc, "modelContext") {
			t.Errorf("%s carries WebMCP without being asked", name)
		}
	}
}

// TestWebMCPReachesEveryDocumentOfARealBuild is the test a unit test cannot
// stand in for. The injection runs inside transformHTMLPage, which listing
// pages enter with a nil page context — the front page and the archives, which
// is to say the documents an agent opens first.
func TestWebMCPReachesEveryDocumentOfARealBuild(t *testing.T) {
	contentDir, tmplDir, outDir := webmcpSite(t)
	cfg := webmcpConfig(contentDir, tmplDir, outDir)
	cfg.WebMCP = true
	docs := buildWebMCPSite(t, cfg)

	for name, doc := range docs {
		if !strings.Contains(doc, "navigator.modelContext") {
			t.Errorf("%s has no WebMCP registration", name)
		}
		for _, tool := range []string{"searchPosts", "listByTag", "getDocument", "navigate"} {
			if !strings.Contains(doc, `"`+tool+`"`) {
				t.Errorf("%s does not declare %s", name, tool)
			}
		}
	}
	// The front page enters the transform with no models.Page at all, so it is
	// named rather than swept up by the loop above.
	if front, ok := docs[indexHTMLName]; !ok || !strings.Contains(front, "navigator.modelContext") {
		t.Error("the front page must carry the registration")
	}
}

// TestWebMCPBringsItsIndexWithIt: four tools that all throw on first call is a
// worse outcome than no tools, and it is what shipping the script without the
// index would produce. Nothing here sets SearchIndex.
func TestWebMCPBringsItsIndexWithIt(t *testing.T) {
	contentDir, tmplDir, outDir := webmcpSite(t)
	cfg := webmcpConfig(contentDir, tmplDir, outDir)
	cfg.WebMCP = true
	buildWebMCPSite(t, cfg)

	raw, err := os.ReadFile(filepath.Join(outDir, "search-index.json"))
	if err != nil {
		t.Fatalf("WebMCP shipped without the index its tools read: %v", err)
	}
	var docs []map[string]any
	if err := json.Unmarshal(raw, &docs); err != nil {
		t.Fatalf("search-index.json is not the array the script expects: %v", err)
	}
	if len(docs) < 2 {
		t.Errorf("index holds %d documents, want the post and the page", len(docs))
	}
	// The script reads exactly these keys. A rename in searchRecord that left
	// them behind would break every tool silently.
	for _, key := range []string{"title", "url", "excerpt", "tags", "text"} {
		if _, ok := docs[0][key]; !ok {
			t.Errorf("index records have no %q, which the tools read", key)
		}
	}
}

// TestWebMCPSurvivesMinification: the runtime is spliced in before the minifier
// runs, and a minifier that collapsed whitespace inside a script would rewrite
// working JavaScript into broken JavaScript.
func TestWebMCPSurvivesMinification(t *testing.T) {
	contentDir, tmplDir, outDir := webmcpSite(t)
	cfg := webmcpConfig(contentDir, tmplDir, outDir)
	cfg.WebMCP = true
	cfg.MinifyHTML = true
	docs := buildWebMCPSite(t, cfg)

	for name, doc := range docs {
		if !strings.Contains(doc, "navigator.modelContext") {
			t.Errorf("%s lost its registration to minification", name)
		}
		if !strings.Contains(doc, "registerTool") {
			t.Errorf("%s lost registerTool to minification", name)
		}
	}
}

// TestWebMCPLeavesAThemeSRegistrationAlone: a theme that ships its own tools
// keeps them. Two registrations of the same name is the failure this prevents.
func TestWebMCPLeavesAThemeSRegistrationAlone(t *testing.T) {
	own := `<html><body><script>navigator.modelContext.registerTool({name:"mine"})</script></body></html>`
	if got := injectWebMCP(own, "/search-index.json"); got != own {
		t.Errorf("a document with its own registration was modified:\n%s", got)
	}
}

// TestWebMCPGoesBeforeTheClosingBody, and lands at the end even when there is
// no </body> to aim at — a partial or a fragment must not lose the script.
func TestWebMCPGoesBeforeTheClosingBody(t *testing.T) {
	got := injectWebMCP(`<html><body><p>x</p></body></html>`, "/search-index.json")
	if !strings.Contains(got, "</script>\n</body>") {
		t.Errorf("registration is not at the </body> seam:\n%s", got)
	}
	fragment := injectWebMCP(`<p>x</p>`, "/search-index.json")
	if !strings.Contains(fragment, "navigator.modelContext") {
		t.Error("a document without </body> lost the registration")
	}
}

// TestWebMCPIndexURLFollowsTheDocumentSLanguage: an agent asking a Polish page
// for its posts must not be handed the English index.
func TestWebMCPIndexURLFollowsTheDocumentSLanguage(t *testing.T) {
	g := &Generator{config: Config{DefaultLanguage: "en"}}
	if got := g.webmcpIndexURL(nil); got != "/search-index.json" {
		t.Errorf("without i18n: %q", got)
	}

	g.config.I18n.Enabled = true
	g.siteData = &models.SiteData{Languages: []ssgi18n.LanguageConfig{{Code: "en"}, {Code: "pl"}}}

	if got := g.webmcpIndexURL(&models.Page{Lang: "pl"}); got != "/pl/search-index.json" {
		t.Errorf("pl page: %q, want the pl index", got)
	}
	// The default language is not prefixed, so it reads the root index.
	if got := g.webmcpIndexURL(&models.Page{Lang: "en"}); got != "/search-index.json" {
		t.Errorf("en page: %q, want the root index", got)
	}
	// A listing page carries no models.Page; it falls back to the language the
	// build is currently rendering.
	g.currentLang = "pl"
	if got := g.webmcpIndexURL(nil); got != "/pl/search-index.json" {
		t.Errorf("pl listing: %q", got)
	}
}

// TestWebMCPIndexURLIsAJavaScriptLiteral: the URL is interpolated into inline
// script source, so anything that could close the literal must not survive.
func TestWebMCPIndexURLIsAJavaScriptLiteral(t *testing.T) {
	got := injectWebMCP(`<html><body></body></html>`, `/a";alert(1);//search-index.json`)
	// The unescaped sequence would close the literal and start a statement.
	// Its escaped form is /a\";alert(1), which does not contain this.
	if strings.Contains(got, `/a";alert`) {
		t.Errorf("the index URL escaped its string literal:\n%s", got)
	}
	if !strings.Contains(got, `/a\";alert`) {
		t.Errorf("the quote was not escaped as expected:\n%s", got)
	}
	// A newline would end the statement even with the quote handled.
	if strings.Contains(injectWebMCP(`<html><body></body></html>`, "/a\nalert(1)"), "\nalert(1)") {
		t.Error("a newline in the index URL survived into the script")
	}
}
