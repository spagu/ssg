package generator

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
	"github.com/spagu/ssg/internal/depgraph"
	"github.com/spagu/ssg/internal/models"
)

// apiFixture is a small package: a class linking to a function in another
// module, and a function whose doc links back.
func apiFixture(name string) *apimodel.Package {
	lex := apimodel.ModuleID(name, "src/lex.js")
	util := apimodel.ModuleID(name, "src/util.js")
	lexer := apimodel.SymbolID(lex, "Lexer")
	return &apimodel.Package{Name: name, Readme: "# " + name + "\n\nStart with {@link Lexer}.", Modules: []*apimodel.Module{
		{ID: lex, Path: "src/lex", Symbols: []*apimodel.Symbol{
			{ID: lexer, Name: "Lexer", Kind: apimodel.KindClass, Doc: &apimodel.Doc{Summary: "Splits text. See {@link tidy}."},
				Source: &apimodel.Source{File: "src/lex.js", Line: 4},
				Members: []*apimodel.Symbol{{ID: apimodel.MemberID(lexer, "next"), Name: "next", Kind: apimodel.KindMethod,
					Signatures: []*apimodel.Signature{{Returns: apimodel.Named("string", "")}}, Doc: &apimodel.Doc{Summary: "Next token."}}}},
		}},
		{ID: util, Path: "src/util", Symbols: []*apimodel.Symbol{
			{ID: apimodel.SymbolID(util, "tidy"), Name: "tidy", Kind: apimodel.KindFunction,
				Signatures: []*apimodel.Signature{{Params: []*apimodel.Param{{Name: "s", Type: apimodel.Named("string", "")}}}},
				Doc:        &apimodel.Doc{Summary: "Cleans for {@link Lexer.next} and {@link Missing}."}},
		}},
	}}
}

// withExtractor swaps the extractor for the test.
func withExtractor(t *testing.T, fn func(apisource.Config) (*apimodel.Package, []apisource.Diagnostic, error)) {
	t.Helper()
	old := extractAPIPackage
	extractAPIPackage = fn
	t.Cleanup(func() { extractAPIPackage = old })
}

// TestAPIDocsBuild is GO-108 end to end with the bundled simple theme, which
// has no api-* layouts: every API page still renders through page.html.
func TestAPIDocsBuild(t *testing.T) {
	withExtractor(t, func(cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic, error) {
		return apiFixture(cfg.Name), []apisource.Diagnostic{{Severity: apisource.Warning, File: "src/x.js", Line: 2, Message: "odd"}}, nil
	})
	tmp := t.TempDir()
	content := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(content, "metadata.json"), `{}`)
	mustWrite(t, filepath.Join(content, "pages", "guide.md"), "---\ntitle: Guide\nslug: guide\nstatus: publish\ntype: page\n---\n\nRead the [API](/api/core/).\n")
	out := filepath.Join(tmp, "output")
	gen, err := New(Config{Source: "site", Template: "simple", Domain: "example.com", ContentDir: filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"), OutputDir: out, Quiet: true, SearchIndex: true, CheckLinks: "strict",
		APIDocs: []APIDocsOptions{{Source: apisource.Config{Name: "core", Root: tmp}, Readme: true,
			SourceURL: "https://code.example.org/repo/blob/{ref}/{path}#L{line}", SourceRef: "v1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	page := func(rel string) string { return readFileString(t, filepath.Join(out, filepath.FromSlash(rel))) }
	index := page("api/core/index.html")
	if !strings.Contains(index, `<a href="/api/core/src/lex/Lexer/">Lexer</a>`) || !strings.Contains(index, `href="/api/core/src/util/"`) {
		t.Errorf("index page:\n%s", index)
	}
	class := page("api/core/src/lex/Lexer/index.html")
	for _, want := range []string{`<a href="/api/core/src/util/#tidy">tidy</a>`, `id="next"`,
		`href="https://code.example.org/repo/blob/v1/src/lex.js#L4"`} {
		if !strings.Contains(class, want) {
			t.Errorf("class page lacks %s:\n%s", want, class)
		}
	}
	util := page("api/core/src/util/index.html")
	if !strings.Contains(util, `<a href="/api/core/src/lex/Lexer/#next">Lexer.next</a>`) || !strings.Contains(util, "Missing") {
		t.Errorf("util page:\n%s", util)
	}
	model, err := apimodel.Decode([]byte(page("api.json")))
	if err != nil || len(model.Packages) != 1 || model.Packages[0].Name != "core" {
		t.Errorf("api.json: %v", err)
	}
	if idx := page("search-index.json"); !strings.Contains(idx, `"/api/core/src/lex/Lexer/"`) {
		t.Error("API pages must be in the search index")
	}
	if sm := page("sitemap.xml"); !strings.Contains(sm, "/api/core/src/util/") {
		t.Error("API pages must be in the sitemap")
	}
	var msgs []string
	for _, d := range gen.apiDiagnostics {
		msgs = append(msgs, d.String())
	}
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, "src/x.js:2: odd") || !strings.Contains(joined, "unresolved link to Missing") {
		t.Errorf("diagnostics = %s", joined)
	}
}

func TestAPIDocsErrors(t *testing.T) {
	g := &Generator{config: Config{}}
	if err := g.loadAPIDocs(); err != nil || g.apiModel != nil || g.writeAPIModel() != nil {
		t.Error("no api_docs means nothing to do")
	}
	withExtractor(t, func(apisource.Config) (*apimodel.Package, []apisource.Diagnostic, error) {
		return nil, nil, errors.New("no package.json")
	})
	g = &Generator{config: Config{Quiet: true, APIDocs: []APIDocsOptions{{Source: apisource.Config{Root: "lib"}}}}}
	if err := g.loadAPIDocs(); err == nil || !strings.Contains(err.Error(), "api_docs lib: no package.json") {
		t.Errorf("err = %v", err)
	}
	if apiBase(APIDocsOptions{URL: "/ref/"}, "x") != "/ref/" || apiBase(APIDocsOptions{}, "x") != "/api/x/" {
		t.Error("apiBase")
	}
	if sourceLinker(APIDocsOptions{}) != nil {
		t.Error("no template, no linker")
	}
	g.apiModel = &apimodel.API{}
	g.config.OutputDir = filepath.Join(t.TempDir(), "missing", "dir")
	if err := g.writeAPIModel(); err == nil {
		t.Error("an unwritable output must be reported")
	}
}

// TestCheckAPI: warn lists the problems and builds; strict fails with them.
func TestCheckAPI(t *testing.T) {
	g := &Generator{config: Config{Quiet: true}}
	if err := g.checkAPIIfRequested(); err != nil {
		t.Error("off without a model")
	}
	g.config.CheckAPI = "warn"
	if err := g.checkAPIIfRequested(); err != nil {
		t.Error("no model, nothing to check")
	}
	g.apiModel = &apimodel.API{Packages: []*apimodel.Package{apiFixture("core")}}
	g.apiDiagnostics = []apisource.Diagnostic{{File: "core/src/util", Message: "unresolved link to Missing"}}
	if err := g.checkAPIIfRequested(); err != nil {
		t.Errorf("warn must not fail: %v", err)
	}
	g.config.CheckAPI = "strict"
	err := g.checkAPIIfRequested()
	if err == nil || !strings.Contains(err.Error(), "API documentation problem") {
		t.Errorf("strict = %v", err)
	}
	g.apiDiagnostics = nil
	g.apiModel = &apimodel.API{}
	if err := g.checkAPIIfRequested(); err != nil {
		t.Errorf("a clean model passes strict: %v", err)
	}
}

func TestAPITemplateFuncs(t *testing.T) {
	g := &Generator{}
	if g.tmplAPIHref("x#y") != "" || string(g.tmplAPIType(apimodel.Named("T", "x#y"))) != "T" {
		t.Error("without a model, no links")
	}
	g.apiHref = func(id string) string { return "/api/" + id }
	if g.tmplAPIHref("x#y") != "/api/x#y" || string(g.tmplAPIType(apimodel.Named("T", "x#y"))) != `<a href="/api/x#y">T</a>` {
		t.Error("with a model, linked")
	}
}

// TestAPIDocsSSGTheme: the bundled docs theme renders the api-* layouts with
// the precomputed nav and crumbs, and keeps module pages out of its lists.
func TestAPIDocsSSGTheme(t *testing.T) {
	withExtractor(t, func(cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic, error) {
		return apiFixture(cfg.Name), nil, nil
	})
	tmp := t.TempDir()
	content := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(content, "metadata.json"), `{}`)
	mustWrite(t, filepath.Join(content, "pages", "guide.md"), "---\ntitle: Guide\nslug: guide\nstatus: publish\ntype: page\n---\n\nHi.\n")
	out := filepath.Join(tmp, "output")
	gen, err := New(Config{Source: "site", Template: "ssgtheme", Domain: "example.com", ContentDir: filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join("..", "..", "templates"), OutputDir: out, Quiet: true,
		APIDocs: []APIDocsOptions{{Source: apisource.Config{Name: "core", Root: tmp}, Readme: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	class := readFileString(t, filepath.Join(out, "api", "core", "src", "lex", "Lexer", "index.html"))
	for _, want := range []string{`aria-label="API reference"`, `<p class="docs-nav__title">core API</p>`,
		`href="/api/core/src/lex/Lexer/" class="docs-nav__link is-current"`, `<span aria-current="page">Lexer</span>`,
		`<a href="/api/core/src/lex/">src/lex</a>`, "<h1>Class Lexer — core</h1>", `id="next"`} {
		if !strings.Contains(class, want) {
			t.Errorf("class page lacks %s", want)
		}
	}
	guide := readFileString(t, filepath.Join(out, "guide", "index.html"))
	if strings.Contains(guide, "Module src/lex") {
		t.Error("module pages must not appear in the guides list")
	}
	if !strings.Contains(guide, "core API") {
		t.Error("the package front page belongs in the guides list")
	}
}

// TestCodeFilesAreGraphInputs: a change to a documented package's code makes
// the next incremental build full; node_modules and dot-directories are not
// the package's code.
func TestCodeFilesAreGraphInputs(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "src", "a.js"), "export const a = 1")
	mustWrite(t, filepath.Join(root, "package.json"), "{}")
	mustWrite(t, filepath.Join(root, "node_modules", "dep", "x.js"), "x")
	mustWrite(t, filepath.Join(root, ".cache", "y.js"), "y")
	mustWrite(t, filepath.Join(root, "logo.png"), "png")
	g := &Generator{config: Config{APIDocs: []APIDocsOptions{{Source: apisource.Config{Root: root}}}}, graph: depgraph.New(),
		siteData: &models.SiteData{}}
	g.recordCodeInputs()
	var ids []string
	for id, n := range g.graph.Nodes {
		if n.Kind == depgraph.KindConfig {
			ids = append(ids, filepath.Base(id))
		}
	}
	sort.Strings(ids)
	got := strings.Join(ids, ",")
	if got != "a.js,package.json" {
		t.Errorf("inputs = %s", got)
	}
	g.config.APIDocs[0].Source.Root = filepath.Join(root, "missing")
	g.recordCodeInputs() // a missing root records nothing and does not panic
}

// TestExtractorChoice: a package that ships declarations is read from them;
// a plain JavaScript package from its code; an entry naming a .d.ts wins.
func TestExtractorChoice(t *testing.T) {
	dtsRoot := filepath.Join("..", "apisource", "dts", "testdata", "lib")
	jsRoot := filepath.Join("..", "apisource", "js", "testdata", "lib")
	if !usesDeclarations(apisource.Config{Root: dtsRoot}) {
		t.Error("the declaration fixture must be read from its .d.ts files")
	}
	if usesDeclarations(apisource.Config{Root: jsRoot}) {
		t.Error("the JavaScript fixture has no declarations")
	}
	if !usesDeclarations(apisource.Config{Root: jsRoot, Entries: []string{"types/index.d.mts"}}) {
		t.Error("an entry naming a declaration file selects the declaration reader")
	}
	for _, root := range []string{dtsRoot, jsRoot} {
		pkg, _, err := extractPackage(apisource.Config{Root: root})
		if err != nil || pkg == nil || len(pkg.Modules) == 0 {
			t.Errorf("extractPackage(%s) = %v, %v", root, pkg, err)
		}
	}
}

// TestPackageLanguage: the configured language wins, then the marker files,
// then the code itself; JavaScript when nothing says.
func TestPackageLanguage(t *testing.T) {
	dir := func(files ...string) string {
		root := t.TempDir()
		for _, f := range files {
			mustWrite(t, filepath.Join(root, f), "x")
		}
		return root
	}
	for _, tt := range []struct {
		cfg  apisource.Config
		want string
	}{
		{apisource.Config{Language: "golang", Root: dir()}, "go"},
		{apisource.Config{Language: "php", Root: dir("package.json")}, "php"},
		{apisource.Config{Root: dir("package.json", "go.mod")}, "javascript"},
		{apisource.Config{Root: dir("go.mod")}, "go"},
		{apisource.Config{Root: dir("composer.json")}, "php"},
		{apisource.Config{Root: dir("setup.cfg")}, "python"},
		{apisource.Config{Root: dir("main.go")}, "go"},
		{apisource.Config{Root: dir("index.php")}, "php"},
		{apisource.Config{Root: dir("pkg/__init__.py")}, "python"},
		{apisource.Config{Root: dir("README.md")}, "javascript"},
	} {
		if got := packageLanguage(tt.cfg); got != tt.want {
			t.Errorf("packageLanguage(%+v) = %s, want %s", tt.cfg, got, tt.want)
		}
	}
}

// TestExtractEachLanguage reads the example packages in every language.
func TestExtractEachLanguage(t *testing.T) {
	for dir, lang := range map[string]string{"go": "go", "php": "php", "python": "python"} {
		pkg, _, err := extractPackage(apisource.Config{Root: filepath.Join("..", "..", "examples", "api-docs", dir)})
		if err != nil || pkg.Language != lang || len(pkg.Modules) == 0 {
			t.Errorf("%s: %v %+v", dir, err, pkg)
		}
	}
}
