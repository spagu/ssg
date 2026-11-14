package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	ssgroot "github.com/spagu/ssg"
)

// pagesSnippetBefore123 is the text `ssg new worker` printed for every
// template before GO-123, pinned literally: a Pages Functions template must
// keep printing exactly this.
const pagesSnippetBefore123 = `Add this to your .ssg.yaml:

worker:
  dir: workers/%s
  mode: functions
  routes_include:
    - /api/*

Then set the secrets listed in workers/%s/README.md via ` + "`wrangler pages secret put`" + `.
`

// TestPagesTemplatesScaffoldAsBefore (GO-123): every template without its own
// wrangler.toml lands byte for byte as embedded and prints the same text as
// before standalone Workers existed.
func TestPagesTemplatesScaffoldAsBefore(t *testing.T) {
	checked := 0
	for _, name := range availableWorkerTemplates() {
		root := "workers/" + name
		if _, err := fs.Stat(ssgroot.EmbeddedWorkers, root+"/wrangler.toml"); err == nil {
			continue
		}
		if _, err := fs.Stat(ssgroot.EmbeddedWorkers, root+"/functions"); err != nil {
			continue // an untracked local leftover, not a template (see embedded.go)
		}
		checked++
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			out, code := captureGraphOutput(t, func() int { return runNewWorker([]string{name}) })
			if code != 0 {
				t.Fatalf("exit %d", code)
			}
			// A template that declares its own routes (auth: /auth/* pages)
			// prints them instead of the generic /api/* — checked by
			// TestWorkerConfigSnippetUsesTheTemplatesRoutes. Every other one
			// prints exactly what it always did.
			snippet := fmt.Sprintf(pagesSnippetBefore123, name, name)
			if include, _ := templateRoutes(name); !slices.Equal(include, []string{"/api/*"}) {
				snippet = workerConfigSnippet(name)
			}
			want := fmt.Sprintf("✅ Scaffolded worker %q into workers/%s/\n\n", name, name) + snippet
			if out != want {
				t.Errorf("output changed:\n%s\nwant:\n%s", out, want)
			}
			assertSameTree(t, root, filepath.Join("workers", name))
			assertModuleManifest(t, name)
		})
	}
	if checked < 10 {
		t.Fatalf("only %d Pages templates checked", checked)
	}
}

// assertSameTree compares every embedded file under root with its copy on disk.
func assertSameTree(t *testing.T, root, dest string) {
	t.Helper()
	err := fs.WalkDir(ssgroot.EmbeddedWorkers, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		want, _ := ssgroot.EmbeddedWorkers.ReadFile(p)
		got, rerr := os.ReadFile(filepath.Join(dest, filepath.FromSlash(strings.TrimPrefix(p, root+"/"))))
		if rerr != nil || !bytes.Equal(got, want) {
			t.Errorf("%s differs from the embedded file (%v)", p, rerr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// assertModuleManifest (GO-143): the one file every template gained in 1.9.9 is
// its module.json, scaffolded with the rest of the tree, and it names the
// template it ships in. The format itself is checked by workers/modules.test.ts.
func assertModuleManifest(t *testing.T, name string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("workers", name, "module.json"))
	if err != nil {
		t.Fatalf("scaffold lacks module.json: %v", err)
	}
	var m struct {
		ID       string `json:"id"`
		Contract int    `json:"contract"`
	}
	if err := json.Unmarshal(b, &m); err != nil || m.ID != name || m.Contract != 1 {
		t.Errorf("module.json: id %q contract %d (%v), want id %q contract 1", m.ID, m.Contract, err, name)
	}
}

// TestLooseFilesUnderWorkersAreNotTemplates (GO-143): workers/ also holds the
// manifest schema, its reader and the cross-template test. Only directories
// are templates, so none of them is listed or can be scaffolded — and the
// schema still rides in the binary for the build to read later.
func TestLooseFilesUnderWorkersAreNotTemplates(t *testing.T) {
	loose := []string{"module.schema.json", "module-manifest.ts", "modules.test.ts"}
	for _, f := range loose {
		if _, err := fs.Stat(ssgroot.EmbeddedWorkers, "workers/"+f); err != nil {
			t.Errorf("workers/%s is not embedded: %v", f, err)
		}
	}
	for _, name := range availableWorkerTemplates() {
		if strings.Contains(name, ".") {
			t.Errorf("%q is listed as a template", name)
		}
	}
	t.Chdir(t.TempDir())
	for _, f := range loose {
		_, code := captureGraphOutput(t, func() int { return runNewWorker([]string{f}) })
		if code != 1 {
			t.Errorf("ssg new worker %s: exit %d, want 1 (unknown template)", f, code)
		}
	}
	if _, err := os.Stat("workers"); err == nil {
		t.Error("refusing a loose file must not create workers/")
	}
}

// TestNewsletterUIScaffoldsAsStandaloneWorker (GO-123): the template ships its
// own wrangler.toml, so it gets deploy steps, never a worker: block.
func TestNewsletterUIScaffoldsAsStandaloneWorker(t *testing.T) {
	t.Chdir(t.TempDir())
	out, code := captureGraphOutput(t, func() int { return runNewWorker([]string{"newsletter-ui"}) })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{
		"standalone Cloudflare Worker",
		"cd workers/newsletter-ui",
		"npx wrangler deploy",
		"npx wrangler queues create ssg-newsletter-send",
		"SAME id as the `newsletter` template's D1",
		"`wrangler secret put`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("instructions lack %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "routes_include") || strings.Contains(out, "mode: functions") {
		t.Errorf("a standalone Worker must not be offered a worker: block:\n%s", out)
	}
	assertSameTree(t, "workers/newsletter-ui", filepath.Join("workers", "newsletter-ui"))
	assertModuleManifest(t, "newsletter-ui")
	if _, err := os.Stat(filepath.Join("workers", "newsletter-ui", "functions")); err == nil {
		t.Error("newsletter-ui must not ship a functions/ tree")
	}
}

func TestStandaloneWorkerInstructionsStopAtTheCommentBlock(t *testing.T) {
	got := standaloneWorkerInstructions("x", []byte("# step one\n#\n#   step two\nname = \"x\"\n# not this\n"))
	if !strings.Contains(got, "step one\n\n  step two\n") || strings.Contains(got, "not this") {
		t.Fatalf("got:\n%s", got)
	}
	if bare := standaloneWorkerInstructions("x", []byte("name = \"x\"\n")); !strings.Contains(bare, "workers/x/README.md") {
		t.Fatalf("got:\n%s", bare)
	}
}

func TestWorkerTemplatesInWithoutWorkersDir(t *testing.T) {
	if got := workerTemplatesIn(fstest.MapFS{}); got != nil {
		t.Fatalf("got %v", got)
	}
}

// unreadableFS lists a file it then refuses to read.
type unreadableFS struct{ fstest.MapFS }

func (u unreadableFS) ReadFile(string) ([]byte, error) { return nil, errors.New("disk on fire") }
func (u unreadableFS) Open(name string) (fs.File, error) {
	if strings.HasSuffix(name, ".ts") {
		return nil, errors.New("disk on fire")
	}
	return u.MapFS.Open(name)
}

func TestExtractTemplateFromSurfacesErrors(t *testing.T) {
	dest := t.TempDir()
	if err := extractTemplateFrom(fstest.MapFS{}, "workers/missing", dest); err == nil {
		t.Error("a missing root must fail")
	}
	fsys := unreadableFS{fstest.MapFS{"workers/w/a.ts": {Data: []byte("x")}}}
	if err := extractTemplateFrom(fsys, "workers/w", dest); err == nil || !strings.Contains(err.Error(), "disk on fire") {
		t.Errorf("a read failure must surface, got %v", err)
	}
}
