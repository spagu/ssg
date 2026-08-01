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
