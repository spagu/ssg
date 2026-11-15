package externalsource

// slug_transliterate (#426) for pages built from records: the site-wide
// switch reaches every source, folds the slug and keeps the plain one.

import "testing"

func TestMapRecordsTransliteratesWhenOn(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"name": "Crème brûlée"},
		map[string]interface{}{"name": "Plain Title"},
	}
	src := mappedSource(ContentMap{Title: "name"})
	imp, _, err := MapRecords(src, records)
	if err != nil {
		t.Fatal(err)
	}
	if imp.Posts[0].Slug != "cr-me-br-l-e" || imp.Posts[0].LegacySlug != "" {
		t.Errorf("off: slug %q legacy %q", imp.Posts[0].Slug, imp.Posts[0].LegacySlug)
	}

	src.SlugTransliterate = true
	imp, _, err = MapRecords(src, records)
	if err != nil {
		t.Fatal(err)
	}
	if imp.Posts[0].Slug != "creme-brulee" || imp.Posts[0].LegacySlug != "cr-me-br-l-e" {
		t.Errorf("on: slug %q legacy %q", imp.Posts[0].Slug, imp.Posts[0].LegacySlug)
	}
	if imp.Posts[1].Slug != "plain-title" || imp.Posts[1].LegacySlug != "" {
		t.Errorf("an ASCII title did not move: slug %q legacy %q", imp.Posts[1].Slug, imp.Posts[1].LegacySlug)
	}
}

func TestResolveCopiesSlugTransliterate(t *testing.T) {
	cfg := Config{Enabled: true, SlugTransliterate: true, Sources: map[string]SourceConfig{
		"alpha": {Type: "file", Path: "a.json"},
	}}
	sources, err := Resolve(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !sources[0].SlugTransliterate {
		t.Error("the site's switch did not reach the source")
	}
}
