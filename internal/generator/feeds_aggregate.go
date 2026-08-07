package generator

// Aggregating feeds (#89): several inputs — other sites' feeds and this site's
// own posts — merged into one published feed.
//
// The point of including your own content is that a "planet" without you is not
// your planet. An aggregate that only republishes other people reads as a link
// dump; mixing your own posts in makes it a section of your site that happens to
// draw on more than one source.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/externalsource"
	"github.com/spagu/ssg/internal/models"
)

// aggregateItems collects, filters, sorts and deduplicates the inputs of an
// aggregating feed.
func (g *Generator) aggregateItems(spec models.FeedSpec) []externalsource.FeedItem {
	var items []externalsource.FeedItem
	for _, in := range spec.Aggregate {
		var got []externalsource.FeedItem
		switch {
		case strings.TrimSpace(in.Source) != "":
			got = g.itemsFromExternalFeed(in)
		case strings.TrimSpace(in.Site) != "":
			got = g.itemsFromSite(in)
		default:
			continue
		}
		// Narrow at the source first: what counts as noise depends on the feed it
		// came from, and that context is gone once everything is merged.
		items = append(items, filterFeedItems(got, in.Include, in.Exclude)...)
	}
	// Then the feed-wide rules, for anything that should never appear whatever
	// its origin.
	items = filterFeedItems(items, spec.Include, spec.Exclude)
	items = dedupeFeedItems(items)
	// Newest first, with the title breaking ties so two items published in the
	// same second do not swap places between builds.
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].Published.Equal(items[j].Published) {
			return items[i].Published.After(items[j].Published)
		}
		return items[i].Title < items[j].Title
	})
	return items
}

// itemsFromExternalFeed reads one already-loaded external source. The source must
// be declared with `format: feed`; anything else has not been normalized and
// cannot be merged.
func (g *Generator) itemsFromExternalFeed(in models.FeedInput) []externalsource.FeedItem {
	name := strings.TrimSpace(in.Source)
	raw, ok := g.externalData[name]
	if !ok {
		fmt.Printf("   ⚠️  feeds: external source %q is not loaded — skipping\n", name)
		return nil
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		fmt.Printf("   ⚠️  feeds: external source %q is not a feed (use format: feed) — skipping\n", name)
		return nil
	}
	list, ok := m["items"].([]interface{})
	if !ok {
		fmt.Printf("   ⚠️  feeds: external source %q has no items (use format: feed) — skipping\n", name)
		return nil
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = name
	}
	out := make([]externalsource.FeedItem, 0, len(list))
	for _, e := range list {
		em, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		it := externalsource.FeedItem{
			ID: str(em["id"]), URL: str(em["url"]), Title: str(em["title"]),
			Summary: str(em["summary"]), ContentHTML: str(em["content_html"]),
			Author: str(em["author"]), Tags: strSlice(em["tags"]),
			Source: name, Label: label,
		}
		it.Published, _ = em["published"].(time.Time)
		it.Updated, _ = em["updated"].(time.Time)
		out = append(out, it)
	}
	return out
}

// itemsFromSite turns this site's own posts into feed items, so an aggregate can
// include the blog it is published on. `site: "*"` takes every post; anything
// else names a content folder.
func (g *Generator) itemsFromSite(in models.FeedInput) []externalsource.FeedItem {
	sel := models.FeedSpec{}
	if s := strings.TrimSpace(in.Site); s != "*" {
		sel.Source = s
	}
	posts := g.selectFeedPosts(sel)
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = g.config.Domain
	}
	out := make([]externalsource.FeedItem, 0, len(posts))
	for _, p := range posts {
		_, summary := g.feedBody(p, false)
		canonical := p.GetCanonical(g.config.Domain)
		out = append(out, externalsource.FeedItem{
			ID: canonical, URL: canonical, Title: p.Title, Summary: summary,
			Published: p.Date, Updated: g.lastModFor(p), Tags: p.Tags,
			Source: "site", Label: label,
		})
	}
	return out
}

// filterFeedItems applies the include and exclude lists. Exclusion wins: a feed
// republishing other people's writing has to be able to say "not this" with
// certainty, and an item matching both lists is far more likely to be the thing
// being excluded.
func filterFeedItems(items []externalsource.FeedItem, include, exclude models.FeedFilter) []externalsource.FeedItem {
	incWords, incTags := lowerSet(include.Words), lowerSet(include.Tags)
	excWords, excTags := lowerSet(exclude.Words), lowerSet(exclude.Tags)
	if incWords == nil && incTags == nil && excWords == nil && excTags == nil {
		return items
	}
	out := make([]externalsource.FeedItem, 0, len(items))
	for _, it := range items {
		if matchesFeedFilter(it, excWords, excTags) {
			continue
		}
		if (incWords != nil || incTags != nil) && !matchesFeedFilter(it, incWords, incTags) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// matchesFeedFilter reports whether an item matches any word or tag. Words are
// matched against the title and summary — the parts a reader sees — rather than
// the full body, so a passing mention deep in an article does not drop it.
func matchesFeedFilter(it externalsource.FeedItem, words, tags map[string]bool) bool {
	if words != nil {
		hay := strings.ToLower(it.Title + " " + it.Summary)
		for w := range words {
			if strings.Contains(hay, w) {
				return true
			}
		}
	}
	if tags != nil {
		for _, t := range it.Tags {
			if tags[strings.ToLower(strings.TrimSpace(t))] {
				return true
			}
		}
	}
	return false
}

// dedupeFeedItems keeps the first occurrence of a URL. The same post reached
// through two feeds — a site's own feed and an aggregator's — is one item, and
// publishing it twice is the most visible way an aggregate looks broken.
func dedupeFeedItems(items []externalsource.FeedItem) []externalsource.FeedItem {
	seen := make(map[string]bool, len(items))
	out := make([]externalsource.FeedItem, 0, len(items))
	for _, it := range items {
		key := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(it.URL)), "/")
		if key == "" {
			key = strings.ToLower(it.ID)
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, it)
	}
	return out
}

func str(v interface{}) string {
	s, _ := v.(string)
	return s
}

func strSlice(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
