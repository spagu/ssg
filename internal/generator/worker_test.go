package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderRoutesJSON_DefaultsAndContent(t *testing.T) {
	data, err := renderRoutesJSON(nil, nil)
	if err != nil {
		t.Fatalf("renderRoutesJSON: %v", err)
	}
	var doc routesJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if doc.Version != 1 || len(doc.Include) != 1 || doc.Include[0] != "/api/*" {
		t.Fatalf("unexpected default routes: %+v", doc)
	}
	if doc.Exclude == nil {
		t.Fatal("exclude should be an empty array, not null")
	}
}

func TestRenderRoutesJSON_CapExceeded(t *testing.T) {
	include := make([]string, cfMaxRoutesRules+1)
	for i := range include {
		include[i] = "/a"
	}
	if _, err := renderRoutesJSON(include, nil); err == nil {
		t.Fatal("expected a rule-cap error")
	}
}

func TestGenerateWorkerFiles_FunctionsMode(t *testing.T) {
	src := t.TempDir()
	fnDir := filepath.Join(src, "functions", "api")
	if err := os.MkdirAll(fnDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fnDir, "hello.ts"), []byte("export const onRequest = () => new Response('hi')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Worker: WorkerConfig{Dir: src, Mode: "functions"}}}
	if err := g.generateWorkerFiles(); err != nil {
		t.Fatalf("generateWorkerFiles: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "functions", "api", "hello.ts")); err != nil {
		t.Fatalf("function file not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "_routes.json")); err != nil {
		t.Fatalf("_routes.json not written: %v", err)
	}
}

func TestGenerateWorkerFiles_WorkerModeMissingFile(t *testing.T) {
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Worker: WorkerConfig{Dir: t.TempDir(), Mode: "worker"}}}
	if err := g.generateWorkerFiles(); err == nil {
		t.Fatal("expected an error when _worker.js is missing")
	}
}

func TestGenerateWorkerFiles_WorkerModeCopies(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "_worker.js"), []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Worker: WorkerConfig{Dir: src, Mode: "worker"}}}
	if err := g.generateWorkerFiles(); err != nil {
		t.Fatalf("generateWorkerFiles: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "_worker.js")); err != nil {
		t.Fatalf("_worker.js not copied: %v", err)
	}
}

func TestGenerateWorkerFiles_NoWorkerConfigured(t *testing.T) {
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out}}
	if err := g.generateWorkerFiles(); err != nil {
		t.Fatalf("expected no-op, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "_routes.json")); !os.IsNotExist(err) {
		t.Fatal("_routes.json should not be written when no worker is configured")
	}
}

func TestGenerateWorkerFiles_MissingFunctionsDir(t *testing.T) {
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Worker: WorkerConfig{Dir: filepath.Join(t.TempDir(), "nope"), Mode: "functions"}}}
	if err := g.generateWorkerFiles(); err == nil {
		t.Fatal("expected an error for a missing functions directory")
	}
}

func TestGenerateWorkerFiles_WarnsBareImport(t *testing.T) {
	src := t.TempDir()
	fnDir := filepath.Join(src, "functions", "api")
	if err := os.MkdirAll(fnDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A bare npm import should be flagged (best-effort; never fails the build).
	if err := os.WriteFile(filepath.Join(fnDir, "pay.ts"), []byte("import Stripe from \"stripe\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Worker: WorkerConfig{Dir: src}}}
	if err := g.generateWorkerFiles(); err != nil {
		t.Fatalf("generateWorkerFiles should not fail on a bare import: %v", err)
	}
}

func TestGenerateWorkerFiles_ParentDirWithFunctions(t *testing.T) {
	// Passing the project dir (parent of functions/) is accepted too.
	src := t.TempDir()
	fnDir := filepath.Join(src, "functions")
	if err := os.MkdirAll(fnDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fnDir, "x.ts"), []byte("export const onRequest = () => {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Worker: WorkerConfig{Dir: src}}}
	if err := g.generateWorkerFiles(); err != nil {
		t.Fatalf("generateWorkerFiles: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "functions", "x.ts")); err != nil {
		t.Fatalf("function not copied from parent dir: %v", err)
	}
}

func TestGenerateWorkerFiles_UnknownMode(t *testing.T) {
	out := t.TempDir()
	g := &Generator{config: Config{OutputDir: out, Worker: WorkerConfig{Dir: t.TempDir(), Mode: "bogus"}}}
	if err := g.generateWorkerFiles(); err == nil {
		t.Fatal("expected an error for an unknown mode")
	}
}

func TestBareImportSpec(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		`import { Foo } from "stripe"`:             {"stripe", true},
		`import x from './local'`:                  {"", false},
		`import y from "/abs"`:                     {"", false},
		`import { env } from "cloudflare:workers"`: {"", false},
		`const z = 1`:                              {"", false},
	}
	for line, want := range cases {
		got, ok := bareImportSpec(line)
		if ok != want.ok || got != want.want {
			t.Fatalf("bareImportSpec(%q) = (%q,%v), want (%q,%v)", line, got, ok, want.want, want.ok)
		}
	}
}

func TestIsWorkerSourceFile(t *testing.T) {
	if !isWorkerSourceFile("a.ts") || !isWorkerSourceFile("b.mjs") {
		t.Fatal("expected TS/MJS to be worker sources")
	}
	if isWorkerSourceFile("c.css") {
		t.Fatal("CSS is not a worker source")
	}
}
