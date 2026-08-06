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
