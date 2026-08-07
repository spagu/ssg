package generator

import (
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/externalsource"
	"github.com/spagu/ssg/internal/models"
)

// aggGen wires two external feeds and one own post, the shape a "planet" has.
func aggGen(t *testing.T) *Generator {
	t.Helper()
	g := newTestGen(t, "")
	g.config.Domain = "tradik.com"
	g.siteData.Posts = []models.Page{{
		Slug: "own", Title: "Our own post", Type: "post", Status: "publish", SourceDir: "blog",
		Date: time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC), Tags: []string{"tradik"}, Excerpt: "Ours.",
	}}
	mk := func(title, url, tag string, day int) map[string]interface{} {
		return map[string]interface{}{
			"id": url, "url": url, "title": title, "summary": "s",
			"tags": []string{tag}, "published": time.Date(2026, 8, day, 0, 0, 0, 0, time.UTC),
		}
	}
	g.externalData = map[string]interface{}{
		"ssg": map[string]interface{}{"items": []interface{}{
			mk("Making builds faster", "https://ssg.tradik.com/a/", "performance", 3),
			mk("Sponsored: buy this", "https://ssg.tradik.com/ad/", "ads", 2),
		}},
		"mddb": map[string]interface{}{"items": []interface{}{
			mk("Markdown as a database", "https://mddb.tradik.com/b/", "database", 5),
			mk("Conference recap", "https://mddb.tradik.com/c/", "events", 5),
		}},
	}
	return g
}

func aggTitles(items []externalsource.FeedItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

// TestAggregateMergesSourcesAndOwnPosts covers the headline of #89: two other
// sites' feeds and this site's own content in one feed, newest first, each item
// labelled with where it came from.
func TestAggregateMergesSourcesAndOwnPosts(t *testing.T) {
	g := aggGen(t)
	items := g.aggregateItems(models.FeedSpec{Aggregate: []models.FeedInput{
		{Source: "ssg", Label: "SSG"},
		{Source: "mddb", Label: "MDDB"},
		{Site: "blog", Label: "Tradik"},
	}})
	if len(items) != 5 {
		t.Fatalf("merged %d items, want 5: %v", len(items), aggTitles(items))
	}
	// Newest first, across sources.
	if items[0].Title != "Our own post" {
		t.Errorf("expected the newest item first, got %v", aggTitles(items))
	}
	// Provenance survives the merge — without it an aggregate cannot be grouped.
	labels := map[string]string{}
	for _, it := range items {
		labels[it.Title] = it.Label
	}
	if labels["Markdown as a database"] != "MDDB" || labels["Our own post"] != "Tradik" {
		t.Errorf("labels lost: %v", labels)
	}
	// The label reaches the output as a term, which is what makes it sortable.
	terms := feedItemTerms(items[0])
	if !strings.Contains(strings.Join(terms, ","), "Tradik") {
		t.Errorf("label missing from item terms: %v", terms)
	}
}

// TestAggregateFilters: per-source narrowing runs before the feed-wide rules, so
// "noise" can be defined per feed — the context that disappears once merged.
func TestAggregateFilters(t *testing.T) {
	g := aggGen(t)
	items := g.aggregateItems(models.FeedSpec{
		Aggregate: []models.FeedInput{
			{Source: "ssg", Label: "SSG"},
			{Source: "mddb", Label: "MDDB", Exclude: models.FeedFilter{Tags: []string{"events"}}},
			{Site: "blog", Label: "Tradik"},
		},
		Exclude: models.FeedFilter{Words: []string{"sponsored"}},
	})
	got := strings.Join(aggTitles(items), " | ")
	if strings.Contains(got, "Conference recap") {
		t.Errorf("per-source exclude did not apply: %s", got)
	}
	if strings.Contains(got, "Sponsored") {
		t.Errorf("feed-wide word exclude did not apply: %s", got)
	}
	if len(items) != 3 {
		t.Errorf("want 3 items after filtering, got %d: %s", len(items), got)
	}

	// include narrows to matching items only; exclude still wins over it.
	only := g.aggregateItems(models.FeedSpec{
		Aggregate: []models.FeedInput{{Source: "ssg"}, {Source: "mddb"}},
		Include:   models.FeedFilter{Tags: []string{"database", "ads"}},
		Exclude:   models.FeedFilter{Tags: []string{"ads"}},
	})
	if len(only) != 1 || only[0].Title != "Markdown as a database" {
		t.Errorf("include/exclude precedence wrong: %v", aggTitles(only))
	}
}

// TestAggregateDedupes: the same URL reached through two feeds is one item —
// publishing it twice is the most visible way an aggregate looks broken.
func TestAggregateDedupes(t *testing.T) {
	g := aggGen(t)
	dup := g.externalData["ssg"].(map[string]interface{})
	g.externalData["mirror"] = map[string]interface{}{"items": dup["items"]}
	items := g.aggregateItems(models.FeedSpec{Aggregate: []models.FeedInput{
		{Source: "ssg"}, {Source: "mirror"},
	}})
	if len(items) != 2 {
		t.Errorf("the same two URLs from two feeds must merge to two items, got %d: %v",
			len(items), aggTitles(items))
	}
}

// TestAggregateMissingSource: an unloaded or non-feed source warns and is skipped
// rather than failing a build over one unreachable site.
func TestAggregateMissingSource(t *testing.T) {
	g := aggGen(t)
	g.externalData["notafeed"] = map[string]interface{}{"rows": []interface{}{}}
	items := g.aggregateItems(models.FeedSpec{Aggregate: []models.FeedInput{
		{Source: "absent"}, {Source: "notafeed"}, {Site: "blog"},
	}})
	if len(items) != 1 {
		t.Errorf("only the site input should contribute, got %v", aggTitles(items))
	}
}

// TestFeedPagePath: page one keeps the declared path, so the advertised URL never
// moves as the archive grows.
func TestFeedPagePath(t *testing.T) {
	cases := map[int]string{1: "planet.xml", 2: "planet-2.xml", 7: "planet-7.xml"}
	for n, want := range cases {
		if got := feedPagePath("planet.xml", n); got != want {
			t.Errorf("feedPagePath(n=%d) = %q, want %q", n, got, want)
		}
	}
	if got := feedPagePath("a/b/feed.json", 3); got != "a/b/feed-3.json" {
		t.Errorf("nested path paginated wrong: %q", got)
	}
}

// TestTmplFeedItems covers #91: a template can reach a declared feed's items, so
// an HTML page shows the same list the feed carries — filters, dedupe, ordering
// and labels included — instead of re-implementing them and drifting.
func TestTmplFeedItems(t *testing.T) {
	g := aggGen(t)
	g.config.Feeds = []models.FeedSpec{{
		Path: "/planet.xml", Name: "planet", Title: "Planet",
		Aggregate: []models.FeedInput{
			{Source: "ssg", Label: "SSG"},
			{Source: "mddb", Label: "MDDB"},
			{Site: "blog", Label: "Tradik"},
		},
		Exclude: models.FeedFilter{Words: []string{"sponsored"}},
	}}

	byPath := g.tmplFeedItems("/planet.xml")
	if len(byPath) != 4 {
		t.Fatalf("got %d items, want 4 (one excluded): %v", len(byPath), aggTitles(byPath))
	}
	// The feed's own rules applied — this is the whole point of reusing it.
	if strings.Contains(strings.Join(aggTitles(byPath), " "), "Sponsored") {
		t.Error("the feed's exclude must apply to the template view too")
	}
	if byPath[0].Title != "Our own post" {
		t.Errorf("ordering must match the feed (newest first): %v", aggTitles(byPath))
	}
	if byPath[0].Label != "Tradik" {
		t.Errorf("provenance label must reach the template: %+v", byPath[0])
	}

	// Reachable by name as well as by path, whichever reads better in a theme.
	if byName := g.tmplFeedItems("planet"); len(byName) != len(byPath) {
		t.Errorf("lookup by name = %d items, by path = %d", len(byName), len(byPath))
	}
	// An undeclared feed returns nothing rather than breaking the render.
	if got := g.tmplFeedItems("/nope.xml"); got != nil {
		t.Errorf("unknown feed should yield nothing, got %v", aggTitles(got))
	}
	if got := g.tmplFeedItems(""); got != nil {
		t.Error("an empty key should yield nothing")
	}
	// Registered under the name a template actually calls.
	if _, ok := g.buildTemplateFuncs(nil)["feed"]; !ok {
		t.Error("the feed helper must be registered")
	}
}

// TestTmplFeedItemsRespectsItemCap: the template view is capped like the feed, so
// the page cannot quietly show more than the feed advertises.
func TestTmplFeedItemsRespectsItemCap(t *testing.T) {
	g := aggGen(t)
	two := 2
	g.config.Feeds = []models.FeedSpec{{
		Path: "/planet.xml", Items: &two,
		Aggregate: []models.FeedInput{{Source: "ssg"}, {Source: "mddb"}, {Site: "blog"}},
	}}
	if got := g.tmplFeedItems("/planet.xml"); len(got) != 2 {
		t.Errorf("items: 2 should cap the template view, got %d", len(got))
	}
}

// TestTmplConcatAndFlatten covers the general helpers #91 asked for: Go templates
// cannot compose two collections — `slice a b` builds a list OF two lists and
// nothing flattens it — so a page mixing site posts with external items had no
// way to produce one ordered list.
func TestTmplConcatAndFlatten(t *testing.T) {
	a := []string{"one", "two"}
	b := []interface{}{3, 4}

	got := tmplConcat(a, b)
	if len(got) != 4 {
		t.Fatalf("concat = %v, want four elements", got)
	}
	// Mixed element types are fine: the result is []any, which the collection
	// helpers handle by reflection, so concat composes with sort/first.
	if got[0] != "one" || got[3] != 4 {
		t.Errorf("concat lost or reordered elements: %v", got)
	}
	// A nil input contributes nothing rather than a nil hole for range to trip on.
	if n := len(tmplConcat(a, nil, b)); n != 4 {
		t.Errorf("nil input should contribute nothing, got %d elements", n)
	}
	if n := len(tmplConcat()); n != 0 {
		t.Errorf("concat of nothing should be empty, got %d", n)
	}
	// A non-collection is one element, so concat works with a single item too.
	if n := len(tmplConcat("solo")); n != 1 {
		t.Errorf("a scalar should count as one element, got %d", n)
	}

	// flatten unnests one level — the shape `slice a b` produces.
	flat := tmplFlatten([]interface{}{a, b})
	if len(flat) != 4 {
		t.Errorf("flatten = %v, want four elements", flat)
	}
	if n := len(tmplFlatten(nil)); n != 0 {
		t.Errorf("flatten(nil) should be empty, got %d", n)
	}
	for _, name := range []string{"concat", "flatten"} {
		if _, ok := newTestGen(t, "").buildTemplateFuncs(nil)[name]; !ok {
			t.Errorf("%s must be registered", name)
		}
	}
}
