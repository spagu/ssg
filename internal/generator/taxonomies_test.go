package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/engine"
	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/taxonomy"
)

func taxBool(v bool) *bool { return &v }

// writeTaxonomyTemplates writes the core theme plus taxonomy templates that
// exercise the index/term contexts and every template helper.
func writeTaxonomyTemplates(t *testing.T, tmplDir string) {
	t.Helper()
	writeSimpleTemplates(t, tmplDir)
	mustWrite(t, filepath.Join(tmplDir, "taxonomy.html"),
		`{{define "taxonomy.html"}}<html><body><h1>{{.Taxonomy.Label}}</h1>`+
			`{{range .Terms}}<a href="{{.URL}}">{{.Name}} ({{.Count}})</a>{{end}}`+
			`|helpers:{{range taxonomies}}[{{.Name}}]{{end}}{{termURL "technology" "Go"}}`+
			`</body></html>{{end}}`)
	mustWrite(t, filepath.Join(tmplDir, "taxonomy-term.html"),
		`{{define "taxonomy-term.html"}}<html><body><h1>{{.Taxonomy.Singular}}: {{.Term.Name}}</h1>`+
			`<p>{{.Term.Description}}</p>{{range .Posts}}<article>{{.Title}}</article>{{end}}`+
			`|pager:{{.Pager.Current}}/{{.Pager.Total}}|prev:{{.Pager.PrevURL}}|next:{{.Pager.NextURL}}`+
			`{{with index .Posts 0}}|pt:{{range pageTerms "technology" .}}{{.Slug}},{{end}}`+
			`|has:{{hasTerm "technology" "go" .}}{{end}}`+
			`|by:{{range pagesByTerm "technology" .Term.Name}}{{.Slug}},{{end}}`+
			`</body></html>{{end}}`)
}

// writeTaxonomyMeta writes the minimal metadata.json loadContent requires.
func writeTaxonomyMeta(t *testing.T, tmp string) {
	t.Helper()
	mustWrite(t, filepath.Join(tmp, "content", "site", "metadata.json"),
		`{"categories":[],"exported_at":"","media":[],"users":[]}`)
}

// taxonomyTestConfig builds a Generate-ready config over tmp with the fixture
// taxonomies: technology (multiple, feed), difficulty (single), platform.
func taxonomyTestConfig(tmp string) Config {
	return Config{
		Source:       "site",
		Template:     "simple",
		Domain:       "example.com",
		ContentDir:   filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir:    filepath.Join(tmp, "output"),
		DataDir:      filepath.Join(tmp, "data"),
		Feed:         true,
		SearchIndex:  true,
		Quiet:        true,
		Taxonomies: map[string]taxonomy.DefinitionConfig{
			"technology": {Feed: taxBool(true)},
			"difficulty": {Multiple: taxBool(false), Singular: "Level"},
			"platform":   {},
		},
	}
}

func writeTaxonomyPost(t *testing.T, dir, slug, title, extra string) {
	t.Helper()
	mustWrite(t, filepath.Join(dir, slug+".md"),
		"---\ntitle: "+title+"\nslug: "+slug+"\nstatus: publish\ntype: post\ndate: 2024-01-0"+
			string(rune('1'+len(slug)%8))+"\n"+extra+"---\n\nBody of "+title+".\n")
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test file under t.TempDir()
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// wantContains asserts every want substring appears in content.
func wantContains(t *testing.T, label, content string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(content, w) {
			t.Errorf("%s missing %q in %s", label, w, content)
		}
	}
}

// wantFiles asserts every path exists.
func wantFiles(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing file %s", p)
		}
	}
}

// TestTaxonomyFullBuild drives Generate() over the dynamic-taxonomies fixture
// shape and asserts archives, term metadata, feeds, sitemap, search index,
// legacy tag sync and every template helper.
func TestTaxonomyFullBuild(t *testing.T) {
	tmp := t.TempDir()
	postsDir := filepath.Join(tmp, "content", "site", "posts", "news")
	writeTaxonomyPost(t, postsDir, "one", "One",
		"technology: [Go, Rust]\ndifficulty: Beginner\ntaxonomies:\n  platform: [Linux, macOS]\n  tag: [cli]\n")
	writeTaxonomyPost(t, postsDir, "two", "Two",
		"technology: [go]\ndifficulty: Advanced\n")
	mustWrite(t, filepath.Join(tmp, "data", "taxonomies", "technology.yaml"),
		"go:\n  description: The Go programming language\n  weight: 5\nrust:\n  name: Rust Lang\n")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))

	gen, err := New(taxonomyTestConfig(tmp))
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := filepath.Join(tmp, "output")

	// Index page: term list with counts, case-variant merged, metadata rename.
	wantContains(t, "technology index", mustRead(t, filepath.Join(out, "technology", "index.html")),
		"<h1>Technology</h1>", `<a href="/technology/go/">Go (2)</a>`,
		`<a href="/technology/rust/">Rust Lang (1)</a>`,
		"[category][tag][series][difficulty][platform][technology]", "/technology/go/")

	// Term archive: posts, metadata description, helpers (pageTerms/hasTerm/pagesByTerm).
	wantContains(t, "go term", mustRead(t, filepath.Join(out, "technology", "go", "index.html")),
		"Technology: Go", "The Go programming language", "<article>One</article>",
		"<article>Two</article>", "|has:true", "pt:go,", "by:one,two,")

	// Single-value taxonomy and taxonomies-map values get archives too.
	wantFiles(t,
		filepath.Join(out, "difficulty", "beginner", "index.html"),
		filepath.Join(out, "difficulty", "advanced", "index.html"),
		filepath.Join(out, "platform", "linux", "index.html"),
		filepath.Join(out, "platform", "macos", "index.html"),
		filepath.Join(out, "tag", "cli", "index.html"), // generic → legacy sync
	)

	// Feed only for feed:true taxonomies.
	wantContains(t, "term feed", mustRead(t, filepath.Join(out, "technology", "go", "feed.xml")),
		"<title>Go</title>", "<title>One</title>")
	if _, err := os.Stat(filepath.Join(out, "platform", "linux", "feed.xml")); err == nil {
		t.Error("platform must not emit feeds (feed:false default)")
	}

	// Sitemap: index + terms for sitemap:true taxonomies.
	wantContains(t, "sitemap", mustRead(t, filepath.Join(out, "sitemap.xml")),
		"<loc>https://example.com/technology/</loc>",
		"<loc>https://example.com/technology/go/</loc>",
		"<loc>https://example.com/difficulty/beginner/</loc>")

	// Search index carries the taxonomies map.
	wantContains(t, "search index", mustRead(t, filepath.Join(out, "search-index.json")),
		`"taxonomies"`, `"platform"`)
}

// TestTaxonomySingleValueViolation: two values on a single-value taxonomy fail the build.
func TestTaxonomySingleValueViolation(t *testing.T) {
	tmp := t.TempDir()
	writeTaxonomyPost(t, filepath.Join(tmp, "content", "site", "posts", "news"), "bad", "Bad",
		"difficulty: [Beginner, Advanced]\n")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	gen, err := New(taxonomyTestConfig(tmp))
	if err != nil {
		t.Fatal(err)
	}
	err = gen.Generate()
	if err == nil || !strings.Contains(err.Error(), "single-value") {
		t.Fatalf("err = %v", err)
	}
}

// TestTaxonomySlugCollision: distinct terms slugifying identically fail the build.
func TestTaxonomySlugCollision(t *testing.T) {
	tmp := t.TempDir()
	writeTaxonomyPost(t, filepath.Join(tmp, "content", "site", "posts", "news"), "clash", "Clash",
		"technology: [\"C++\", \"C--\"]\n")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	gen, err := New(taxonomyTestConfig(tmp))
	if err != nil {
		t.Fatal(err)
	}
	err = gen.Generate()
	if err == nil || !strings.Contains(err.Error(), "collide") {
		t.Fatalf("err = %v", err)
	}
}

// TestTaxonomyURLCollisionWithPage: a page already owning /technology/ fails the build.
func TestTaxonomyURLCollisionWithPage(t *testing.T) {
	tmp := t.TempDir()
	writeTaxonomyPost(t, filepath.Join(tmp, "content", "site", "posts", "news"), "one", "One",
		"technology: [Go]\n")
	mustWrite(t, filepath.Join(tmp, "content", "site", "pages", "technology.md"),
		"---\ntitle: Tech page\nslug: technology\nstatus: publish\ntype: page\n---\n\nStatic.\n")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	gen, err := New(taxonomyTestConfig(tmp))
	if err != nil {
		t.Fatal(err)
	}
	err = gen.Generate()
	if err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("err = %v", err)
	}
}

// TestTaxonomyReservedPath: claiming a reserved segment (author) fails the build.
func TestTaxonomyReservedPath(t *testing.T) {
	tmp := t.TempDir()
	writeTaxonomyPost(t, filepath.Join(tmp, "content", "site", "posts", "news"), "one", "One", "")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	cfg := taxonomyTestConfig(tmp)
	cfg.Taxonomies["writers"] = taxonomy.DefinitionConfig{Path: "author"}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = gen.Generate()
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("err = %v", err)
	}
}

// TestTaxonomyPagination: paginate=1 with two posts in a term yields /page/2/.
func TestTaxonomyPagination(t *testing.T) {
	tmp := t.TempDir()
	postsDir := filepath.Join(tmp, "content", "site", "posts", "news")
	writeTaxonomyPost(t, postsDir, "one", "One", "technology: [Go]\n")
	writeTaxonomyPost(t, postsDir, "two", "Two", "technology: [Go]\n")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	cfg := taxonomyTestConfig(tmp)
	cfg.Paginate = 1
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	first := mustRead(t, filepath.Join(tmp, "output", "technology", "go", "index.html"))
	second := mustRead(t, filepath.Join(tmp, "output", "technology", "go", "page", "2", "index.html"))
	if !strings.Contains(first, "pager:1/2") || !strings.Contains(first, "next:/technology/go/page/2/") {
		t.Errorf("first page pager: %s", first)
	}
	if !strings.Contains(second, "pager:2/2") || !strings.Contains(second, "prev:/technology/go/") {
		t.Errorf("second page pager: %s", second)
	}
}

// TestTaxonomyI18nBuckets: multilingual builds scope custom archives per language.
func TestTaxonomyI18nBuckets(t *testing.T) {
	tmp := t.TempDir()
	postsDir := filepath.Join(tmp, "content", "site", "posts", "news")
	writeTaxonomyPost(t, postsDir, "sieci", "Sieci",
		"technology: [Sieci Neuronowe]\nlang: pl\ntranslation_key: nets\n")
	writeTaxonomyPost(t, postsDir, "networks", "Networks",
		"technology: [Neural Networks]\nlang: en\ntranslation_key: nets\n")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	cfg := taxonomyTestConfig(tmp)
	cfg.Languages = []string{"pl", "en"}
	cfg.DefaultLanguage = "pl"
	cfg.I18n.Enabled = true
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := filepath.Join(tmp, "output")
	pl := mustRead(t, filepath.Join(out, "technology", "index.html"))
	en := mustRead(t, filepath.Join(out, "en", "technology", "index.html"))
	if !strings.Contains(pl, "Sieci Neuronowe") || strings.Contains(pl, "Neural Networks") {
		t.Errorf("pl bucket leaked: %s", pl)
	}
	if !strings.Contains(en, `<a href="/en/technology/neural-networks/">Neural Networks (1)</a>`) {
		t.Errorf("en bucket: %s", en)
	}
	if _, err := os.Stat(filepath.Join(out, "en", "technology", "neural-networks", "index.html")); err != nil {
		t.Error("missing prefixed term archive")
	}
}

// TestTaxonomyChainsAndRenderFallbacks covers explicit template overrides, the
// engine-based template lookup and renderTaxonomyPage's guard branches.
func TestTaxonomyChainsAndRenderFallbacks(t *testing.T) {
	g, err := New(Config{Domain: "example.com", OutputDir: filepath.Join(t.TempDir(), "out")})
	if err != nil {
		t.Fatal(err)
	}
	def := taxonomy.Definition{Name: "tech", Template: "my.html", TermTemplate: "my-term.html"}
	if chain := g.taxonomyIndexChain(def); chain[0] != "my.html" || chain[1] != "taxonomy-tech.html" {
		t.Fatalf("index chain = %v", chain)
	}
	if chain := g.taxonomyTermChain(def); chain[0] != "my-term.html" || chain[1] != "taxonomy-tech-term.html" {
		t.Fatalf("term chain = %v", chain)
	}
	// No template in the chain exists: warning path, no error, no file.
	out := filepath.Join(g.config.OutputDir, "tech", "index.html")
	if err := g.renderTaxonomyPage([]string{"missing.html"}, out, nil); err != nil {
		t.Fatalf("missing-template chain must not error: %v", err)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("no file expected without templates")
	}
	// Unsafe output path is skipped, not fatal.
	if err := g.renderTaxonomyPage([]string{"missing.html"}, "/etc/ssg-test/index.html", nil); err != nil {
		t.Fatalf("unsafe path must be skipped: %v", err)
	}
	// Engine-backed template lookup branch.
	eng, err := engine.New("pongo2")
	if err != nil {
		t.Fatal(err)
	}
	g.engine = eng
	g.engineTmpls = map[string]engine.Template{}
	if g.hasTemplate("nope.html") {
		t.Fatal("engine lookup: unknown template reported present")
	}
}

// TestTaxonomyHelperEdges covers the nil-registry and unknown-name guards the
// full builds never hit.
func TestTaxonomyHelperEdges(t *testing.T) {
	g, err := New(Config{Domain: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	page := models.Page{Slug: "p", Taxonomies: map[string][]string{"technology": {"Go"}}}
	// Nil registry: everything degrades to empty values.
	if g.tmplTaxonomies() != nil || g.tmplTaxonomy("technology") != nil ||
		g.tmplTaxonomyTerms("technology") != nil || g.tmplTermURL("technology", "Go") != "" ||
		g.tmplHasTerm("technology", "Go", page) || g.tmplPagesByTerm("technology", "Go") != nil ||
		g.tmplPageTerms("technology", page) != nil {
		t.Fatal("nil-registry helpers must be empty")
	}
	if err := g.buildTaxonomies(); err != nil {
		t.Fatal(err)
	}
	// Unknown taxonomy names and non-page values.
	if g.tmplTaxonomy("nope") != nil || g.tmplTaxonomyTerms("nope") != nil ||
		g.tmplTermURL("nope", "x") != "" || g.tmplHasTerm("nope", "x", page) ||
		g.tmplPagesByTerm("nope", "x") != nil || g.tmplPageTerms("nope", page) != nil ||
		g.tmplHasTerm("tag", "x", 42) || g.tmplPageTerms("tag", "not a page") != nil {
		t.Fatal("unknown-name helpers must be empty")
	}
	// Legacy taxonomies are listed with unprefixed archive URLs.
	infos := g.tmplTaxonomies()
	if len(infos) != 3 || infos[0].Name != "category" || infos[0].URL != "/category/" {
		t.Fatalf("infos = %+v", infos)
	}
	// Positive lookups on a built (empty) registry.
	if info, ok := g.tmplTaxonomy("tag").(TaxonomyInfo); !ok || info.Label != "Tags" {
		t.Fatalf("taxonomy view = %#v", g.tmplTaxonomy("tag"))
	}
	if terms := g.tmplTaxonomyTerms("tag"); len(terms) != 0 {
		t.Fatalf("no terms expected, got %+v", terms)
	}
	// pageTerms falls back to name+slug views for terms missing from the registry.
	views := g.tmplPageTerms("tag", models.Page{Taxonomies: map[string][]string{"tag": {"New Term"}}})
	if len(views) != 1 || views[0].Slug != "new-term" || views[0].URL != "" {
		t.Fatalf("fallback views = %+v", views)
	}
}
