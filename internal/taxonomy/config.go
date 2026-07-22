// Package taxonomy implements the generic taxonomy system (audit/
// taxonomies-feature.md): any number of user-defined classifications with
// per-term archives, replacing the separate category/tag/series pipelines
// with one registry while keeping every legacy URL, template, model field and
// helper intact.
package taxonomy

import (
	"fmt"
	"regexp"
	"sort"
)

// Definition is one fully-resolved taxonomy configuration.
type Definition struct {
	Name          string
	Field         string // frontmatter field holding assignments (default: Name)
	Label         string // plural display label (default: title-cased Name)
	Singular      string
	Path          string // URL segment (default: Name)
	Multiple      bool
	Archive       bool
	Feed          bool
	Sitemap       bool
	Template      string // taxonomy index template (default: taxonomy.html chain)
	TermTemplate  string // term archive template (default: taxonomy-term.html chain)
	Sort          string // term ordering: name (default) | count | weight
	CaseSensitive bool
	Slugify       bool
	GenerateEmpty bool
	Paginate      int  // posts per term-archive page; 0 = fall back to the global paginate (#44)
	Legacy        bool // category/tag/series: rendered by the legacy pipeline
}

// DefinitionConfig is the YAML/TOML/JSON shape; pointer booleans distinguish
// "unset" from "false" so user overrides merge onto defaults.
type DefinitionConfig struct {
	Label         string `yaml:"label" toml:"label" json:"label"`
	Singular      string `yaml:"singular" toml:"singular" json:"singular"`
	Path          string `yaml:"path" toml:"path" json:"path"`
	Field         string `yaml:"field" toml:"field" json:"field"`
	Template      string `yaml:"template" toml:"template" json:"template"`
	TermTemplate  string `yaml:"term_template" toml:"term_template" json:"term_template"`
	Sort          string `yaml:"sort" toml:"sort" json:"sort"`
	Multiple      *bool  `yaml:"multiple" toml:"multiple" json:"multiple"`
	Archive       *bool  `yaml:"archive" toml:"archive" json:"archive"`
	Feed          *bool  `yaml:"feed" toml:"feed" json:"feed"`
	Sitemap       *bool  `yaml:"sitemap" toml:"sitemap" json:"sitemap"`
	CaseSensitive *bool  `yaml:"case_sensitive" toml:"case_sensitive" json:"case_sensitive"`
	Slugify       *bool  `yaml:"slugify" toml:"slugify" json:"slugify"`
	GenerateEmpty *bool  `yaml:"generate_empty" toml:"generate_empty" json:"generate_empty"`
	Paginate      *int   `yaml:"paginate" toml:"paginate" json:"paginate"`
}

// nameRe constrains taxonomy names to safe identifier/URL material.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// legacyNames are auto-registered and keep their historical pipelines.
var legacyNames = []string{"category", "tag", "series"}

// legacyDefaults returns the built-in category/tag/series definitions matching
// today's behaviour exactly (URLs, templates, feeds, sitemap).
func legacyDefaults() map[string]Definition {
	return map[string]Definition{
		"category": {Name: "category", Field: "categories", Label: "Categories", Singular: "Category",
			Path: "category", Multiple: true, Archive: true, Feed: true, Sitemap: true,
			Template: "category.html", TermTemplate: "category.html", Sort: "name", Slugify: true, Legacy: true},
		"tag": {Name: "tag", Field: "tags", Label: "Tags", Singular: "Tag",
			Path: "tag", Multiple: true, Archive: true, Feed: true, Sitemap: true,
			Template: "tag.html", TermTemplate: "tag.html", Sort: "name", Slugify: true, Legacy: true},
		"series": {Name: "series", Field: "series", Label: "Series", Singular: "Series",
			Path: "series", Multiple: false, Archive: true, Feed: false, Sitemap: true,
			Template: "series.html", TermTemplate: "series.html", Sort: "name", Slugify: true, Legacy: true},
	}
}

// boolOr resolves a pointer-boolean against its default.
func boolOr(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// Resolve merges user configuration onto the legacy defaults and validates the
// result. reserved lists URL segments taxonomies may not claim (e.g. "author",
// "page", configured language codes). Returned names are deterministic:
// category, tag, series first, then user taxonomies alphabetically.
func Resolve(user map[string]DefinitionConfig, reserved []string) (map[string]Definition, []string, error) {
	defs := legacyDefaults()

	customNames := make([]string, 0, len(user))
	for name, uc := range user {
		if !nameRe.MatchString(name) {
			return nil, nil, fmt.Errorf("invalid taxonomy name %q (want lowercase letters, digits, _ or -)", name)
		}
		base, isLegacy := defs[name]
		if !isLegacy {
			base = Definition{Name: name, Field: name, Label: titleCase(name), Singular: titleCase(name),
				Path: name, Multiple: true, Archive: true, Feed: false, Sitemap: true, Sort: "name", Slugify: true}
			customNames = append(customNames, name)
		}
		defs[name] = applyOverrides(base, uc)
	}
	sort.Strings(customNames)
	names := append(append([]string{}, legacyNames...), customNames...)

	if err := validate(defs, names, reserved); err != nil {
		return nil, nil, err
	}
	return defs, names, nil
}

// applyOverrides copies explicitly-set user values onto a base definition.
func applyOverrides(base Definition, uc DefinitionConfig) Definition {
	if uc.Label != "" {
		base.Label = uc.Label
	}
	if uc.Singular != "" {
		base.Singular = uc.Singular
	}
	if uc.Path != "" {
		base.Path = uc.Path
	}
	if uc.Field != "" {
		base.Field = uc.Field
	}
	if uc.Template != "" {
		base.Template = uc.Template
	}
	if uc.TermTemplate != "" {
		base.TermTemplate = uc.TermTemplate
	}
	if uc.Sort != "" {
		base.Sort = uc.Sort
	}
	if uc.Paginate != nil {
		base.Paginate = *uc.Paginate
	}
	base.Multiple = boolOr(uc.Multiple, base.Multiple)
	base.Archive = boolOr(uc.Archive, base.Archive)
	base.Feed = boolOr(uc.Feed, base.Feed)
	base.Sitemap = boolOr(uc.Sitemap, base.Sitemap)
	base.CaseSensitive = boolOr(uc.CaseSensitive, base.CaseSensitive)
	base.Slugify = boolOr(uc.Slugify, base.Slugify)
	base.GenerateEmpty = boolOr(uc.GenerateEmpty, base.GenerateEmpty)
	return base
}

// validate enforces unique paths, sane sort values and reserved segments.
func validate(defs map[string]Definition, names, reserved []string) error {
	res := make(map[string]bool, len(reserved))
	for _, r := range reserved {
		res[r] = true
	}
	paths := map[string]string{}
	for _, name := range names {
		d := defs[name]
		if d.Path == "" {
			return fmt.Errorf("taxonomy %q has an empty path", name)
		}
		if res[d.Path] && !d.Legacy {
			return fmt.Errorf("taxonomy %q uses reserved path %q", name, d.Path)
		}
		if prev, dup := paths[d.Path]; dup {
			return fmt.Errorf("taxonomies %q and %q share the path %q", prev, name, d.Path)
		}
		paths[d.Path] = name
		switch d.Sort {
		case "name", "count", "weight":
		default:
			return fmt.Errorf("taxonomy %q: unsupported sort %q (want name, count or weight)", name, d.Sort)
		}
		if d.Paginate < 0 {
			return fmt.Errorf("taxonomy %q: paginate must be >= 0", name)
		}
	}
	return nil
}

// titleCase upper-cases the first rune (ASCII is enough for config names).
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}
