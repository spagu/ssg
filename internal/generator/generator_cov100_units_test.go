package generator

// Edge paths of generator.go reached directly: loaders that meet an
// unreadable directory, templates that fail twice, template sets that have
// already executed, git history that cannot be read, and fingerprinting over
// an output tree it cannot read or write.

import (
	"bytes"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/engine"
	"github.com/spagu/ssg/internal/models"
)

// failingTmpl fails at execution: an int cannot be indexed.
const failingTmpl = `{{index 1 0}}`

// TestRenderArchiveBothTemplatesFail: the primary and the category.html
// fallback both fail at execution; the archive is reported, not fatal.
func TestRenderArchiveBothTemplatesFail(t *testing.T) {
	g := newTestGen(t, `{{define "tag.html"}}`+failingTmpl+`{{end}}{{define "category.html"}}`+failingTmpl+`{{end}}`)
	out := captureGeneratorStdout(t, func() {
		if err := g.renderArchive("tag", "Go", "go", nil, tagHTMLName, false); err != nil {
			t.Errorf("a double template failure must warn, not abort: %v", err)
		}
	})
	if !strings.Contains(out, "Failed to generate tag Go") {
		t.Errorf("the failure must be reported:\n%s", out)
	}
}

// TestGenerateCategoriesRenderFailure: a category.html that fails at
// execution stops that category's pages with a warning.
func TestGenerateCategoriesRenderFailure(t *testing.T) {
	g := newTestGen(t, `{{define "category.html"}}`+failingTmpl+`{{end}}`)
	g.siteData.Categories[2] = models.Category{ID: 2, Name: "News", Slug: "news"}
	g.siteData.Posts = []models.Page{{Slug: "p", Categories: []int{2}}}
	out := captureGeneratorStdout(t, func() {
		if err := g.generateCategories(); err != nil {
			t.Errorf("a render failure must warn, not abort: %v", err)
		}
	})
	if !strings.Contains(out, "failed to generate category news") || g.categoryArchives[2] != "" {
		t.Errorf("an unwritten archive must be reported and kept out of the sitemap:\n%s", out)
	}
}

// TestLoadContentFromMddbClientAndSourceErrors: a client that refuses its
// configuration, and a source the build cannot read, both fail the load.
func TestLoadContentFromMddbClientAndSourceErrors(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Mddb = MddbConfig{Enabled: true, Protocol: "grpc", URL: "grpc://example.com:1", APIKey: "k", Timeout: 1}
	if err := g.loadContentFromMddb(); err == nil || !strings.Contains(err.Error(), "creating mddb client") {
		t.Errorf("an API key over insecure remote gRPC must be refused: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	g = newTestGen(t, "")
	g.config.Mddb = MddbConfig{Enabled: true, Protocol: "http", URL: srv.URL, Timeout: 5,
		Sources: []MddbSource{{Collection: " "}}}
	if err := g.loadContentFromMddb(); err == nil || !strings.Contains(err.Error(), "collection is required") {
		t.Errorf("a source without a collection must fail: %v", err)
	}
}

// TestLoadersSkipUnreadableDirectories: a subdirectory that cannot be read is
// a warning; the rest of the tree still loads.
func TestLoadersSkipUnreadableDirectories(t *testing.T) {
	skipIfRoot(t)
	g := newTestGen(t, "")
	page := "---\ntitle: A\nslug: a\nstatus: publish\ntype: page\n---\n\nA.\n"

	pages := t.TempDir()
	mustWrite(t, filepath.Join(pages, "a.md"), page)
	mustMkdir(t, filepath.Join(pages, "locked"))
	lockDir(t, filepath.Join(pages, "locked"), 0o000)
	var got []models.Page
	out := captureGeneratorStdout(t, func() {
		var err error
		if got, err = g.loadMarkdownDir(pages); err != nil {
			t.Fatal(err)
		}
	})
	if len(got) != 1 || !strings.Contains(out, "failed to load subdirectory locked") {
		t.Errorf("pages = %d, output:\n%s", len(got), out)
	}

	posts := t.TempDir()
	mustWrite(t, filepath.Join(posts, "news", "a.md"), strings.Replace(page, "type: page", "type: post", 1))
	mustMkdir(t, filepath.Join(posts, "locked"))
	lockDir(t, filepath.Join(posts, "locked"), 0o000)
	out = captureGeneratorStdout(t, func() {
		var err error
		if got, err = g.loadPostsDir(posts); err != nil {
			t.Fatal(err)
		}
	})
	if len(got) != 1 || !strings.Contains(out, "failed to load category locked") {
		t.Errorf("posts = %d, output:\n%s", len(got), out)
	}
}

// executedTemplate returns a set that has already rendered, which html/template
// refuses to clone.
func executedTemplate(t *testing.T) *template.Template {
	t.Helper()
	tmpl := template.Must(template.New("x").Parse(`x`))
	if err := tmpl.Execute(&bytes.Buffer{}, nil); err != nil {
		t.Fatal(err)
	}
	return tmpl
}

// TestShortcodeSetsFromAnExecutedTemplate: a theme set that cannot be cloned
// leaves shortcodes to parse standalone.
func TestShortcodeSetsFromAnExecutedTemplate(t *testing.T) {
	if shortcodeBaseOf(executedTemplate(t), template.FuncMap{}) != nil {
		t.Error("an executed set cannot be cloned, so there is no base")
	}
	g := &Generator{shortcodeBase: executedTemplate(t)}
	if g.shortcodeSet() != nil {
		t.Error("an executed base cannot be cloned, so there is no set")
	}
}

// TestParseShortcodeTemplateIntoThemeSetError: a malformed shortcode parsed
// into the theme's namespace is a warning and no template.
func TestParseShortcodeTemplateIntoThemeSetError(t *testing.T) {
	g := &Generator{shortcodeBase: template.New("theme")}
	path := filepath.Join(t.TempDir(), "bad.html")
	mustWrite(t, path, "{{end}}")
	out := captureGeneratorStdout(t, func() {
		if g.parseShortcodeTemplate(path) != nil {
			t.Error("a malformed shortcode must not parse")
		}
	})
	if !strings.Contains(out, "shortcode template parse error") {
		t.Errorf("the parse error must be reported:\n%s", out)
	}
}

// TestRenderWithEngineExecuteError: an alt-engine template failing at
// execution returns the error and writes nothing.
func TestRenderWithEngineExecuteError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "page.html")
	mustWrite(t, src, failingTmpl)
	tmpl, err := engine.NewGoEngine().ParseFile(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	g := newTestGen(t, "")
	g.engineTmpls = map[string]engine.Template{"page.html": tmpl}
	outPath := filepath.Join(g.config.OutputDir, "page.html")
	if err := g.renderWithEngine("page.html", outPath, map[string]any{}, nil, false); err == nil {
		t.Error("an execution error must be returned")
	}
	if _, err := os.Stat(outPath); err == nil {
		t.Error("a failed render must write nothing")
	}
}

// TestBuildMdLinkMapIgnoresEmptyKeys: a page without a slug adds no "" key.
func TestBuildMdLinkMapIgnoresEmptyKeys(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Pages = []models.Page{{Title: "No slug", Link: "/x/"}}
	if _, ok := g.buildMdLinkMap()[""]; ok {
		t.Error("an empty key must not be indexed")
	}
}

// TestHasTranslationHelper: the helper reports a translation only when the
// page has one in that language.
func TestHasTranslationHelper(t *testing.T) {
	g := edgeGen(t, Config{})
	has := g.buildTemplateFuncs(nil)["hasTranslation"].(func(string, any) bool)
	page := models.Page{Translations: []models.TranslationLink{{Lang: "pl", URL: "/pl/a/"}}}
	if !has("pl", page) || has("de", page) {
		t.Error("hasTranslation must follow the page's translations")
	}
}

// TestSafeHTMLStripsMdLinkText: with strip_md_link_text the visible ".md"
// goes, the link itself stays.
func TestSafeHTMLStripsMdLinkText(t *testing.T) {
	g := bareRenderGen()
	g.config.StripMdLinkText = true
	got := string(g.tmplSafeHTML(map[string]string{}, map[string]map[string]string{})("[guide.md](guide.md)"))
	if strings.Contains(got, ">guide.md<") || !strings.Contains(got, `href="guide.md"`) {
		t.Errorf("got %s", got)
	}
}

// stringerValue is a fmt.Stringer that is neither a string nor HTML.
type stringerValue struct{}

func (stringerValue) String() string { return "<b>x</b>" }

// TestSafeHTMLValueStringer: a Stringer renders through its String().
func TestSafeHTMLValueStringer(t *testing.T) {
	if got := (&Generator{}).safeHTMLValue(stringerValue{}); got != "<b>x</b>" {
		t.Errorf("got %q", got)
	}
}

// TestEnsureTemplatesWriteFailures: each scaffold step reports a failure to
// write — the embedded theme, a template file, and the theme assets.
func TestEnsureTemplatesWriteFailures(t *testing.T) {
	embedded := t.TempDir()
	mustWrite(t, filepath.Join(embedded, "css"), "not a dir")
	g := &Generator{config: Config{Template: "simple", Quiet: true}}
	if err := g.ensureTemplates(embedded); err == nil || !strings.Contains(err.Error(), "extracting embedded theme") {
		t.Errorf("embedded theme: %v", err)
	}

	blocked := t.TempDir()
	mustMkdir(t, filepath.Join(blocked, pageHTMLName))
	g = &Generator{config: Config{Template: "no-such-theme", Quiet: true}}
	if err := g.ensureTemplates(blocked); err == nil || !strings.Contains(err.Error(), "creating template") {
		t.Errorf("template file: %v", err)
	}

	assets := t.TempDir()
	mustWrite(t, filepath.Join(assets, "css"), "not a dir")
	if err := g.ensureTemplates(assets); err == nil || !strings.Contains(err.Error(), "creating css") {
		t.Errorf("theme assets: %v", err)
	}
}

// TestAssetDirEntriesIsMemoized: the second read of a directory is the first
// one's answer, even after the directory changed.
func TestAssetDirEntriesIsMemoized(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.png"), "x")
	g := &Generator{}
	first := g.assetDirEntries(dir)
	mustWrite(t, filepath.Join(dir, "b.png"), "x")
	if second := g.assetDirEntries(dir); len(first) != 1 || len(second) != 1 {
		t.Errorf("entries = %d then %d, want the memoized 1", len(first), len(second))
	}
}

// TestGitLastModWithoutWorkingDirectory: a relative path cannot be made
// absolute once the working directory is gone, so there is no date.
func TestGitLastModWithoutWorkingDirectory(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	mustMkdir(t, gone)
	t.Chdir(gone)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform still resolves a removed working directory")
	}
	g := &Generator{}
	g.gitOnce.Do(func() {
		g.gitRoot = "/"
		g.gitTimes = map[string]time.Time{"a.md": time.Now()}
	})
	if _, ok := g.gitLastModForFile("a.md"); ok {
		t.Error("no working directory, no date")
	}
}

// TestLoadGitLastModTimesFailures: outside a repository, and in one without
// commits, the history is empty rather than an error.
func TestLoadGitLastModTimesFailures(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)
	if root, times := loadGitLastModTimes(); root != "" || times != nil {
		t.Errorf("outside a repository: %q %v", root, times)
	}
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}
	if root, times := loadGitLastModTimes(); root != "" || times != nil {
		t.Errorf("a repository without commits: %q %v", root, times)
	}
}

// TestWriteSitemapsOutsideTheOutput: with no output root every sitemap path
// is out of bounds and refused.
func TestWriteSitemapsOutsideTheOutput(t *testing.T) {
	g := edgeGen(t, Config{})
	g.config.OutputDir = ""
	if err := g.writeSitemaps([]sitemapEntry{{loc: "https://example.com/"}}); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("err = %v", err)
	}
}

// TestFingerprintAssetsIOErrors: every read and write of the fingerprint pass
// reports its failure — the walk, the reference scan, the hashed copy and the
// HTML rewrite.
func TestFingerprintAssetsIOErrors(t *testing.T) {
	skipIfRoot(t)
	fingerprint := func(setup func(out string)) error {
		out := t.TempDir()
		setup(out)
		g := &Generator{config: Config{OutputDir: out, Quiet: true, Domain: "example.com"}}
		return g.fingerprintAssets()
	}
	cases := map[string]func(out string){
		"unreadable directory": func(out string) {
			mustMkdir(t, filepath.Join(out, "locked"))
			lockDir(t, filepath.Join(out, "locked"), 0o000)
		},
		"unreadable asset": func(out string) {
			css := filepath.Join(out, "a.css")
			mustWrite(t, css, "body{}")
			lockDir(t, css, 0o000)
		},
		"read-only asset directory": func(out string) {
			dir := filepath.Join(out, "css")
			mustWrite(t, filepath.Join(dir, "a.css"), "body{}")
			lockDir(t, dir, 0o555)
		},
		"unreadable page": func(out string) {
			page := filepath.Join(out, "index.html")
			mustWrite(t, page, "<html></html>")
			lockDir(t, page, 0o000)
		},
	}
	for name, setup := range cases {
		if err := fingerprint(setup); err == nil {
			t.Errorf("%s: fingerprinting must fail", name)
		}
	}
}
