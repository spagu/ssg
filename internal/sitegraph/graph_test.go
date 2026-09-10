package sitegraph

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sample(n int) Graph {
	g := Graph{Schema: Schema, Domain: "example.com",
		Build:      Build{Version: "test", Time: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)},
		Sections:   []Section{{Path: "/tag/go/", Kind: "tag", Title: "Go"}},
		Taxonomies: []Taxonomy{{Name: "tag", Path: "tag", Terms: []Term{{Name: "Go", Slug: "go", URL: "/tag/go/", Count: n}}}},
		Redirects:  []Redirect{{From: "/old/", To: "/new/", Status: 301}},
		Links:      []Link{{From: "/a/", To: "/b/", Kind: "page"}},
	}
	for i := 0; i < n; i++ {
		g.Pages = append(g.Pages, Page{URL: "/p" + strings.Repeat("x", i%3) + "/", Type: "post", Title: "P", Canonical: "https://example.com/p/"})
	}
	return g
}

// TestEncodeIsDeterministic: the same graph encodes to the same bytes, which is
// what lets a hash mean "the site changed" rather than "the map iterated
// differently".
func TestEncodeIsDeterministic(t *testing.T) {
	a, err := Encode(sample(3))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Encode(sample(3))
	if !bytes.Equal(a, b) {
		t.Error("two encodings of one graph differ")
	}
	if !strings.Contains(string(a), `"schema": 1`) || strings.Contains(string(a), `<`) {
		t.Errorf("unexpected encoding:\n%s", a)
	}
}

// TestHashIgnoresTheClock: built a minute apart, the same site hashes equal.
func TestHashIgnoresTheClock(t *testing.T) {
	g1, g2 := sample(2), sample(2)
	g2.Build.Time = g2.Build.Time.Add(time.Hour)
	g2.Build.Hash = "stale"
	h1, _ := Hash(g1)
	h2, _ := Hash(g2)
	if h1 != h2 {
		t.Errorf("hash changed with the clock: %s vs %s", h1, h2)
	}
	g2.Pages[0].Title = "changed"
	if h3, _ := Hash(g2); h3 == h1 {
		t.Error("hash did not change with the content")
	}
	if err := g1.Stamp(); err != nil || g1.Build.Hash != h1 {
		t.Errorf("Stamp = %q, %v; want %q", g1.Build.Hash, err, h1)
	}
}

// TestWriteAndLoadSingleFile: a small site is one file, and it reads back whole.
func TestWriteAndLoadSingleFile(t *testing.T) {
	dir := t.TempDir()
	files, err := Write(dir, sample(3), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != FileName {
		t.Errorf("files = %v", files)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 3 || len(got.Links) != 1 || got.Redirects[0].To != "/new/" || got.Taxonomies[0].Terms[0].Count != 3 {
		t.Errorf("round trip lost data: %+v", got)
	}
}

// TestWriteAndLoadSharded: past the threshold, site-graph.json is an index and
// the records live in JSON Lines — and Load hides the difference.
func TestWriteAndLoadSharded(t *testing.T) {
	dir := t.TempDir()
	g := sample(12)
	files, err := Write(dir, g, 5) // threshold 5, shard size is 5000 so pages land in one shard
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 3 || files[0] != FileName {
		t.Fatalf("files = %v", files)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, FileName))
	// The index names its shards and inlines nothing that scales with the
	// site: no page record (no canonical) lives in it.
	if !strings.Contains(string(raw), `"sharded": true`) || strings.Contains(string(raw), `"canonical"`) {
		t.Errorf("index should not inline pages:\n%s", raw)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 12 || len(got.Links) != 1 || len(got.Sections) != 1 {
		t.Errorf("sharded round trip lost data: pages=%d links=%d sections=%d", len(got.Pages), len(got.Links), len(got.Sections))
	}
}

// TestLoadRefusesWhatItCannotRead: no file, a wrong schema, and a broken shard
// are three distinct, named failures — never a silently empty graph.
func TestLoadRefusesWhatItCannotRead(t *testing.T) {
	if _, err := Load(t.TempDir()); err != ErrNotFound {
		t.Errorf("missing file: %v, want ErrNotFound", err)
	}
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, FileName), []byte(`{"schema": 99}`), 0o644)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "schema 99") {
		t.Errorf("wrong schema: %v", err)
	}
	_ = os.WriteFile(filepath.Join(dir, FileName), []byte(`not json`), 0o644)
	if _, err := Load(dir); err == nil {
		t.Error("garbage parsed")
	}
	// A sharded index whose shard is missing.
	_ = os.WriteFile(filepath.Join(dir, FileName), []byte(`{"schema":1,"sharded":true,"shards":{"pages":["site-graph/pages-1.jsonl"],"links":"site-graph/links.jsonl"}}`), 0o644)
	if _, err := Load(dir); err == nil {
		t.Error("a missing shard was not an error")
	}
	// A shard with a broken line.
	_ = os.MkdirAll(filepath.Join(dir, ShardDir), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ShardDir, "pages-1.jsonl"), []byte("{bad\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ShardDir, "links.jsonl"), []byte(""), 0o644)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "pages-1.jsonl") {
		t.Errorf("broken shard: %v", err)
	}
}
