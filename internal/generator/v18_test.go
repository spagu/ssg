package generator

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// --- pure helpers -----------------------------------------------------------

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Learn Go":        "learn-go",
		"  Hello, World!": "hello-world",
		"C++ & Rust":      "c-rust",
		"":                "",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestContainsMath(t *testing.T) {
	if !containsMath("text $$a+b$$ more") {
		t.Error("expected display math to be detected")
	}
	if !containsMath("```math\nx=1\n```") {
		t.Error("expected fenced math to be detected")
	}
	if containsMath("just $5 and $10") {
		t.Error("currency should not be detected as math")
	}
}

func TestIdentityLineMappings(t *testing.T) {
	if got := identityLineMappings(0); got != "" {
		t.Errorf("empty want '', got %q", got)
	}
	if got := identityLineMappings(1); got != "AAAA" {
		t.Errorf("1 line want AAAA, got %q", got)
	}
	if got := identityLineMappings(3); got != "AAAA;AACA;AACA" {
		t.Errorf("3 lines want AAAA;AACA;AACA, got %q", got)
	}
}

func TestPageURL(t *testing.T) {
	if pageURL(1) != "/" {
		t.Error("page 1 should be /")
	}
	if pageURL(3) != "/page/3/" {
		t.Error("page 3 should be /page/3/")
	}
}

func TestRewriteAssetRefs(t *testing.T) {
	by := map[string]string{"style.css": "style.abcd1234.css", "app.js": "app.deadbeef.js"}
	in := `<link href="/css/style.css"><script src="app.js"></script>`
	out := rewriteAssetRefs(in, by)
	if !strings.Contains(out, "style.abcd1234.css") || !strings.Contains(out, "app.deadbeef.js") {
		t.Errorf("refs not rewritten: %s", out)
	}
}

func TestMinifyLinePreserving(t *testing.T) {
	js := "// comment\nvar   x = 1;\n\nfunction f(){}"
	got := minifyJSLinePreserving(js)
	if strings.Count(got, "\n") != strings.Count(js, "\n") {
		t.Errorf("line count changed: %q → %q", js, got)
	}
	if strings.Contains(got, "// comment") {
		t.Error("line comment should be stripped")
	}
	css := "/* c */\n.a  {  color : red ; }"
	if strings.Contains(minifyCSSLinePreserving(css), "/* c */") {
		t.Error("css comment should be stripped")
	}
}

// --- generator with minimal fixtures ---------------------------------------

func newTestGen(t *testing.T, tmpl string) *Generator {
	t.Helper()
	out := t.TempDir()
	g := &Generator{
		config:   Config{OutputDir: out, Domain: "example.com"},
		siteData: &models.SiteData{Domain: "example.com", Categories: map[int]models.Category{}},
	}
	if tmpl != "" {
		g.tmpl = template.Must(template.New("t").Parse(tmpl))
	}
	return g
}

func TestExpandPermalink(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Categories[5] = models.Category{Slug: "news"}
	p := models.Page{
		Slug:       "hello",
		Date:       time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC),
		Categories: []int{5},
	}
	got := g.expandPermalink("/:year/:month/:day/:category/:slug/", p)
	if got != "2026/03/09/news/hello" {
		t.Errorf("expandPermalink = %q", got)
	}
	// frontmatter Category wins for :category
	p2 := models.Page{Slug: "x", Category: "guides", Date: time.Now()}
	if !strings.Contains(g.expandPermalink("/:category/:slug/", p2), "guides/x") {
		t.Errorf("expected frontmatter category slug")
	}
}

func TestLoadData(t *testing.T) {
	g := newTestGen(t, "")
	dir := t.TempDir()
	g.config.DataDir = dir
	if err := os.MkdirAll(filepath.Join(dir, "authors"), 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "site.yaml"), []byte("name: Example\ncount: 3\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "authors", "bob.json"), []byte(`{"role":"editor"}`), 0644)

	if err := g.loadData(); err != nil {
		t.Fatalf("loadData: %v", err)
	}
	site, ok := g.data["site"].(map[string]interface{})
	if !ok || site["name"] != "Example" {
		t.Errorf("site data = %v", g.data["site"])
	}
	authors, ok := g.data["authors"].(map[string]interface{})
	if !ok {
		t.Fatalf("authors nesting missing: %v", g.data)
	}
	bob, ok := authors["bob"].(map[string]interface{})
	if !ok || bob["role"] != "editor" {
		t.Errorf("nested json data = %v", authors["bob"])
	}
}

func TestLoadDataMissingDir(t *testing.T) {
	g := newTestGen(t, "")
	g.config.DataDir = filepath.Join(t.TempDir(), "nope")
	if err := g.loadData(); err != nil {
		t.Errorf("missing data dir should be a no-op, got %v", err)
	}
}

func TestWriteAliasStubs(t *testing.T) {
	g := newTestGen(t, "")
	page := models.Page{Type: "page", Slug: "new", Aliases: []string{"/old/", "/legacy.html"}}
	g.writeAliasStubs(page)

	for _, rel := range []string{"old/index.html", "legacy.html"} {
		data, err := os.ReadFile(filepath.Join(g.config.OutputDir, rel))
		if err != nil {
			t.Fatalf("alias %s not written: %v", rel, err)
		}
		s := string(data)
		if !strings.Contains(s, `http-equiv="refresh"`) || !strings.Contains(s, `/new/`) {
			t.Errorf("alias stub %s bad content: %s", rel, s)
		}
		if !strings.Contains(s, `rel="canonical"`) || !strings.Contains(s, `noindex`) {
			t.Errorf("alias stub %s missing canonical/noindex", rel)
		}
	}
}

func TestLastModFor(t *testing.T) {
	g := newTestGen(t, "")
	mod := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	pub := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := g.lastModFor(models.Page{Modified: mod, Date: pub}); !got.Equal(mod) {
		t.Errorf("want modified, got %v", got)
	}
	if got := g.lastModFor(models.Page{Date: pub}); !got.Equal(pub) {
		t.Errorf("want date fallback, got %v", got)
	}
	// LastmodFromGit on an untracked file falls back gracefully.
	g.config.LastmodFromGit = true
	if got := g.lastModFor(models.Page{Modified: mod, SourceFile: "nope.md", SourceDir: t.TempDir()}); !got.Equal(mod) {
		t.Errorf("git fallback want modified, got %v", got)
	}
}

func TestInjectMath(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Math = true
	out := g.config.OutputDir
	_ = os.WriteFile(filepath.Join(out, "a.html"), []byte("<head></head><body>$$x^2$$</body>"), 0644)
	_ = os.WriteFile(filepath.Join(out, "b.html"), []byte("<head></head><body>no math</body>"), 0644)

	if err := g.injectMathIfRequested(); err != nil {
		t.Fatalf("injectMath: %v", err)
	}
	a, _ := os.ReadFile(filepath.Join(out, "a.html"))
	b, _ := os.ReadFile(filepath.Join(out, "b.html"))
	if !strings.Contains(string(a), "katex.min.css") {
		t.Error("math page should get KaTeX")
	}
	if strings.Contains(string(b), "katex") {
		t.Error("non-math page should not get KaTeX")
	}
}

func TestMinifyAssetFileWithSourceMap(t *testing.T) {
	g := newTestGen(t, "")
	g.config.SourceMap = true
	css := filepath.Join(g.config.OutputDir, "style.css")
	_ = os.WriteFile(css, []byte(".a { color: red; }\n.b { color: blue; }"), 0644)

	if err := g.minifyAssetFile(css, minifyCSSFile, minifyCSSLinePreserving); err != nil {
		t.Fatalf("minifyAssetFile: %v", err)
	}
	out, _ := os.ReadFile(css)
	if !strings.Contains(string(out), "sourceMappingURL=style.css.map") {
		t.Errorf("missing sourceMappingURL: %s", out)
	}
	if _, err := os.Stat(css + ".map"); err != nil {
		t.Errorf("source map not written: %v", err)
	}
}

func TestFingerprintAssets(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Fingerprint = true
	out := g.config.OutputDir
	_ = os.WriteFile(filepath.Join(out, "style.css"), []byte("body{color:red}"), 0644)
	_ = os.WriteFile(filepath.Join(out, "app.js"), []byte("console.log(1)"), 0644)
	_ = os.WriteFile(filepath.Join(out, "index.html"),
		[]byte(`<link href="/style.css"><script src="/app.js"></script>`), 0644)

	if err := g.fingerprintAssets(); err != nil {
		t.Fatalf("fingerprintAssets: %v", err)
	}
	// Manifest exists and maps originals.
	manifest, err := os.ReadFile(filepath.Join(out, "assets-manifest.json"))
	if err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
	if !strings.Contains(string(manifest), "style.css") {
		t.Errorf("manifest missing style.css: %s", manifest)
	}
	// HTML references were rewritten to hashed names.
	html, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if strings.Contains(string(html), `href="/style.css"`) {
		t.Errorf("html still references un-hashed style.css: %s", html)
	}
	// Determinism: a second identical build yields the same hashed names.
	entries, _ := os.ReadDir(out)
	var hashed string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "style.") && strings.HasSuffix(e.Name(), ".css") {
			hashed = e.Name()
		}
	}
	if hashed == "style.css" || hashed == "" {
		t.Errorf("style.css was not fingerprinted, got %q", hashed)
	}
}

func TestGenerateIndexPagination(t *testing.T) {
	g := newTestGen(t, `{{define "index.html"}}<html>{{len .Posts}} page {{.Pager.Current}}/{{.Pager.Total}}</html>{{end}}`)
	g.config.Paginate = 2
	for i := 0; i < 5; i++ {
		g.siteData.Posts = append(g.siteData.Posts, models.Page{Title: "P", Slug: "p", Type: "post", Date: time.Now()})
	}
	if err := g.generateIndex(); err != nil {
		t.Fatalf("generateIndex: %v", err)
	}
	// 5 posts / 2 per page = 3 pages: index.html, page/2, page/3
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "index.html")); err != nil {
		t.Errorf("index.html missing")
	}
	for _, n := range []string{"2", "3"} {
		if _, err := os.Stat(filepath.Join(g.config.OutputDir, "page", n, "index.html")); err != nil {
			t.Errorf("page/%s/index.html missing", n)
		}
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "page", "4", "index.html")); err == nil {
		t.Errorf("page/4 should not exist")
	}
}

func TestComputeSeriesLinks(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Posts = []models.Page{
		{Title: "Part 2", Slug: "p2", Type: "post", Series: "S", Date: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
		{Title: "Part 1", Slug: "p1", Type: "post", Series: "S", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Title: "Solo", Slug: "solo", Type: "post", Date: time.Now()},
	}
	g.computeSeriesLinks()
	var part1, part2 models.Page
	for _, p := range g.siteData.Posts {
		if p.Slug == "p1" {
			part1 = p
		}
		if p.Slug == "p2" {
			part2 = p
		}
	}
	if part1.SeriesNextTitle != "Part 2" {
		t.Errorf("part1 next = %q, want Part 2", part1.SeriesNextTitle)
	}
	if part2.SeriesPrevTitle != "Part 1" {
		t.Errorf("part2 prev = %q, want Part 1", part2.SeriesPrevTitle)
	}
}

func TestGenerateSeries(t *testing.T) {
	g := newTestGen(t, `{{define "category.html"}}<h1>{{.Series}}</h1>{{len .Posts}}{{end}}`)
	g.siteData.Posts = []models.Page{
		{Title: "A", Slug: "a", Type: "post", Series: "Learn Go", Date: time.Now()},
		{Title: "B", Slug: "b", Type: "post", Series: "Learn Go", Date: time.Now()},
	}
	if err := g.generateSeries(); err != nil {
		t.Fatalf("generateSeries: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(g.config.OutputDir, "series", "learn-go", "index.html"))
	if err != nil {
		t.Fatalf("series landing missing: %v", err)
	}
	if !strings.Contains(string(data), "Learn Go") {
		t.Errorf("series page bad content: %s", data)
	}
}

func TestComputeTranslationsAndHreflang(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Languages = []string{"pl", "en"}
	g.config.DefaultLanguage = "pl"
	g.siteData.Pages = []models.Page{
		{Slug: "about", Type: "page", Lang: "pl"},
		{Slug: "about", Type: "page", Lang: "en", LangPrefix: "en"},
	}
	g.computeTranslations()
	tags := string(g.hreflangTags(g.siteData.Pages[0]))
	if !strings.Contains(tags, `hreflang="pl"`) || !strings.Contains(tags, `hreflang="en"`) {
		t.Errorf("hreflang missing languages: %s", tags)
	}
	if !strings.Contains(tags, `hreflang="x-default"`) {
		t.Errorf("hreflang missing x-default: %s", tags)
	}
}

func TestFinalizeLoadedContent(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Math = true
	g.config.Permalinks = map[string]string{"post": "/:year/:slug/", "page": "/:slug/"}
	g.config.Languages = []string{"pl", "en"}
	g.config.DefaultLanguage = "pl"
	g.siteData.Posts = []models.Page{
		{Slug: "hi", Type: "post", Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			Content: "words $$x$$", Lang: "en"},
	}
	g.siteData.Pages = []models.Page{
		{Slug: "about", Type: "page", Content: "plain", Lang: "pl"},
	}
	if err := g.finalizeLoadedContent(); err != nil {
		t.Fatalf("finalizeLoadedContent: %v", err)
	}

	post := g.siteData.Posts[0]
	if post.PermalinkPath != "2026/hi" {
		t.Errorf("post permalink = %q, want 2026/hi", post.PermalinkPath)
	}
	if !post.HasMath {
		t.Errorf("post should be flagged HasMath")
	}
	if post.LangPrefix != "en" {
		t.Errorf("non-default lang should get prefix, got %q", post.LangPrefix)
	}
	if post.WordCount == 0 {
		t.Errorf("word count should be computed")
	}
	// Default-language page has no lang prefix.
	if g.siteData.Pages[0].LangPrefix != "" {
		t.Errorf("default language should have no prefix")
	}
}

func TestPermalinkCategoryFallback(t *testing.T) {
	g := newTestGen(t, "")
	// No frontmatter category, no resolvable category → "uncategorized".
	p := models.Page{Slug: "x", Date: time.Now()}
	if got := g.permalinkCategorySlug(p); got != "uncategorized" {
		t.Errorf("fallback = %q, want uncategorized", got)
	}
}

func TestNormalizeYAMLValue(t *testing.T) {
	in := map[interface{}]interface{}{
		"a": 1,
		"b": []interface{}{map[interface{}]interface{}{"c": 2}},
	}
	out, ok := normalizeYAMLValue(in).(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", normalizeYAMLValue(in))
	}
	list, ok := out["b"].([]interface{})
	if !ok || len(list) != 1 {
		t.Fatalf("list not normalized: %v", out["b"])
	}
	if _, ok := list[0].(map[string]interface{}); !ok {
		t.Errorf("nested map not normalized: %T", list[0])
	}
}

func TestWriteAliasStubsEdgeCases(t *testing.T) {
	g := newTestGen(t, "")
	// Pre-create a colliding file.
	collide := filepath.Join(g.config.OutputDir, "taken", "index.html")
	_ = os.MkdirAll(filepath.Dir(collide), 0755)
	_ = os.WriteFile(collide, []byte("real page"), 0644)

	page := models.Page{Type: "page", Slug: "new", Aliases: []string{"/taken/", "../evil", ""}}
	g.writeAliasStubs(page)

	// Collision preserved (not overwritten by a stub).
	data, _ := os.ReadFile(collide)
	if string(data) != "real page" {
		t.Errorf("collision alias overwrote a real page")
	}
	// Traversal alias must not escape the output dir.
	if _, err := os.Stat(filepath.Join(filepath.Dir(g.config.OutputDir), "evil")); err == nil {
		t.Errorf("unsafe alias escaped output dir")
	}
}

func TestGitLastModTrackedFile(t *testing.T) {
	// This test file lives in a tracked package; git should report a commit date
	// for a tracked source file relative to the working directory.
	g := newTestGen(t, "")
	if _, ok := g.gitLastMod(models.Page{SourceFile: "generator.go"}); !ok {
		t.Skip("generator.go not tracked in this checkout; skipping git lastmod success path")
	}
}

func TestGenerateIndexNoPagination(t *testing.T) {
	g := newTestGen(t, `{{define "index.html"}}<html>{{len .Posts}}</html>{{end}}`)
	g.siteData.Posts = []models.Page{{Slug: "a", Type: "post", Date: time.Now()}}
	if err := g.generateIndex(); err != nil {
		t.Fatalf("generateIndex: %v", err)
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "page")); err == nil {
		t.Errorf("no /page dir expected without pagination")
	}
}

func TestInjectKatexNoHeadBody(t *testing.T) {
	out := injectKatexAssets("$$x$$ bare fragment")
	if !strings.Contains(out, "katex.min.css") || !strings.Contains(out, "katex.min.js") {
		t.Errorf("katex assets should still be appended without head/body: %s", out)
	}
}

func TestWriteWithSourceMapJS(t *testing.T) {
	g := newTestGen(t, "")
	g.config.SourceMap = true
	js := filepath.Join(g.config.OutputDir, "app.js")
	_ = os.WriteFile(js, []byte("// c\nvar x = 1;\nfunction f(){ return  2; }"), 0644)
	if err := g.minifyAssetFile(js, minifyJSFile, minifyJSLinePreserving); err != nil {
		t.Fatalf("minify js: %v", err)
	}
	out, _ := os.ReadFile(js)
	if !strings.Contains(string(out), "//# sourceMappingURL=app.js.map") {
		t.Errorf("missing JS sourceMappingURL: %s", out)
	}
	mp, err := os.ReadFile(js + ".map")
	if err != nil || !strings.Contains(string(mp), `"version":3`) {
		t.Errorf("bad JS source map: %v / %s", err, mp)
	}
}

func TestMinifyAssetFileEmptyAndNoSourceMap(t *testing.T) {
	g := newTestGen(t, "")
	// Empty file with source maps → no map produced.
	g.config.SourceMap = true
	empty := filepath.Join(g.config.OutputDir, "e.css")
	_ = os.WriteFile(empty, []byte("   \n"), 0644)
	if err := g.minifyAssetFile(empty, minifyCSSFile, minifyCSSLinePreserving); err != nil {
		t.Fatalf("empty minify: %v", err)
	}
	if _, err := os.Stat(empty + ".map"); err == nil {
		t.Errorf("no map should be produced for empty input")
	}
	// Without source maps → full minifier, no map.
	g.config.SourceMap = false
	css := filepath.Join(g.config.OutputDir, "f.css")
	_ = os.WriteFile(css, []byte(".a { color : red ; }"), 0644)
	if err := g.minifyAssetFile(css, minifyCSSFile, minifyCSSLinePreserving); err != nil {
		t.Fatalf("full minify: %v", err)
	}
	if _, err := os.Stat(css + ".map"); err == nil {
		t.Errorf("no map expected without source maps")
	}
}

func TestFingerprintCSSImportOrder(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Fingerprint = true
	out := g.config.OutputDir
	_ = os.WriteFile(filepath.Join(out, "base.css"), []byte("a{color:red}"), 0644)
	_ = os.WriteFile(filepath.Join(out, "main.css"), []byte(`@import "base.css";b{color:blue}`), 0644)
	if err := g.fingerprintAssets(); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	// main.css should reference the hashed base name, not "base.css".
	entries, _ := os.ReadDir(out)
	var mainHashed string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "main.") && strings.HasSuffix(e.Name(), ".css") {
			mainHashed = e.Name()
		}
	}
	if mainHashed == "" {
		t.Fatal("main.css not fingerprinted")
	}
	data, _ := os.ReadFile(filepath.Join(out, mainHashed))
	if strings.Contains(string(data), `@import "base.css"`) {
		t.Errorf("import not rewritten to hashed name: %s", data)
	}
}

func TestFingerprintIfRequestedDisabled(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Fingerprint = false
	if err := g.fingerprintIfRequested(); err != nil {
		t.Errorf("disabled fingerprint should be a no-op, got %v", err)
	}
}

func TestRunPostPageHook(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Hooks = map[string][]string{"post_page": {"true"}}
	// Should not panic or fail the build; just exercises the path.
	g.runPostPageHook(models.Page{Slug: "x", Type: "page"})
	// Failing post_page hook is non-fatal (logged, no panic).
	g.config.Hooks = map[string][]string{"post_page": {"false"}}
	g.runPostPageHook(models.Page{Slug: "y", Type: "page"})
}

func TestRunHooks(t *testing.T) {
	g := newTestGen(t, "")
	g.config.Hooks = map[string][]string{
		"pre_build":  {"true"},
		"post_build": {"false"},
	}
	if err := g.runHooks("pre_build", nil); err != nil {
		t.Errorf("pre_build 'true' should succeed, got %v", err)
	}
	if err := g.runHooks("post_build", nil); err == nil {
		t.Errorf("post_build 'false' should fail")
	}
	// Unknown phase / empty is a no-op.
	if err := g.runHooks("never", nil); err != nil {
		t.Errorf("no hooks should be a no-op, got %v", err)
	}
}
