package generator

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFn creates workers/<name>/functions/api/<file> with a tiny handler.
func writeFn(t *testing.T, root, api, body string) string {
	t.Helper()
	dir := filepath.Join(root, "functions", "api")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, api), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// GO-076: several independent workers build into the one Pages functions tree,
// their routes combined into a single _routes.json.
func TestMultipleWorkersBuildAndCombineRoutes(t *testing.T) {
	a := writeFn(t, t.TempDir(), "consent.ts", "export const onRequest = () => new Response('c')\n")
	b := writeFn(t, t.TempDir(), "comments.ts", "export const onRequest = () => new Response('m')\n")
	out := t.TempDir()

	g := &Generator{config: Config{OutputDir: out, Workers: []WorkerConfig{
		{Name: "cookie-consent", Dir: a, RoutesInclude: []string{"/api/consent"}},
		{Name: "comments", Dir: b, RoutesInclude: []string{"/api/comments"}},
	}}}
	if err := g.generateWorkerFiles(); err != nil {
		t.Fatalf("generateWorkerFiles: %v", err)
	}

	for _, f := range []string{"consent.ts", "comments.ts"} {
		if _, err := os.Stat(filepath.Join(out, "functions", "api", f)); err != nil {
			t.Errorf("missing %s in the merged functions tree: %v", f, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(out, "_routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc routesJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Include) != 2 {
		t.Errorf("routes include = %v, want both workers' routes", doc.Include)
	}
}

// Two workers providing the same output path is a hard error, never a silent
// overwrite.
func TestWorkerFunctionCollisionErrors(t *testing.T) {
	a := writeFn(t, t.TempDir(), "same.ts", "a\n")
	b := writeFn(t, t.TempDir(), "same.ts", "b\n")
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Workers: []WorkerConfig{
		{Name: "one", Dir: a}, {Name: "two", Dir: b},
	}}}
	err := g.generateWorkerFiles()
	if err == nil || !strings.Contains(err.Error(), "both provide") {
		t.Fatalf("collision not reported: %v", err)
	}
}

// Only one _worker.js can exist per project.
func TestTwoPrebuiltWorkersError(t *testing.T) {
	mk := func() string {
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "_worker.js"), []byte("export default {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return d
	}
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Workers: []WorkerConfig{
		{Name: "a", Dir: mk(), Mode: "worker"}, {Name: "b", Dir: mk(), Mode: "worker"},
	}}}
	if err := g.generateWorkerFiles(); err == nil || !strings.Contains(err.Error(), "only one _worker.js") {
		t.Fatalf("two prebuilt workers not rejected: %v", err)
	}
}

// The singular worker: still works unchanged (back-compat via ResolvedWorkers).
func TestSingularWorkerStillWorks(t *testing.T) {
	a := writeFn(t, t.TempDir(), "hi.ts", "x\n")
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Worker: WorkerConfig{Dir: a}}}
	if err := g.generateWorkerFiles(); err != nil {
		t.Fatalf("singular worker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "functions", "api", "hi.ts")); err != nil {
		t.Errorf("singular worker did not build: %v", err)
	}
}

// A remote source is fetched into a dir and then built like a local one.
func TestWorkerRemoteSourceFetched(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("repo-main/functions/api/remote.ts")
	_, _ = w.Write([]byte("export const onRequest = () => new Response('r')\n"))
	_ = zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write(buf.Bytes())
	}))
	defer srv.Close()

	dir := filepath.Join(t.TempDir(), "fetched")
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Workers: []WorkerConfig{
		{Name: "remote", Source: srv.URL + "/x.zip", Dir: dir},
	}}}
	if err := g.generateWorkerFiles(); err != nil {
		t.Fatalf("remote worker build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "functions", "api", "remote.ts")); err != nil {
		t.Errorf("remote worker not fetched+built: %v", err)
	}
}
