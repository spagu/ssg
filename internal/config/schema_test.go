package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// schemaTwin is the copy designer-ui reads; UPDATE_CONFIG_SCHEMA=1 rewrites it.
var schemaTwin = filepath.Join("..", "..", "workers", "designer-ui", "src", "config-schema.json")

// TestSchemaCoversEveryField is the completeness gate: a configuration field
// without a description, class or group — or an entry for a field that no
// longer exists — fails here, so a new option cannot ship without its entry.
func TestSchemaCoversEveryField(t *testing.T) {
	s := BuildSchema()
	for _, p := range s.problems {
		t.Error(p)
	}
	walkSchema(s.Keys, func(n *SchemaNode) {
		if n.Group == "" {
			t.Errorf("%s: no group", n.Path)
		}
		if n.Key == "" { // anonymous element: owner carries the rest
			return
		}
		switch n.Class {
		case ClassPlain, ClassExecutes:
		case ClassSecret:
			if n.SecretFormat == "" {
				t.Errorf("%s: secret without a format", n.Path)
			}
		default:
			t.Errorf("%s: class %q", n.Path, n.Class)
		}
		if n.Description == "" {
			t.Errorf("%s: no description", n.Path)
		}
	})
}

// TestSchemaClassHasNoDefault: an entry built without p/x/s has no class, and
// the builder reports it rather than calling it plain.
func TestSchemaClassHasNoDefault(t *testing.T) {
	b := &schemaBuilder{used: map[string]bool{}}
	b.checkDoc("k", &SchemaNode{Group: "g"}, keyDoc{desc: "d"}, true)
	b.checkDoc("u", &SchemaNode{}, keyDoc{}, false) // undocumented: reported elsewhere
	if len(b.problems) != 1 || !strings.Contains(b.problems[0], "k") {
		t.Errorf("problems = %v", b.problems)
	}
}

// TestSchemaReportsStaleAndDuplicateEntries keeps the table honest both ways.
func TestSchemaReportsStaleAndDuplicateEntries(t *testing.T) {
	savedDocs, savedDup := schemaDocs["config.Config"], schemaDocDuplicates
	t.Cleanup(func() { schemaDocs["config.Config"], schemaDocDuplicates = savedDocs, savedDup })
	copied := typeDocs{}
	for k, v := range savedDocs {
		copied[k] = v
	}
	schemaDocs["config.Config"] = copied
	describe(Config{}, typeDocs{"title": p("again"), "no_such_key": p("x").in(gSite)})
	delete(copied, "quiet")
	got := strings.Join(BuildSchema().problems, "\n")
	for _, want := range []string{"documented twice: config.Config.title", "documented but not a field: config.Config.no_such_key", "undocumented: quiet"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestSchemaFieldWithoutYAMLTag is reported, not guessed.
func TestSchemaFieldWithoutYAMLTag(t *testing.T) {
	type noTag struct{ Name string }
	b := &schemaBuilder{used: map[string]bool{}}
	b.fields(reflect.TypeOf(noTag{}), "", reflect.Value{}, "")
	if len(b.problems) != 1 || !strings.Contains(b.problems[0], "without a yaml tag") {
		t.Errorf("problems = %v", b.problems)
	}
}

func TestSchemaShapesTypesAndDefaults(t *testing.T) {
	s := BuildSchema()
	cases := []struct {
		path, typ, alt string
		def            any
	}{
		{"port", "int", "", 8888},
		{"title", "string", "", ""},
		{"check_markup", "enum", "", "warn"},
		{"rate_limit", "number", "", 0.0},
		{"minify_all", "bool", "", false},
		{"heading_anchors", "object", "bool", nil},
		{"story", "object", "bool", nil},
		{"pretty_urls", "enum", "bool", nil},
		{"outputs", "list", "map", nil},
		{"sitemaps[].prefix", "list", "string", nil},
		{"languages", "list", "list", nil},
		{"build_report", "string", "bool", ""},
		{"variables", "map", "", nil},
		{"variables.*", "", "", nil},
		{"auto_reload", "bool", "", true}, // a nil *bool means on: the entry says so (.defaults)
		{"mddb.timeout", "int", "", 30},
	}
	for _, c := range cases {
		n := s.Node(c.path)
		if c.path == "variables.*" {
			if v := s.Node("variables").Values; v.Type != "any" {
				t.Errorf("variables values = %s", v.Type)
			}
			continue
		}
		if n == nil {
			t.Errorf("%s: missing", c.path)
			continue
		}
		if n.Type != c.typ {
			t.Errorf("%s: type %s, want %s", c.path, n.Type, c.typ)
		}
		if (n.Alternate == nil) != (c.alt == "") || (n.Alternate != nil && n.Alternate.Type != c.alt) {
			t.Errorf("%s: alternate %+v, want %q", c.path, n.Alternate, c.alt)
		}
		if c.def != nil && !reflect.DeepEqual(n.Default, c.def) {
			t.Errorf("%s: default %#v, want %#v", c.path, n.Default, c.def)
		}
		if c.def == nil && n.Default != nil && n.Type != "enum" {
			t.Errorf("%s: unexpected default %#v", c.path, n.Default)
		}
	}
	if !s.Node("story").Open {
		t.Error("story takes keys it does not name")
	}
	if n := s.Node("server_users"); n.Class != ClassSecret || n.SecretFormat != SecretLogin {
		t.Errorf("server_users = %+v", n)
	}
	if n := s.Node("jwt_secret"); n.SecretFormat != SecretWhole {
		t.Errorf("jwt_secret format = %q", n.SecretFormat)
	}
	if s.Node("nope") != nil {
		t.Error("an unknown path has no node")
	}
}

// TestSchemaTwinUpToDate keeps workers/designer-ui/src/config-schema.json the
// bytes `ssg config schema` prints. UPDATE_CONFIG_SCHEMA=1 regenerates it.
func TestSchemaTwinUpToDate(t *testing.T) {
	want := SchemaJSON()
	if os.Getenv("UPDATE_CONFIG_SCHEMA") == "1" {
		if err := os.MkdirAll(filepath.Dir(schemaTwin), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(schemaTwin, want, 0o644); err != nil { // #nosec G306 -- a source file of the repository
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(schemaTwin)
	if err != nil {
		t.Fatalf("%v (UPDATE_CONFIG_SCHEMA=1 go test ./internal/config -run TestSchemaTwinUpToDate)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is stale: UPDATE_CONFIG_SCHEMA=1 go test ./internal/config -run TestSchemaTwinUpToDate", schemaTwin)
	}
	var doc Schema
	if err := json.Unmarshal(want, &doc); err != nil || doc.SchemaVersion != SchemaVersion || doc.OverlayFile != OverlayFileName || len(doc.Groups) == 0 {
		t.Errorf("schema JSON does not round-trip: %v", err)
	}
}

func TestMCPWritableKeysKinds(t *testing.T) {
	kinds := map[string]string{}
	for _, k := range MCPWritableKeys() {
		kinds[k.Path] = k.Kind
	}
	want := map[string]string{"title": "string", "paginate": "int", "webp": "bool", "outputs": "list", "story": "switch", "image_formats": "list"}
	for k, v := range want {
		if kinds[k] != v {
			t.Errorf("%s kind = %q, want %q", k, kinds[k], v)
		}
	}
	if mcpKind("enum") != "string" {
		t.Error("an enum is written as a string")
	}
}

func TestDerefValue(t *testing.T) {
	var nilPtr *int
	if derefValue(reflect.ValueOf(nilPtr)).IsValid() || derefValue(reflect.Value{}).IsValid() || derefValue(reflect.ValueOf(3)).IsValid() {
		t.Error("only a non-nil pointer has a value")
	}
	n := 4
	if derefValue(reflect.ValueOf(&n)).Int() != 4 {
		t.Error("pointer not followed")
	}
}

// A switch whose description promises a default carries it, so a panel can show
// "on" rather than "not set"; and a default given by .defaults() agrees with
// what the description says.
func TestSchemaDefaultsMatchTheirDescriptions(t *testing.T) {
	var walk func(n *SchemaNode)
	checked := 0
	walk = func(n *SchemaNode) {
		if n.Type == "bool" && n.Key != "" {
			d := n.Description
			switch {
			case strings.Contains(d, "(default on)"), strings.Contains(d, "(default true)"):
				checked++
				if n.Default != true {
					t.Errorf("%s says it defaults on, schema default = %v", n.Path, n.Default)
				}
			case strings.Contains(d, "(default false)"):
				checked++
				if n.Default != false {
					t.Errorf("%s says it defaults off, schema default = %v", n.Path, n.Default)
				}
			}
		}
		for _, f := range n.Fields {
			walk(f)
		}
		for _, c := range []*SchemaNode{n.Items, n.Values, n.Alternate} {
			if c != nil {
				walk(c)
			}
		}
	}
	for _, k := range BuildSchema().Keys {
		walk(k)
	}
	if checked < 15 {
		t.Fatalf("only %d described defaults found", checked)
	}
	if BuildSchema().Node("attribution").Default != true {
		t.Error("attribution is on unless a commercial license turns it off")
	}
}
