package config

// mddb.sources: several collections per build, each filtered (#361).

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestMddbSourcesParse reads the issue's config as written.
func TestMddbSourcesParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ssg.yaml")
	body := `mddb:
  url: http://mddb:11023
  sources:
    - collection: articles
      filter: { visibility: [public] }
      type: post
      section: articles
    - collection: events
      type: post
      section: events
    - collection: kb
      type: page
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Mddb.Sources) != 3 {
		t.Fatalf("sources = %+v", cfg.Mddb.Sources)
	}
	first := cfg.Mddb.Sources[0]
	if first.Collection != "articles" || first.Type != "post" || first.Section != "articles" ||
		!reflect.DeepEqual(first.Filter, map[string][]string{"visibility": {"public"}}) {
		t.Errorf("first source = %+v", first)
	}
	if got := cfg.Mddb.Collections(); !reflect.DeepEqual(got, []string{"articles", "events", "kb"}) {
		t.Errorf("Collections = %v", got)
	}
}

// TestMddbCollections: the single collection still counts, a collection named
// twice is watched once, and nothing configured is nothing.
func TestMddbCollections(t *testing.T) {
	if got := (MddbConfig{Collection: "blog"}).Collections(); !reflect.DeepEqual(got, []string{"blog"}) {
		t.Errorf("single = %v", got)
	}
	if got := (MddbConfig{}).Collections(); got != nil {
		t.Errorf("none = %v", got)
	}
	twice := MddbConfig{Collection: "ignored", Sources: []MddbSource{
		{Collection: "news"}, {Collection: ""}, {Collection: "news", Type: "page"},
	}}
	if got := twice.Collections(); !reflect.DeepEqual(got, []string{"news"}) {
		t.Errorf("sources = %v", got)
	}
}
