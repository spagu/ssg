package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/taxonomy"
)

// apiTemplate tells the taxonomy index from a term archive by what it has.
const apiTemplate = `{{if .Term}}{"term":"{{.Term.Name}}","posts":[{{range $i, $p := .Posts}}{{if $i}},{{end}}"{{$p.Slug}}"{{end}}]}` +
	`{{else if .Terms}}{"taxonomy":"{{.Taxonomy.Label}}","domain":"{{.Domain}}","terms":[{{range $i, $t := .Terms}}{{if $i}},{{end}}"{{$t.Name}}:{{$t.Count}}"{{end}}]}{{end}}`

// taxonomyOutputsSite writes two posts under technology: [Go] and returns a
// config with an `api` custom format.
func taxonomyOutputsSite(t *testing.T) (string, Config) {
	t.Helper()
	tmp := t.TempDir()
	postsDir := filepath.Join(tmp, "content", "site", "posts", "news")
	writeTaxonomyPost(t, postsDir, "one", "One", "technology: [Go]\n")
	writeTaxonomyPost(t, postsDir, "two", "Two", "technology: [Go]\n")
	writeTaxonomyMeta(t, tmp)
	writeTaxonomyTemplates(t, filepath.Join(tmp, "templates", "simple"))
	tmpl := filepath.Join(tmp, "api.json.tmpl")
	mustWrite(t, tmpl, apiTemplate)
	cfg := taxonomyTestConfig(tmp)
	cfg.OutputsCustom = []CustomOutput{{Name: "api", Suffix: "index.json", MIME: "application/json", Template: tmpl}}
	return tmp, cfg
}

func generateSite(t *testing.T, cfg Config) {
	t.Helper()
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
}

// TestTaxonomyOutputsIndexAndTerm: `outputs: [html, api]` writes the format
// beside the taxonomy index and beside each term, from .Terms and .Term.
func TestTaxonomyOutputsIndexAndTerm(t *testing.T) {
	tmp, cfg := taxonomyOutputsSite(t)
	cfg.Taxonomies["technology"] = taxonomy.DefinitionConfig{Outputs: []string{"html", "api"}}
	generateSite(t, cfg)
	out := filepath.Join(tmp, "output", "technology")
	index := mustRead(t, filepath.Join(out, "index.json"))
	if index != `{"taxonomy":"Technology","domain":"example.com","terms":["Go:2"]}` {
		t.Errorf("index.json = %s", index)
	}
	term := mustRead(t, filepath.Join(out, "go", "index.json"))
	if !strings.HasPrefix(term, `{"term":"Go","posts":[`) || !strings.Contains(term, `"one"`) || !strings.Contains(term, `"two"`) {
		t.Errorf("term index.json = %s", term)
	}
	wantFiles(t, filepath.Join(out, "index.html"), filepath.Join(out, "go", "index.html"))
	// Another taxonomy without outputs stays HTML only.
	if _, err := os.Stat(filepath.Join(tmp, "output", "platform", "index.json")); err == nil {
		t.Error("platform has no outputs and must not get index.json")
	}
}

// TestTaxonomyOutputsWithoutOutputsIsHTMLOnly: the default writes no format.
func TestTaxonomyOutputsWithoutOutputsIsHTMLOnly(t *testing.T) {
	tmp, cfg := taxonomyOutputsSite(t)
	generateSite(t, cfg)
	for _, p := range []string{"index.json", filepath.Join("go", "index.json")} {
		if _, err := os.Stat(filepath.Join(tmp, "output", "technology", p)); err == nil {
			t.Errorf("%s written without outputs", p)
		}
	}
}

// TestTaxonomyOutputsIndexOrTermOnly: outputs_index and outputs_term choose
// one of the two views each.
func TestTaxonomyOutputsIndexOrTermOnly(t *testing.T) {
	tmp, cfg := taxonomyOutputsSite(t)
	cfg.Taxonomies["technology"] = taxonomy.DefinitionConfig{OutputsTerm: []string{"api"}}
	cfg.Taxonomies["platform"] = taxonomy.DefinitionConfig{OutputsIndex: []string{"api"}}
	generateSite(t, cfg)
	out := filepath.Join(tmp, "output")
	wantFiles(t, filepath.Join(out, "technology", "go", "index.json"))
	if _, err := os.Stat(filepath.Join(out, "technology", "index.json")); err == nil {
		t.Error("outputs_term must not write the index format")
	}
}

// TestTaxonomyOutputsPaginatedTermOnce: a paginated term writes its format once,
// beside page 1, listing every post — not a page of them.
func TestTaxonomyOutputsPaginatedTermOnce(t *testing.T) {
	tmp, cfg := taxonomyOutputsSite(t)
	cfg.Paginate = 1
	cfg.Taxonomies["technology"] = taxonomy.DefinitionConfig{Outputs: []string{"api"}}
	generateSite(t, cfg)
	term := filepath.Join(tmp, "output", "technology", "go")
	data := mustRead(t, filepath.Join(term, "index.json"))
	if !strings.Contains(data, `"one"`) || !strings.Contains(data, `"two"`) {
		t.Errorf("page-1 format should list every post: %s", data)
	}
	if _, err := os.Stat(filepath.Join(term, "page", "2", "index.json")); err == nil {
		t.Error("page 2 must not get its own format file")
	}
}

// TestTaxonomyOutputsMultilingual: each language's base gets its own file.
func TestTaxonomyOutputsMultilingual(t *testing.T) {
	tmp, cfg := taxonomyOutputsSite(t)
	postsDir := filepath.Join(tmp, "content", "site", "posts", "news")
	writeTaxonomyPost(t, postsDir, "drei", "Drei", "technology: [Rust]\nlang: en\n")
	cfg.Languages = []string{"pl", "en"}
	cfg.DefaultLanguage = "pl"
	cfg.I18n.Enabled = true
	cfg.Taxonomies["technology"] = taxonomy.DefinitionConfig{Outputs: []string{"api"}}
	generateSite(t, cfg)
	out := filepath.Join(tmp, "output")
	en := mustRead(t, filepath.Join(out, "en", "technology", "index.json"))
	if !strings.Contains(en, "Rust:1") || strings.Contains(en, "Go:") {
		t.Errorf("en index.json = %s", en)
	}
	wantFiles(t, filepath.Join(out, "technology", "index.json"), filepath.Join(out, "en", "technology", "rust", "index.json"))
}

// TestTaxonomyOutputsRejectedFormats: a built-in or unknown format is skipped
// with a warning, and a template that fails or renders nothing writes nothing.
func TestTaxonomyOutputsRejectedFormats(t *testing.T) {
	tmp, cfg := taxonomyOutputsSite(t)
	empty := filepath.Join(tmp, "empty.tmpl")
	mustWrite(t, empty, `{{if .Term}}{{.Term.Name}}{{end}}`)
	broken := filepath.Join(tmp, "broken.tmpl")
	mustWrite(t, broken, `{{index .Terms 99}}`)
	cfg.OutputsCustom = append(cfg.OutputsCustom,
		CustomOutput{Name: "empty", Suffix: "index.txt", Template: empty},
		CustomOutput{Name: "broken", Suffix: "index.xml", Template: broken})
	cfg.Taxonomies["technology"] = taxonomy.DefinitionConfig{Outputs: []string{"json", "nope", "", "empty", "broken"}}
	generateSite(t, cfg)
	out := filepath.Join(tmp, "output", "technology")
	for _, p := range []string{"index.json", "index.txt", "index.xml", filepath.Join("go", "index.xml")} {
		if _, err := os.Stat(filepath.Join(out, p)); err == nil {
			t.Errorf("%s should not be written", p)
		}
	}
	wantFiles(t, filepath.Join(out, "go", "index.txt"))
}

func TestWriteViewOutputsWithoutRegistry(t *testing.T) {
	g := &Generator{}
	g.writeViewOutputs([]string{"api"}, "x/index.html", "view", nil) // no registry: nothing to do
}

func TestViewContext(t *testing.T) {
	m := map[string]interface{}{"a": 1}
	if got := viewContext(m); got["a"] != 1 {
		t.Errorf("map passes through: %v", got)
	}
	type view struct {
		Name   string
		hidden int
	}
	got := viewContext(&view{Name: "x", hidden: 1})
	if got["Name"] != "x" || len(got) != 1 {
		t.Errorf("struct pointer: %v", got)
	}
	if got := viewContext(42); len(got) != 0 {
		t.Errorf("non-struct: %v", got)
	}
}
