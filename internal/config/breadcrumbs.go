package config

// `breadcrumbs:` — the derived JSON-LD BreadcrumbList (1.8.73, #335). See
// docs/STRUCTURED_DATA.md.

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// BreadcrumbsConfig names the steps of the derived BreadcrumbList.
// `breadcrumbs: false` emits none, so a theme can supply its own;
// `breadcrumbs: true` (or no key at all) keeps the derived one.
type BreadcrumbsConfig struct {
	// Enabled is nil when unset, which means on.
	Enabled *bool `yaml:"enabled" toml:"enabled" json:"enabled"`
	// Home is the first step's label; default "Home".
	Home string `yaml:"home" toml:"home" json:"home"`
	// Sections maps a URL segment ("posts") to the name and target of its
	// step. A section with a URL is always emitted; one without keeps the
	// rule that a step needs a page behind it.
	Sections map[string]BreadcrumbSection `yaml:"sections" toml:"sections" json:"sections"`
}

// BreadcrumbSection is one configured step.
type BreadcrumbSection struct {
	Name string `yaml:"name" toml:"name" json:"name"`
	URL  string `yaml:"url" toml:"url" json:"url"`
}

// breadcrumbsFields decodes the mapping form without recursing into the
// custom decoders below.
type breadcrumbsFields BreadcrumbsConfig

// UnmarshalYAML accepts `breadcrumbs: false` as well as the mapping.
func (b *BreadcrumbsConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		var on bool
		if err := value.Decode(&on); err != nil {
			return err
		}
		b.Enabled = &on
		return nil
	}
	return value.Decode((*breadcrumbsFields)(b))
}

// UnmarshalJSON accepts `"breadcrumbs": false` as well as the object.
func (b *BreadcrumbsConfig) UnmarshalJSON(data []byte) error {
	var on bool
	if err := json.Unmarshal(data, &on); err == nil {
		b.Enabled = &on
		return nil
	}
	return json.Unmarshal(data, (*breadcrumbsFields)(b))
}

// UnmarshalTOML accepts `breadcrumbs = false` as well as the table.
func (b *BreadcrumbsConfig) UnmarshalTOML(data interface{}) error {
	switch v := data.(type) {
	case bool:
		b.Enabled = &v
		return nil
	case map[string]interface{}:
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, (*breadcrumbsFields)(b))
	}
	return fmt.Errorf("breadcrumbs: expected true/false or a table")
}
