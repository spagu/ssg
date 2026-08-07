package models

// ContentSchema is a per-type frontmatter contract (#62): the fields a page of a
// given content type must carry, and the type/format rules each declared field
// obeys. It lives in models so both the config loader and the generator share one
// definition. Keyed by content type ("post", "page", …) in the site config.
type ContentSchema struct {
	Required []string             `yaml:"required" toml:"required" json:"required"`
	Fields   map[string]FieldRule `yaml:"fields" toml:"fields" json:"fields"`
}

// FieldRule constrains one frontmatter field. Type is one of string, int, bool,
// date, url, list or enum; Values enumerates the allowed values when Type=enum.
type FieldRule struct {
	Type   string   `yaml:"type" toml:"type" json:"type"`
	Values []string `yaml:"values" toml:"values" json:"values"`
}

// MetaLimits are the advisory character ranges --check-meta reports on (#76).
// Every field is optional: unset uses the built-in default, and an explicit 0
// switches that bound off. Lengths are counted in runes, so an accented or CJK
// title is measured as a reader sees it rather than in bytes.
//
// These produce warnings, never build failures. A headline that reads well at 62
// characters is worth more than one mangled to fit, and a check that blocked the
// build on it would simply get switched off the first time it fired.
type MetaLimits struct {
	TitleMin       *int `yaml:"title_min" toml:"title_min" json:"title_min"`
	TitleMax       *int `yaml:"title_max" toml:"title_max" json:"title_max"`
	DescriptionMin *int `yaml:"description_min" toml:"description_min" json:"description_min"`
	DescriptionMax *int `yaml:"description_max" toml:"description_max" json:"description_max"`
}

// StaticSource is one extra verbatim passthrough root, beyond the single
// static_dir (#84). Path may be a directory or a single file; Dest optionally
// places it under a prefix in the output instead of at the root. Copied after
// static_dir, so a later entry wins on a collision.
type StaticSource struct {
	Path string `yaml:"path" toml:"path" json:"path"`
	Dest string `yaml:"dest" toml:"dest" json:"dest"`
}

// FeedSpec is one declared syndication feed (#86). It answers two questions the
// built-in feeds cannot: WHICH posts, and in WHAT format.
//
// The built-in `feed: true` output is all-or-nothing — every post, or one
// taxonomy term — so a site with several content roots cannot offer "just the
// blog" or "just the guides", and "the three tags that mean release" needs three
// subscriptions. Selection here is a set of optional narrowings combined with
// AND; an entry with none of them covers the whole site at a path you choose.
type FeedSpec struct {
	Path   string `yaml:"path" toml:"path" json:"path"`       // output path, e.g. "/blog/feed.xml"
	Name   string `yaml:"name" toml:"name" json:"name"`       // optional handle for the `feed` template helper (#91)
	Title  string `yaml:"title" toml:"title" json:"title"`    // feed title; defaults to the site domain
	Format string `yaml:"format" toml:"format" json:"format"` // atom (default) | rss | json

	// Selection — all optional, combined with AND.
	Source     string   `yaml:"source" toml:"source" json:"source"`             // a content_sources path / content folder
	Categories []string `yaml:"categories" toml:"categories" json:"categories"` // category name or slug
	Tags       []string `yaml:"tags" toml:"tags" json:"tags"`
	Type       string   `yaml:"type" toml:"type" json:"type"` // "post" (default) or "page"

	// Aggregate turns this into a combined feed: several inputs — other sites'
	// feeds and this site's own content — merged, sorted newest first and
	// deduplicated by URL (#89). Empty means an ordinary feed of site content.
	Aggregate []FeedInput `yaml:"aggregate" toml:"aggregate" json:"aggregate"`

	// Exclude/Include filter an aggregated feed by word or tag.
	Exclude FeedFilter `yaml:"exclude" toml:"exclude" json:"exclude"`
	Include FeedFilter `yaml:"include" toml:"include" json:"include"`

	// Paginate splits the feed into pages of N items linked with RFC 5005
	// rel="next"/"prev", so a large archive does not ship as one huge document.
	Paginate int `yaml:"paginate" toml:"paginate" json:"paginate"`

	// Overrides for the site-wide defaults; nil inherits feed_items /
	// feed_full_content rather than repeating them per entry.
	Items       *int  `yaml:"items" toml:"items" json:"items"`
	FullContent *bool `yaml:"full_content" toml:"full_content" json:"full_content"`
}

// FeedInput is one input of an aggregating feed (#89): either an external feed
// declared in external_sources, or this site's own content.
//
// Label is provenance. Once items from several places are mixed, "where did this
// come from" is the first thing a reader and a template both need, and it cannot
// be recovered afterwards — so it is attached at the point of collection and
// carried into the output as a tag.
type FeedInput struct {
	Source string `yaml:"source" toml:"source" json:"source"` // an external_sources name
	Site   string `yaml:"site" toml:"site" json:"site"`       // this site: a content folder, or "*" for all posts
	Label  string `yaml:"label" toml:"label" json:"label"`

	// Per-input filters, applied as this source is collected and before the
	// feed-wide ones. Sources differ in character — one publishes release notes
	// among conference chatter, another tags everything — so a single rule for
	// the whole aggregate either lets noise through or drops wanted items from
	// the quiet sources. Narrowing at the source keeps each decision local to the
	// feed it is about (#89).
	Include FeedFilter `yaml:"include" toml:"include" json:"include"`
	Exclude FeedFilter `yaml:"exclude" toml:"exclude" json:"exclude"`
}

// FeedFilter drops items from an aggregated feed. Words match case-insensitively
// against the title and summary; tags match an item's tags or categories.
//
// Exclusion beats inclusion: a feed republishing other people's writing needs to
// be able to say "not this" with certainty, and an item matching both lists is
// far more likely to be the thing being excluded than a wanted one.
type FeedFilter struct {
	Words []string `yaml:"words" toml:"words" json:"words"`
	Tags  []string `yaml:"tags" toml:"tags" json:"tags"`
}
