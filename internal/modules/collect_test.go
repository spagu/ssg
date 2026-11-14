package modules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func text(s string) map[string]string {
	return map[string]string{"en": s, "pl": s, "hi": s, "de": s, "fr": s}
}

// manifestFor is a valid manifest for id with the given panel ("" = none).
func manifestFor(id, kind, panel string) map[string]any {
	m := map[string]any{
		"id": id, "contract": 1, "version": "1.2.3", "kind": kind,
		"title": text(strings.ToUpper(id)), "description": text("d"), "panel": nil,
		"nav": []any{}, "permissions": []any{}, "endpoints": map[string]any{}, "disable": text("x"),
	}
	if panel != "" {
		m["panel"] = panel
		m["permissions"] = []any{map[string]any{"name": id + ":read", "description": text("r")}}
		m["nav"] = []any{map[string]any{"label": text("p"), "path": panel, "permission": id + ":read"}}
	}
	return m
}

// writeModule writes manifest m as dir/module.json and returns dir.
func writeModule(t *testing.T, dir string, m any) string {
	t.Helper()
	data, _ := json.Marshal(m)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "module.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// shipped is a stand-in for the binary's embedded templates.
func shipped(t *testing.T) fstest.MapFS {
	t.Helper()
	enc := func(m any) *fstest.MapFile { data, _ := json.Marshal(m); return &fstest.MapFile{Data: data} }
	return fstest.MapFS{
		"workers/comments/module.json":   enc(manifestFor("comments", KindPagesFunction, "/comments-admin.html")),
		"workers/contact/module.json":    enc(manifestFor("contact", KindPagesFunction, "")),
		"workers/broken/module.json":     {Data: []byte(`{"id": "broken"}`)},
		"workers/leftover/node_modules":  {Data: []byte("x")},
		"workers/module.schema.json":     {Data: []byte("{}")},
		"workers/newsletter/module.json": enc(manifestFor("newsletter", KindPagesFunction, "")),
	}
}

func TestCollectRegistersExactlyTheSiteModules(t *testing.T) {
	root := t.TempDir()
	core := writeModule(t, filepath.Join(root, "core"), manifestFor("core", KindPagesFunction, "/admin/"))
	comments := writeModule(t, filepath.Join(root, "comments"), manifestFor("comments", KindPagesFunction, "/comments-admin.html"))
	plain := filepath.Join(root, "plain") // a worker without module.json: not a module
	ui := writeModule(t, filepath.Join(root, "ui"), manifestFor("newsletter-ui", KindWorker, "/"))
	reg, findings := Collect(Input{
		ConfigFile: "site.yaml", Schema: mustSchema(t), Templates: shipped(t),
		Workers: []Worker{{"workers[0]", comments}, {"workers[1]", plain}, {"workers[2]", core}},
		Refs:    []Ref{{ID: "newsletter-ui", URL: "/newsletter-admin/", Dir: ui}, {ID: "shop", URL: "/shop"}},
	})
	if len(findings) != 0 {
		t.Fatalf("findings: %v", findings)
	}
	var ids []string
	for _, e := range reg.Modules {
		ids = append(ids, e.ID)
	}
	if strings.Join(ids, ",") != "comments,core,newsletter-ui,shop" {
		t.Fatalf("modules %v", ids)
	}
	ui0, shop := reg.Modules[2], reg.Modules[3]
	if ui0.Bundled || ui0.Base != "/newsletter-admin" || *ui0.Kind != KindWorker || *ui0.Version != "1.2.3" || *ui0.Contract != 1 || ui0.Manifest == nil {
		t.Errorf("newsletter-ui entry %+v", ui0)
	}
	if shop.Kind != nil || shop.Manifest != nil || shop.Contract != nil || shop.Version != nil || shop.Base != "/shop" {
		t.Errorf("an entry without dir records id and base only: %+v", shop)
	}
	if !reg.Modules[0].Bundled || reg.Modules[0].Base != "/" {
		t.Errorf("bundled entry %+v", reg.Modules[0])
	}
	if len(reg.Available) != 2 || reg.Available[0] != (Available{ID: "contact", Kind: KindPagesFunction, Title: "CONTACT"}) || reg.Available[1].ID != "newsletter" {
		t.Errorf("available %+v", reg.Available)
	}
	a, _ := Encode(reg)
	b, _ := Encode(reg)
	if string(a) != string(b) || !strings.Contains(string(a), `"theme": null`) || !strings.HasPrefix(string(a), "{\n  \"format\": 1,\n  \"modules\": [") {
		t.Errorf("encoding:\n%s", a)
	}
}

func TestCollectReportsEveryProblemWithFileAndKey(t *testing.T) {
	root := t.TempDir()
	bad := writeModule(t, filepath.Join(root, "bad"), map[string]any{"id": "bad", "contract": 1})
	newer := manifestFor("newer", KindPagesFunction, "")
	newer["contract"] = 7
	newerDir := writeModule(t, filepath.Join(root, "newer"), newer)
	standalone := writeModule(t, filepath.Join(root, "standalone"), manifestFor("standalone", KindWorker, ""))
	a := writeModule(t, filepath.Join(root, "a"), manifestFor("a", KindPagesFunction, "/panel.html"))
	b := writeModule(t, filepath.Join(root, "b"), manifestFor("b", KindPagesFunction, "/panel.html"))
	unreadable := filepath.Join(root, "unreadable")
	if err := os.MkdirAll(filepath.Join(unreadable, "module.json"), 0o750); err != nil {
		t.Fatal(err)
	}
	reg, findings := Collect(Input{
		ConfigFile: "site.yaml", Schema: mustSchema(t), Templates: fstest.MapFS{},
		Workers: []Worker{{"workers[0]", bad}, {"workers[1]", newerDir}, {"workers[2]", standalone}, {"workers[3]", a}, {"workers[4]", b}, {"workers[5]", unreadable}},
		Refs: []Ref{
			{ID: "a", URL: "/x"},
			{ID: "other", URL: "/y", Dir: a},
			{ID: "missing", URL: "/y/sub", Dir: filepath.Join(root, "nowhere")},
			{ID: "z", URL: "/y"},
		},
	})
	var got []string
	for _, f := range findings {
		got = append(got, f.File+" → "+f.Detail)
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{
		filepath.Join(bad, "module.json") + " → manifest.description: is required",
		filepath.Join(bad, "module.json") + " → workers[0] is left out of the module registry until its manifest is fixed",
		`module "newer" uses manifest contract 7, but this reader understands contract 1 at most`,
		`manifest.kind is "worker", a standalone Worker, but workers[2] builds it into this site's functions/`,
		filepath.Join(unreadable, "module.json") + " → read ",
		`site.yaml → modules[1].id is "other", but ` + filepath.Join(a, "module.json") + ` says "a"`,
		"site.yaml → modules[2].dir: " + filepath.Join(root, "nowhere") + " has no module.json",
		`site.yaml → modules[0]: module id "a" is already registered by workers[3] — only the first is kept`,
		`site.yaml → workers[4]: the panel of "b" is /panel.html, which is already the panel of "a" (workers[3])`,
		"site.yaml → modules[1].url /y overlaps modules[2].url /y/sub — give each module a base path of its own",
		"site.yaml → modules[3].url /y overlaps modules[1].url /y",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing finding %q in\n%s", want, all)
		}
	}
	var ids []string
	for _, e := range reg.Modules {
		ids = append(ids, e.ID)
	}
	if strings.Join(ids, ",") != "a,b,missing,other,standalone,z" {
		t.Errorf("modules %v: broken bundled manifests are left out, declared modules stay", ids)
	}
}

func TestCheckRef(t *testing.T) {
	ok := []Ref{{ID: "newsletter-ui", URL: "/newsletter-admin"}, {ID: "a1", URL: "/a/b/"}, {ID: "x", URL: "/~u/x.y_z"}}
	for _, r := range ok {
		if err := CheckRef("site.yaml", "modules[0]", r); err != nil {
			t.Errorf("%+v: %v", r, err)
		}
	}
	for _, url := range []string{"", "/", "https://example.com/x", "//example.com/x", "/a/../b", "/a/./b", "/a\\b", "/a?b=1", "/a#b", "a", "/a//b", "/.hidden"} {
		err := CheckRef("site.yaml", "modules[2]", Ref{ID: "m", URL: url})
		if err == nil || !strings.Contains(err.Error(), "site.yaml: modules[2].url") {
			t.Errorf("url %q: %v", url, err)
		}
	}
	for _, id := range []string{"", "Upper", "1st", "a_b", strings.Repeat("a", 65)} {
		err := CheckRef("site.yaml", "modules[1]", Ref{ID: id, URL: "/x"})
		if err == nil || !strings.Contains(err.Error(), "site.yaml: modules[1].id") {
			t.Errorf("id %q: %v", id, err)
		}
	}
}

func TestReadManifestEdges(t *testing.T) {
	if m, errs := mustSchema(t).ReadManifest([]byte("{")); m != nil || len(errs) != 1 || errs[0] != "manifest: not valid JSON" {
		t.Errorf("%v %q", m, errs)
	}
	defer func(p string) { embeddedSchemaPath = p }(embeddedSchemaPath)
	embeddedSchemaPath = "workers/none.json"
	if _, err := EmbeddedSchema(); err == nil {
		t.Error("a missing embedded schema is an error")
	}
}

func TestIsCore(t *testing.T) {
	root := t.TempDir()
	core := writeModule(t, filepath.Join(root, "core"), map[string]any{"id": "core"})
	other := writeModule(t, filepath.Join(root, "other"), map[string]any{"id": "comments"})
	broken := filepath.Join(root, "broken")
	_ = os.MkdirAll(broken, 0o750)
	_ = os.WriteFile(filepath.Join(broken, "module.json"), []byte("{"), 0o600)
	if !IsCore(core) || IsCore(other) || IsCore(broken) || IsCore(filepath.Join(root, "none")) {
		t.Error("IsCore reads the id of dir/module.json and nothing else")
	}
}

func TestPathsAndEncode(t *testing.T) {
	if panelURL("/", "/p.html#x") != "/p.html" || panelURL("/admin", "/#/c") != "/admin/" {
		t.Error("panelURL")
	}
	if cleanBase("/") != "/" || cleanBase("/a/") != "/a" || !within("/a/b", "/a") || within("/ab", "/a") {
		t.Error("cleanBase/within")
	}
	data, err := Encode(Registry{Format: RegistryFormat})
	if err != nil || string(data) != "{\n  \"format\": 1,\n  \"modules\": [],\n  \"available\": [],\n  \"theme\": null\n}\n" {
		t.Errorf("empty registry: %q %v", data, err)
	}
	if _, err := Encode(Registry{Modules: []Entry{{Manifest: func() {}}}}); err == nil {
		t.Error("an unencodable value is an error")
	}
}

func TestRender(t *testing.T) {
	data, findings, err := Render(Input{Templates: fstest.MapFS{}})
	if err != nil || len(findings) != 0 || !strings.Contains(string(data), `"modules": []`) {
		t.Errorf("%s %v %v", data, findings, err)
	}
	defer func(p string) { embeddedSchemaPath = p }(embeddedSchemaPath)
	embeddedSchemaPath = "workers/none.json"
	if _, _, err := Render(Input{}); err == nil {
		t.Error("a missing embedded schema is an error")
	}
}

// TestSiteFilePath (GO-152): a file on this host, nothing that could leave
// it or escape a CSS string.
func TestSiteFilePath(t *testing.T) {
	for _, ok := range []string{"/img/logo.svg", "/a", "/fonts/Brand_Serif-1.0.woff2", "/~x/y.png"} {
		if !SiteFilePath(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "/", "img/logo.svg", "//example.com/x.svg", "https://example.com/x.svg", "/img/../x.svg", "/img/./x.svg",
		"/.env", "/img/", "/img\\x.svg", "/img/x.svg?v=1", "/img/x.svg#a", "/img/x%20y.svg", "/img/x y.svg", `/img/x".svg`, "/img/x).svg",
		"/" + strings.Repeat("a", 256)} {
		if SiteFilePath(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
