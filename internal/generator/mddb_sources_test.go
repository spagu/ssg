package generator

// Several MDDB collections per build, filtered on the server (#361).

import (
	"errors"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/mddb"
	"github.com/spagu/ssg/internal/models"
)

// GetMatching applies filterMeta the way the server does: every key must hold
// one of its listed values.
func (f *fakeMddb) GetMatching(collection, _ string, filterMeta map[string][]any) ([]mddb.Document, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.requests = append(f.requests, collection)
	var out []mddb.Document
	for _, doc := range f.byCollection[collection] {
		if metaMatches(doc.Metadata, filterMeta) {
			out = append(out, doc)
		}
	}
	return out, nil
}

func metaMatches(meta map[string]any, filter map[string][]any) bool {
	for key, accepted := range filter {
		found := false
		for _, v := range accepted {
			if meta[key] == v {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func mddbDoc(key string, meta map[string]any) mddb.Document {
	meta["title"] = key
	meta["slug"] = key
	return mddb.Document{Key: key, Metadata: meta, Content: "# " + key}
}

func newMddbGen(sources ...MddbSource) *Generator {
	return &Generator{
		config: Config{Mddb: MddbConfig{Collection: "blog", Sources: sources}, PostURLFormat: "slug"},
		siteData: &models.SiteData{
			Categories: map[int]models.Category{}, Media: map[int]models.MediaItem{}, Authors: map[int]models.Author{},
		},
	}
}

// TestMddbSourcesFilterOnTheServer is the issue's config: private documents are
// never fetched, each collection lands as the type and section it names.
func TestMddbSourcesFilterOnTheServer(t *testing.T) {
	client := &fakeMddb{byCollection: map[string][]mddb.Document{
		"articles": {
			mddbDoc("open", map[string]any{"visibility": "public"}),
			mddbDoc("secret", map[string]any{"visibility": "private"}),
		},
		"events": {mddbDoc("meetup", map[string]any{})},
		"kb":     {mddbDoc("faq", map[string]any{"type": "post"})},
	}}
	g := newMddbGen(
		MddbSource{Collection: "articles", Filter: map[string][]string{"visibility": {"public"}}, Type: "post", Section: "articles"},
		MddbSource{Collection: "events", Type: "post", Section: "events"},
		MddbSource{Collection: "kb", Type: "page"},
	)

	if err := g.loadMddbSources(client); err != nil {
		t.Fatal(err)
	}
	posts := map[string]models.Page{}
	for _, p := range g.siteData.Posts {
		posts[p.Slug] = p
	}
	if _, leaked := posts["secret"]; leaked {
		t.Error("a document the filter leaves out must not reach the build")
	}
	if p := posts["open"]; p.Section != "articles" || p.Type != "post" || p.URLFormat != "slug" {
		t.Errorf("articles: %+v", p)
	}
	if p := posts["meetup"]; p.Section != "events" {
		t.Errorf("events: section %q", p.Section)
	}
	// type: page wins over the document's own type meta.
	if len(g.siteData.Pages) != 1 || g.siteData.Pages[0].Slug != "faq" || g.siteData.Pages[0].Type != "page" {
		t.Errorf("kb pages: %+v", g.siteData.Pages)
	}
	if got := g.contentSection(posts["open"]); got != "articles" {
		t.Errorf("contentSection = %q, want articles", got)
	}
	if !pageInSource(posts["open"], "articles") || pageInSource(posts["open"], "events") {
		t.Error("a declared feed's source: must match the section")
	}
}

// TestMddbSingleCollectionStillReadsByType: no sources is the config every
// site had before, and each document's own type still decides.
func TestMddbSingleCollectionStillReadsByType(t *testing.T) {
	client := &fakeMddb{byCollection: map[string][]mddb.Document{"blog": {
		mddbDoc("about", map[string]any{"type": "page"}),
		mddbDoc("hello", map[string]any{"type": "post"}),
		mddbDoc("other", map[string]any{"type": "note"}),
	}}}
	g := newMddbGen()
	if err := g.loadMddbSources(client); err != nil {
		t.Fatal(err)
	}
	if len(g.siteData.Pages) != 1 || len(g.siteData.Posts) != 1 {
		t.Fatalf("pages %d posts %d, want 1 and 1", len(g.siteData.Pages), len(g.siteData.Posts))
	}
	if g.siteData.Posts[0].Section != "" || g.contentSection(g.siteData.Posts[0]) != "" {
		t.Error("a single collection has no section")
	}
	if strings.Join(client.requests, ",") != "blog,blog" {
		t.Errorf("requests = %v", client.requests)
	}
}

// TestMddbSourcesRejectWhatCannotBeRead, and pass a server error on.
func TestMddbSourcesRejectWhatCannotBeRead(t *testing.T) {
	for name, src := range map[string]MddbSource{
		"collection is required": {Type: "post"},
		"unsupported type":       {Collection: "kb", Type: "note"},
	} {
		if err := newMddbGen(src).loadMddbSources(&fakeMddb{}); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	failing := &fakeMddb{err: errors.New("down")}
	if err := newMddbGen().loadMddbSources(failing); err == nil || !strings.Contains(err.Error(), "loading pages from mddb") {
		t.Errorf("a server error must surface: %v", err)
	}
}

// TestMddbFilterCopies: the caller adds "type" to the result, which must not
// write into the configured filter.
func TestMddbFilterCopies(t *testing.T) {
	cfg := map[string][]string{"visibility": {"public", "members"}}
	got := mddbFilter(cfg)
	got["type"] = []any{"post"}
	if len(cfg) != 1 || len(got["visibility"]) != 2 || got["visibility"][1] != "members" {
		t.Errorf("filter = %v, config = %v", got, cfg)
	}
}
