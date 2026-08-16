package generator

// The reporter's site, as a regression (#158): schema_defaults promises Recipe
// for a section, the theme's post template carries its own hand-written FAQPage
// block, and SEO injection — gated on the theme having emitted no
// application/ld+json — silently skips the page. Zero Recipe shipped for three
// days while check_schema reported that structured data carried every required
// property, because the FAQPage it could see was complete.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

const faqBlock = `<script type="application/ld+json">` +
	`{"@context":"https://schema.org","@type":"FAQPage","mainEntity":[{"@type":"Question","name":"How long?"}]}` +
	`</script>`

// TestLdTypesInReadsEveryShape: a Recipe beside an FAQPage is two sibling
// blocks, one array or one @graph — Google's own guidance produces all three,
// so a check that read only the root node would report a type plainly present.
func TestLdTypesInReadsEveryShape(t *testing.T) {
	cases := map[string]string{
		"plain node": `{"@type":"Recipe","name":"Soup"}`,
		"array":      `[{"@type":"FAQPage"},{"@type":"Recipe"}]`,
		"graph":      `{"@context":"https://schema.org","@graph":[{"@type":"FAQPage"},{"@type":"Recipe"}]}`,
		"type list":  `{"@type":["Recipe","Product"]}`,
	}
	for name, raw := range cases {
		got := ldTypesIn(raw)
		found := false
		for _, ty := range got {
			if ty == "Recipe" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: Recipe not found in %v", name, got)
		}
	}
	// Malformed JSON-LD belongs to the property check, not to this one.
	if got := ldTypesIn(`{not json`); got != nil {
		t.Errorf("malformed block = %v, want nothing", got)
	}
	// A self-referential graph must not spin the build.
	deep := `{"@graph":[{"@graph":[{"@graph":[{"@graph":[{"@graph":[{"@graph":[{"@graph":[{"@graph":[{"@type":"Recipe"}]}]}]}]}]}]}]}]}`
	_ = ldTypesIn(deep) // bounded; the assertion is that it returns at all
}

// TestMissingPromisedType: the question the check asks — did the type the
// section declared actually ship?
func TestMissingPromisedType(t *testing.T) {
	faq := `{"@type":"FAQPage","mainEntity":[]}`
	recipe := `{"@type":"Recipe","name":"Soup"}`

	if !missingPromisedType([]string{faq}, "Recipe") {
		t.Error("a page with only FAQPage is missing the promised Recipe")
	}
	if missingPromisedType([]string{faq, recipe}, "Recipe") {
		t.Error("two sibling blocks satisfy the promise")
	}
	if missingPromisedType([]string{`{"@graph":[{"@type":"FAQPage"},{"@type":"Recipe"}]}`}, "Recipe") {
		t.Error("a @graph carrying both satisfies the promise")
	}
	// Case is not the author's problem.
	if missingPromisedType([]string{`{"@type":"recipe"}`}, "Recipe") {
		t.Error("@type matching must not be case-sensitive")
	}
	// Nothing promised, and nothing emitted at all, are both other checks' business.
	if missingPromisedType([]string{faq}, "") {
		t.Error("an empty promise has nothing to verify")
	}
	if missingPromisedType(nil, "Recipe") {
		t.Error("a page with no structured data is seo:'s business, not this check's")
	}
}

// TestLdTypeOf: what a section promises, and what it deliberately does not.
func TestLdTypeOf(t *testing.T) {
	if got, ok := ldTypeOf(map[string]interface{}{"@type": "Recipe"}); !ok || got != "Recipe" {
		t.Errorf("ldTypeOf = %q, %v", got, ok)
	}
	// A list of types leaves which one a page must carry to the author — a
	// guess here would produce a warning nobody can act on.
	if _, ok := ldTypeOf(map[string]interface{}{"@type": []interface{}{"Recipe", "Product"}}); ok {
		t.Error("a list of types promises nothing specific")
	}
	for _, ld := range []map[string]interface{}{nil, {}, {"@type": ""}, {"@type": "   "}, {"name": "x"}} {
		if _, ok := ldTypeOf(ld); ok {
			t.Errorf("%#v promises nothing", ld)
		}
	}
}

// TestPromisedTypeFindingNamesTheCause: the report has to say more than "it is
// missing" — the cause is the theme's own block turning injection off, and the
// way out is emitting the derived data beside it.
func TestPromisedTypeFindingNamesTheCause(t *testing.T) {
	f := promisedTypeFinding("recipes/soup/index.html", "Recipe", 1)
	for _, want := range []string{"schema_defaults", "Recipe", "auto-injection", "toJSON .Schema"} {
		if !strings.Contains(f.detail, want) {
			t.Errorf("the finding must mention %q: %s", want, f.detail)
		}
	}
	// With no blocks of its own, the theme is not the explanation, so the
	// message does not blame it.
	if plain := promisedTypeFinding("a.html", "Recipe", 0); strings.Contains(plain.detail, "auto-injection") {
		t.Errorf("nothing to blame the theme for: %s", plain.detail)
	}
}

// recipeGen builds a generator whose "posts" section promises Recipe, with one
// post placed in it — the reporter's arrangement.
func recipeGen(t *testing.T, tmpl string) (*Generator, models.Page) {
	t.Helper()
	g := newTestGen(t, tmpl)
	root := t.TempDir()
	g.config.ContentDir = filepath.Join(root, "content")
	g.config.Source = "site"
	g.config.SchemaDefaults = map[string]map[string]interface{}{
		"posts": {"@type": "Recipe", "recipeYield": "4"},
	}
	page := models.Page{
		Title: "Chicken soup", Slug: "soup", Type: "post", Status: "publish",
		Date:      time.Date(2024, time.May, 1, 0, 0, 0, 0, time.UTC),
		SourceDir: filepath.Join(g.config.ContentDir, "site", "posts"),
	}
	return g, page
}

// TestPromisedSchemaTypesMapsPagesToFiles: the check walks output files, the
// promise is made per content section, and this is what joins them.
func TestPromisedSchemaTypesMapsPagesToFiles(t *testing.T) {
	if got := newTestGen(t, "").promisedSchemaTypes(); got != nil {
		t.Fatalf("no schema_defaults, no promises: %v", got)
	}
	g, page := recipeGen(t, "")
	if got := g.promisedSchemaTypes(); len(got) != 0 {
		t.Fatalf("no content, no promises: %v", got)
	}

	g.siteData.Posts = []models.Page{page}
	got := g.promisedSchemaTypes()
	if len(got) == 0 {
		t.Fatal("a post in a section that promises Recipe must be listed")
	}
	for file, want := range got {
		if want != "Recipe" {
			t.Errorf("%s promises %q, want Recipe", file, want)
		}
	}
	// A page outside every configured prefix promises nothing.
	g.siteData.Posts = []models.Page{{
		Title: "About", Slug: "about", Type: "page",
		SourceDir: filepath.Join(g.config.ContentDir, "site", "pages"),
	}}
	if got := g.promisedSchemaTypes(); len(got) != 0 {
		t.Errorf("an unconfigured section promises nothing: %v", got)
	}
}

// TestCheckSchemaReportsTheMissingType: end to end, on the reported shape — the
// theme writes FAQPage, the section promised Recipe, the page ships without one,
// and the build now says so instead of reporting success.
func TestCheckSchemaReportsTheMissingType(t *testing.T) {
	g, page := recipeGen(t, "")
	g.config.CheckSchema = "warn"
	g.siteData.Posts = []models.Page{page}

	rel := ""
	for file := range g.promisedSchemaTypes() {
		rel = file
		break
	}
	if rel == "" {
		t.Fatal("the post must carry a promise")
	}
	path := filepath.Join(g.config.OutputDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path,
		[]byte("<html><head>"+faqBlock+"</head><body>Soup</body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := captureBuildOutput(t, func() {
		if err := g.checkSchemaIfRequested(); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"Recipe", "schema_defaults", "auto-injection"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report must mention %q:\n%s", want, out)
		}
	}

	// The same page, once the theme emits the derived data beside its own
	// block, is silent.
	both := "<html><head>" + faqBlock +
		`<script type="application/ld+json">{"@type":"Recipe","name":"Soup","image":"a.jpg","recipeIngredient":["x"],"recipeInstructions":"y"}</script>` +
		"</head><body>Soup</body></html>"
	if err := os.WriteFile(path, []byte(both), 0o600); err != nil {
		t.Fatal(err)
	}
	out = captureBuildOutput(t, func() {
		if err := g.checkSchemaIfRequested(); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(out, "schema_defaults promises") {
		t.Errorf("a theme that emits both must not be warned about:\n%s", out)
	}

	// strict turns the same finding into a failed build.
	if err := os.WriteFile(path,
		[]byte("<html><head>"+faqBlock+"</head><body>Soup</body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	g.config.CheckSchema = "strict"
	if err := captureBuildErr(t, g.checkSchemaIfRequested); err == nil {
		t.Error("strict must fail the build on a promise that did not ship")
	}
}

// captureBuildErr runs fn with its output swallowed and returns its error.
func captureBuildErr(t *testing.T, fn func() error) error {
	t.Helper()
	var err error
	captureBuildOutput(t, func() { err = fn() })
	return err
}

// TestSchemaReachesTemplates: the supported way to have both blocks —
// `{{ toJSON .Schema }}` — needs the merged object, in precedence order, with
// the section's @type on it.
func TestSchemaReachesTemplates(t *testing.T) {
	g, page := recipeGen(t, "")
	data := g.pageToTemplateData(page, true)

	schema, ok := data["Schema"].(map[string]interface{})
	if !ok {
		t.Fatalf(".Schema reached the template as %T", data["Schema"])
	}
	// The section's type wins over the derived BlogPosting — that is what
	// schema_defaults exists for.
	if schema["@type"] != "Recipe" {
		t.Errorf("@type = %v, want Recipe", schema["@type"])
	}
	if schema["recipeYield"] != "4" {
		t.Errorf("the section's own properties must survive: %v", schema["recipeYield"])
	}
	// The derived data is still underneath it, so a theme does not lose what
	// ssg worked out from the frontmatter.
	if schema["headline"] != page.Title {
		t.Errorf("headline = %v, want %q", schema["headline"], page.Title)
	}
	// And it is the same object the injector would emit, so a theme that
	// renders it produces exactly what auto-injection would have.
	if injected := g.buildJSONLD(page, true); !strings.Contains(injected, `"@type":"Recipe"`) {
		t.Errorf("the injected block and .Schema must agree: %s", injected)
	}
}
