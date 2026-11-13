package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeStandalone creates a directory shaped like the newsletter-ui template:
// its own wrangler.toml, src/, public/, and no functions/ tree (GO-123).
func writeStandalone(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range map[string]string{
		"wrangler.toml":     "name = \"ssg-newsletter-ui\"\nmain = \"src/index.ts\"\n",
		"src/index.ts":      "export default {}\n",
		"public/index.html": "<!doctype html>\n",
	} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestIsStandaloneWorkerDir(t *testing.T) {
	standalone := writeStandalone(t)
	if !isStandaloneWorkerDir(standalone) {
		t.Error("own wrangler.toml and no functions/: standalone")
	}

	plain := writeFn(t, t.TempDir(), "contact.ts", "x\n")
	if isStandaloneWorkerDir(plain) {
		t.Error("a Functions tree without wrangler.toml is not standalone")
	}

	// A Pages project root that keeps its wrangler.toml beside functions/.
	pagesRoot := writeFn(t, t.TempDir(), "contact.ts", "x\n")
	if err := os.WriteFile(filepath.Join(pagesRoot, "wrangler.toml"), []byte("name = \"site\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if isStandaloneWorkerDir(pagesRoot) {
		t.Error("wrangler.toml beside functions/ is a Pages project, not standalone")
	}

	// A Pages config without a functions/ dir (dir pointing at the handlers).
	pagesCfg := t.TempDir()
	if err := os.WriteFile(filepath.Join(pagesCfg, "wrangler.toml"), []byte("pages_build_output_dir = \"./output\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if isStandaloneWorkerDir(pagesCfg) {
		t.Error("a Pages wrangler.toml is not standalone")
	}
}

// GO-123: a standalone Worker listed under workers: must not be copied into the
// site's functions/ (its sources and tests would become public routes) nor have
// its panel files published; the build stops with what to do instead.
func TestGenerateWorkerFilesRefusesStandaloneWorker(t *testing.T) {
	out := t.TempDir()
	dir := writeStandalone(t)
	g := &Generator{config: Config{OutputDir: out, Workers: []WorkerConfig{{Name: "newsletter-ui", Dir: dir}}}}
	err := g.generateWorkerFiles()
	if err == nil || !strings.Contains(err.Error(), "standalone Worker") || !strings.Contains(err.Error(), "wrangler deploy") {
		t.Fatalf("want a standalone-Worker error, got %v", err)
	}
	for _, p := range []string{"functions", "index.html", "_routes.json"} {
		if _, statErr := os.Stat(filepath.Join(out, p)); statErr == nil {
			t.Errorf("%s written for a refused worker", p)
		}
	}
}

// mode: worker with a prebuilt _worker.js may keep its own wrangler.toml: that
// layout is unchanged.
func TestPrebuiltWorkerWithWranglerTomlStillBuilds(t *testing.T) {
	out := t.TempDir()
	dir := writeStandalone(t)
	if err := os.WriteFile(filepath.Join(dir, "_worker.js"), []byte("export default {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &Generator{config: Config{OutputDir: out, Workers: []WorkerConfig{{Dir: dir, Mode: "worker"}}}}
	if err := g.generateWorkerFiles(); err != nil {
		t.Fatalf("generateWorkerFiles: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "_worker.js")); err != nil {
		t.Fatal(err)
	}
}

// `ssg new wrangler` folds only wrangler.snippet.toml fragments: a standalone
// Worker's own wrangler.toml never ends up in the site's Pages config.
func TestRenderWranglerConfigLeavesStandaloneWorkerOut(t *testing.T) {
	dir := writeStandalone(t)
	got, err := renderWranglerConfig("example.com", "output", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "ssg-newsletter-ui") || strings.Contains(got, "src/index.ts") {
		t.Fatalf("standalone wrangler.toml folded into the Pages config:\n%s", got)
	}
}

// Overlapping include and exclude rules from several workers are folded into
// their splats, both lists, and the build says so (#252, GO-081).
func TestGenerateWorkerFilesCollapsesBothRouteLists(t *testing.T) {
	a := writeFn(t, t.TempDir(), "a.ts", "a\n")
	b := writeFn(t, t.TempDir(), "b.ts", "b\n")
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Workers: []WorkerConfig{
		{Name: "a", Dir: a, RoutesInclude: []string{"/api/*"}, RoutesExclude: []string{"/api/static/*"}},
		{Name: "b", Dir: b, RoutesInclude: []string{"/api/b"}, RoutesExclude: []string{"/api/static/x"}},
	}}}
	if err := g.generateWorkerFiles(); err != nil {
		t.Fatalf("generateWorkerFiles: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, "_routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc routesJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if strings.Join(doc.Include, ",") != "/api/*" || strings.Join(doc.Exclude, ",") != "/api/static/*" {
		t.Fatalf("routes = %+v, want each list folded into its splat", doc)
	}
}
