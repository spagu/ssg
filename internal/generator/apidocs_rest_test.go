package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
	"github.com/spagu/ssg/internal/depgraph"
	"github.com/spagu/ssg/internal/models"
)

const restSpec = `openapi: 3.0.3
info: {title: Orders API, version: "1"}
servers: [{url: "https://api.example.com/v1"}]
tags: [{name: Orders}]
security: [{key: []}]
paths:
  /orders/{id}:
    get:
      tags: [Orders]
      operationId: getOrder
      summary: Get an order
      parameters: [{name: id, in: path, required: true, schema: {type: integer}}]
      responses:
        "200": {description: The order, content: {application/json: {schema: {$ref: '#/components/schemas/Order'}}}}
components:
  securitySchemes: {key: {type: apiKey, in: header, name: X-Key}}
  schemas: {Order: {type: object, properties: {id: {type: integer}}}}
`

// TestRESTDocsBuild: a code package and an OpenAPI file side by side in the
// apidoc theme, with live examples and the Try it console.
func TestRESTDocsBuild(t *testing.T) {
	withExtractor(t, func(cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic, error) {
		pkg := apiFixture(cfg.Name)
		pkg.Modules[0].Symbols[0].Doc.Examples = []string{"```js\nnew Lexer()\n```"}
		return pkg, nil, nil
	})
	tmp := t.TempDir()
	content := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(content, "metadata.json"), `{}`)
	mustWrite(t, filepath.Join(content, "pages", "guide.md"), "---\ntitle: Guide\nslug: guide\nstatus: publish\ntype: page\n---\n\nSee [orders](/api/orders-api/orders/#getorder).\n")
	mustWrite(t, filepath.Join(content, "posts", "news", "hello.md"),
		"---\ntitle: Release notes\nslug: hello\nstatus: publish\ntype: post\ndate: 2026-10-01\ncategories: [news]\n---\n\nNew.\n")
	spec := filepath.Join(tmp, "orders.yaml")
	mustWrite(t, spec, restSpec)
	out := filepath.Join(tmp, "output")
	gen, err := New(Config{Source: "site", Template: "apidoc", Domain: "example.com", ContentDir: filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join("..", "..", "templates"), OutputDir: out, Quiet: true, SearchIndex: true, CheckLinks: "strict",
		CheckAPI: "warn",
		APIDocs: []APIDocsOptions{
			{Source: apisource.Config{Name: "core", Root: tmp}, Readme: true, Playground: "https://cdn.example.com/core.js"},
			{OpenAPI: spec, TryIt: true},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	page := func(rel string) string { return readFileString(t, filepath.Join(out, filepath.FromSlash(rel))) }
	orders := page("api/orders-api/orders/index.html")
	for _, want := range []string{`<h2 id="getorder">Get an order</h2>`, `data-ssg-tryit="`, "<script data-ssg-tryit-script>",
		"<style data-ssg-api-tools>", `class="ssg-method ssg-method-get"`, `<span aria-current="page">Orders</span>`,
		`href="/api/orders-api/schemas/">Schemas`, `<a href="/api/orders-api/">Orders API</a>`} {
		if !strings.Contains(orders, want) {
			t.Errorf("orders page lacks %s", want)
		}
	}
	if strings.Contains(orders, "data-ssg-playground-script") {
		t.Error("no examples here, no playground script")
	}
	lexer := page("api/core/src/lex/Lexer/index.html")
	if !strings.Contains(lexer, `data-module="https://cdn.example.com/core.js"`) || !strings.Contains(lexer, "data-ssg-playground-script") ||
		strings.Contains(lexer, "data-ssg-tryit-script") {
		t.Error("the class page has a live example and no console")
	}
	home := page("index.html")
	if !strings.Contains(home, `<code>Orders API</code>`) || !strings.Contains(home, `<code>core</code>`) {
		t.Error("the home page lists both references")
	}
	if !strings.Contains(home, "Release notes") {
		t.Error("the home page lists news")
	}
	if cat := page("2026/10/01/hello/index.html"); !strings.Contains(cat, `<p class="ad-eyebrow">News</p>`) || !strings.Contains(cat, "1 October 2026") {
		t.Error("the post renders in apidoc")
	}
	model, err := apimodel.Decode([]byte(page("api.json")))
	if err != nil || len(model.Packages) != 1 {
		t.Errorf("api.json holds the code package only: %v", err)
	}
	if idx := page("search-index.json"); !strings.Contains(idx, `"/api/orders-api/orders/"`) {
		t.Error("REST pages are searchable")
	}
}

func TestRESTDocsOnlyAndErrors(t *testing.T) {
	tmp := t.TempDir()
	spec := filepath.Join(tmp, "my-service.yaml")
	mustWrite(t, spec, "openapi: 3.0.0\ninfo: {title: ' '}\npaths:\n  /a/{id}:\n    get: {}\n")
	g := &Generator{config: Config{Quiet: true, APIDocs: []APIDocsOptions{{OpenAPI: spec}}}, siteData: &models.SiteData{}}
	if err := g.loadAPIDocs(); err != nil {
		t.Fatal(err)
	}
	if g.apiModel != nil || g.writeAPIModel() != nil {
		t.Error("REST only: no code model, no api.json")
	}
	if len(g.siteData.Pages) == 0 || g.siteData.Pages[0].Link != "/api/my-service/" {
		t.Errorf("the base falls back to the file name: %+v", g.siteData.Pages)
	}
	if extra := g.siteData.Pages[0].Extra["api"].(map[string]interface{}); extra["rest"] != true {
		t.Error("REST pages say so")
	}
	g.config.CheckAPI = "strict"
	if err := g.checkAPIIfRequested(); err == nil || !strings.Contains(err.Error(), "API documentation problem") {
		t.Errorf("spec problems fail strict: %v", err)
	}
	named := &Generator{config: Config{Quiet: true, APIDocs: []APIDocsOptions{{OpenAPI: spec, Source: apisource.Config{Name: "Billing"}}}},
		siteData: &models.SiteData{}}
	if err := named.loadAPIDocs(); err != nil || named.siteData.Pages[0].Link != "/api/billing/" ||
		named.siteData.Pages[0].Extra["api"].(map[string]interface{})["package"].(map[string]interface{})["Name"] != "Billing" {
		t.Error("a configured name names the API and its URL")
	}
	bad := &Generator{config: Config{Quiet: true, APIDocs: []APIDocsOptions{{OpenAPI: filepath.Join(tmp, "none.yaml")}}}, siteData: &models.SiteData{}}
	if err := bad.loadAPIDocs(); err == nil || !strings.Contains(err.Error(), "api_docs openapi") {
		t.Errorf("a missing file fails the build: %v", err)
	}
}

func TestOpenAPIFileIsGraphInput(t *testing.T) {
	spec := filepath.Join(t.TempDir(), "spec.yaml")
	mustWrite(t, spec, restSpec)
	g := &Generator{config: Config{APIDocs: []APIDocsOptions{{OpenAPI: spec, Source: apisource.Config{Root: "."}}}},
		graph: depgraph.New(), siteData: &models.SiteData{}}
	g.recordCodeInputs()
	if len(g.graph.Nodes) != 1 {
		t.Errorf("only the spec is an input, not the whole root: %d nodes", len(g.graph.Nodes))
	}
}

func TestApidocThemeIsEmbedded(t *testing.T) {
	dir := t.TempDir()
	ok, err := scaffoldEmbeddedTheme("apidoc", dir)
	if err != nil || !ok {
		t.Fatalf("scaffold: %v %v", ok, err)
	}
	for _, f := range []string{"index.html", "page.html", "layouts/api-symbol.html", "css/tokens.css", "js/main.js"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
}
