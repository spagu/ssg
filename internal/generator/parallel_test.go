package generator

import (
	"os"
	"path/filepath"
	"testing"
)

// buildSiteWithWorkers builds a small multi-page site (pages + posts + a shortcode
// + an .md link, across two languages) at the given worker count and returns the
// output dir. Exercises the shared render caches (mdCache, shortcode, mdLinkWarned).
func buildSiteWithWorkers(t *testing.T, workers int) string {
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

	gen, err := New(Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir:   filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir:    filepath.Join(tmp, "output"),
		Feed:         true, RewriteMdLinks: true, BuildWorkers: workers, Quiet: true,
	})
	if err != nil {
		t.Fatalf("New(workers=%d): %v", workers, err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate(workers=%d): %v", workers, err)
	}
	return filepath.Join(tmp, "output")
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
	seq := snapshot(t, buildSiteWithWorkers(t, 1))
	par := snapshot(t, buildSiteWithWorkers(t, 8))

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
