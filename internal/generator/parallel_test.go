package generator

import (
	"os"
	"path/filepath"
	"testing"
)

// writeParallelCorpus writes a small multi-page site (pages + posts + a shortcode
// + an .md link) and returns its root. Written ONCE per test: both builds must
// read the same files, because a post with no explicit `modified:` takes its feed
// <updated> from the source file's mtime. Writing a fresh copy per build made the
// two corpora differ whenever the writes straddled a second boundary, so the test
// failed intermittently on a difference it was never meant to measure — it bit a
// release tag before this was fixed.
func writeParallelCorpus(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(contentDir, "metadata.json"),
		`{"categories":[{"id":1,"name":"News","slug":"news"}],"exported_at":"","media":[],`+
			`"users":[{"id":1,"name":"Ed","slug":"ed"}]}`)
	for i := 0; i < 12; i++ {
		mustWrite(t, filepath.Join(contentDir, "posts", "news", "p"+string(rune('a'+i))+".md"),
			"---\ntitle: Post "+string(rune('A'+i))+"\nslug: p"+string(rune('a'+i))+
				"\nstatus: publish\ntype: post\ndate: 2024-01-0"+string(rune('1'+i%9))+
				"\ncategories: [News]\ntags: [go]\nauthor: 1\n---\n\nBody with a [link](pa.md).\n")
	}
	for i := 0; i < 6; i++ {
		mustWrite(t, filepath.Join(contentDir, "pages", "pg"+string(rune('a'+i))+".md"),
			"---\ntitle: Page "+string(rune('A'+i))+"\nslug: pg"+string(rune('a'+i))+
				"\nstatus: publish\ntype: page\n---\n\nSome page body.\n")
	}
	writeSimpleTemplates(t, filepath.Join(tmp, "templates", "simple"))
	return tmp
}

// buildSiteWithWorkers builds the given corpus at a worker count, into its own
// output dir, and returns that dir. The corpus is shared so the worker count is
// the only difference between two builds. Exercises the shared render caches
// (mdCache, shortcode, mdLinkWarned).
func buildSiteWithWorkers(t *testing.T, root string, workers int) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "output")
	gen, err := New(Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir:   filepath.Join(root, "content"),
		TemplatesDir: filepath.Join(root, "templates"),
		OutputDir:    out,
		Feed:         true, RewriteMdLinks: true, BuildWorkers: workers, Quiet: true,
	})
	if err != nil {
		t.Fatalf("New(workers=%d): %v", workers, err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate(workers=%d): %v", workers, err)
	}
	return out
}

// snapshot maps every output file to its bytes.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		b, _ := os.ReadFile(path)
		out[rel] = string(b)
		return nil
	})
	return out
}

// TestRenderParallelDeterministic: a parallel build (8 workers) produces
// byte-for-byte the same output tree as a sequential one (1 worker). Run under
// -race, the parallel build also proves the shared render caches are race-free.
func TestRenderParallelDeterministic(t *testing.T) {
	root := writeParallelCorpus(t)
	seq := snapshot(t, buildSiteWithWorkers(t, root, 1))
	par := snapshot(t, buildSiteWithWorkers(t, root, 8))

	if len(seq) != len(par) {
		t.Fatalf("file count differs: sequential %d, parallel %d", len(seq), len(par))
	}
	for path, want := range seq {
		got, ok := par[path]
		if !ok {
			t.Errorf("parallel build missing %s", path)
			continue
		}
		if got != want {
			t.Errorf("output differs at %s (sequential vs 8 workers)", path)
		}
	}
}
