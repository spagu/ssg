package generator

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/modules"
)

// templates is the repository's workers/ directory.
var templates = filepath.Join("..", "..", "workers")

// writeCore makes a fixture of the planned core worker (GO-145 will ship the
// real template): a module.json with id "core" and one function that imports
// the registry.
func writeCore(t *testing.T) string {
	t.Helper()
	dir := writeFn(t, t.TempDir(), "core.ts", "import registry from \"../_ssg-modules.json\" with { type: \"json\" };\n")
	text := map[string]string{"en": "Core", "pl": "Core", "hi": "Core", "de": "Core", "fr": "Core"}
	m := map[string]any{
		"id": "core", "contract": 1, "version": "1.0.0", "kind": "pages-function", "title": text, "description": text,
		"panel": "/admin/", "nav": []any{}, "permissions": []any{}, "endpoints": map[string]any{}, "disable": text,
	}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "module.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func registryBuild(t *testing.T, cfg Config) (string, string, error) {
	t.Helper()
	cfg.OutputDir = t.TempDir()
	cfg.ConfigPath = "site.yaml"
	g := &Generator{config: cfg}
	out, err := capture(t, g.generateWorkerFiles)
	return cfg.OutputDir, out, err
}

// publicFiles lists what `wrangler pages deploy` uploads as static assets:
// everything except its IGNORE_LIST (wrangler's pages/validate), which keeps
// functions/ and the host files out.
func publicFiles(t *testing.T, out string) []string {
	t.Helper()
	ignored := []string{"_worker.js", "_redirects", "_headers", "_routes.json", "functions"}
	var files []string
	err := filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(out, p)
		if slices.Contains(ignored, filepath.ToSlash(rel)) && rel != "." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			files = append(files, "/"+filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// routed reports whether a _routes.json include pattern sends path to Functions.
func routed(include []string, path string) bool {
	for _, p := range include {
		if prefix, ok := strings.CutSuffix(p, "*"); ok && strings.HasPrefix(path, prefix) || p == path {
			return true
		}
	}
	return false
}

// TestModuleRegistryWithCoreAndThreeModules (GO-144): exactly the site's
// modules, sorted, the same bytes every build, readable by functions only.
func TestModuleRegistryWithCoreAndThreeModules(t *testing.T) {
	cfg := Config{
		Workers: []WorkerConfig{
			{Name: "newsletter", Dir: filepath.Join(templates, "newsletter")},
			{Name: "core", Dir: writeCore(t)},
			{Name: "comments", Dir: filepath.Join(templates, "comments")},
		},
		Modules: []modules.Ref{{ID: "newsletter-ui", URL: "/newsletter-admin", Dir: filepath.Join(templates, "newsletter-ui")}},
	}
	out, printed, err := registryBuild(t, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(printed, "module manifests valid") || !strings.Contains(printed, "functions/_ssg-modules.json") {
		t.Errorf("output:\n%s", printed)
	}
	data, err := os.ReadFile(filepath.Join(out, "functions", modules.RegistryFile))
	if err != nil {
		t.Fatal(err)
	}
	var reg modules.Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range reg.Modules {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != "comments,core,newsletter,newsletter-ui" || reg.Format != 1 || reg.Theme != nil {
		t.Errorf("registry %s", data)
	}
	for _, a := range reg.Available {
		if slices.Contains(ids, a.ID) || a.Title == "" {
			t.Errorf("available %+v", a)
		}
	}
	again, _, _ := registryBuild(t, cfg)
	if data2, _ := os.ReadFile(filepath.Join(again, "functions", modules.RegistryFile)); string(data2) != string(data) {
		t.Error("two builds wrote different registries")
	}
	for _, f := range publicFiles(t, out) {
		if strings.Contains(f, "_ssg-modules") {
			t.Errorf("%s is a public file", f)
		}
	}
	var routes struct{ Include []string }
	raw, _ := os.ReadFile(filepath.Join(out, "_routes.json"))
	_ = json.Unmarshal(raw, &routes)
	for _, p := range []string{"/_ssg-modules.json", "/functions/_ssg-modules.json"} {
		if routed(routes.Include, p) {
			t.Errorf("_routes.json %v routes %s", routes.Include, p)
		}
	}
}

// TestNoCoreWritesNothingNew: without a core worker the output is the worker
// trees, their public files and _routes.json — the pre-GO-144 output.
func TestNoCoreWritesNothingNew(t *testing.T) {
	comments := filepath.Join(templates, "comments")
	out, printed, err := registryBuild(t, Config{Workers: []WorkerConfig{{Name: "comments", Dir: comments}}})
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, sub := range []string{"functions", "public"} {
		root := filepath.Join(comments, sub)
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(root, p)
				if sub == "functions" {
					rel = filepath.Join("functions", rel)
				}
				want = append(want, filepath.ToSlash(rel))
			}
			return err
		})
	}
	want = append(want, "_routes.json")
	var got []string
	_ = filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(out, p)
			got = append(got, filepath.ToSlash(rel))
		}
		return err
	})
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("output files changed:\n got %v\nwant %v", got, want)
	}
	if strings.Contains(printed, "module") {
		t.Errorf("a site without core hears nothing about modules:\n%s", printed)
	}
}

func TestModulesWithoutCore(t *testing.T) {
	ref := []modules.Ref{{ID: "newsletter-ui", URL: "/newsletter-admin"}}
	_, printed, err := registryBuild(t, Config{Modules: ref})
	if err != nil || !strings.Contains(printed, "modules in site.yaml → modules: lists 1 module(s), but no worker in workers: is the core worker") {
		t.Errorf("%v\n%s", err, printed)
	}
	if _, _, err := registryBuild(t, Config{Modules: ref, CheckModules: "strict"}); err == nil {
		t.Error("strict fails on a modules: list without core")
	}
	if _, printed, err := registryBuild(t, Config{Modules: ref, CheckModules: "off"}); err != nil || printed != "" {
		t.Errorf("off is silent: %v %q", err, printed)
	}
	_, _, err = registryBuild(t, Config{Modules: []modules.Ref{{ID: "x", URL: "https://example.com/x"}}})
	if err == nil || !strings.Contains(err.Error(), `site.yaml: modules[0].url "https://example.com/x"`) {
		t.Errorf("an off-host url is a configuration error: %v", err)
	}
}

func TestModuleRegistryFindingsAndErrors(t *testing.T) {
	core := writeCore(t)
	bad := writeFn(t, t.TempDir(), "bad.ts", "x\n")
	if err := os.WriteFile(filepath.Join(bad, "module.json"), []byte(`{"id": "bad", "contract": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Worker: WorkerConfig{Dir: core}, Workers: []WorkerConfig{{Dir: core}, {Dir: bad}}}
	_, printed, err := registryBuild(t, cfg)
	if err != nil || !strings.Contains(printed, "workers[1] is left out of the module registry") {
		t.Errorf("warn by default: %v\n%s", err, printed)
	}
	cfg.CheckModules = "strict"
	if _, _, err := registryBuild(t, cfg); err == nil || !strings.Contains(err.Error(), "module problem(s)") {
		t.Errorf("strict: %v", err)
	}

	// A worker that ships a file of the registry's name.
	clash := writeFn(t, t.TempDir(), "x.ts", "x\n")
	if err := os.WriteFile(filepath.Join(clash, "functions", modules.RegistryFile), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err = registryBuild(t, Config{Workers: []WorkerConfig{{Name: "c", Dir: core}, {Name: "x", Dir: clash}}})
	if err == nil || !strings.Contains(err.Error(), `worker "x" provides functions/_ssg-modules.json`) {
		t.Errorf("clash: %v", err)
	}

	// The singular worker: is named "worker"; a broken embedded schema stops the build.
	g := &Generator{config: Config{OutputDir: t.TempDir(), Worker: WorkerConfig{Dir: core}}}
	if g.workerKey(0) != "worker" || g.configFileName() != ".ssg.yaml" {
		t.Error("singular worker key / default config name")
	}
	defer func(f func(modules.Input) ([]byte, []modules.Finding, error)) { renderModuleRegistry = f }(renderModuleRegistry)
	renderModuleRegistry = func(modules.Input) ([]byte, []modules.Finding, error) { return nil, nil, errors.New("schema") }
	if err := g.generateWorkerFiles(); err == nil || !strings.Contains(err.Error(), "module registry: schema") {
		t.Errorf("render error: %v", err)
	}
	renderModuleRegistry = func(modules.Input) ([]byte, []modules.Finding, error) { return []byte("{}"), nil, nil }
	if err := os.MkdirAll(filepath.Join(g.config.OutputDir, "functions", modules.RegistryFile), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := capture(t, g.generateWorkerFiles); err == nil || !strings.Contains(err.Error(), "module registry:") {
		t.Errorf("write error: %v", err)
	}
}
