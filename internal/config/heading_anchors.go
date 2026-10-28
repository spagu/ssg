package config

// `heading_anchors:` — a self-link on each heading that has an id (#364).

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// HeadingAnchorsConfig appends `<a href="#id">#</a>` to headings.
// `heading_anchors: true` turns it on with the defaults, and so does a mapping
// that does not say `enabled: false`.
type HeadingAnchorsConfig struct {
	Enabled bool
	Levels  []int  // heading levels that get a link; default 1, 2, 3
	Symbol  string // link text; default "#"
	Class   string // link class; default "heading-anchor"
	// Label is the link's aria-label; {text} is the heading's text. Default
	// "Link to the section {text}".
	Label string
}

// headingAnchorsFields is the mapping form. Enabled is a pointer so a mapping
// without it can mean "on".
type headingAnchorsFields struct {
	Enabled *bool  `yaml:"enabled" toml:"enabled" json:"enabled"`
	Levels  []int  `yaml:"levels" toml:"levels" json:"levels"`
	Symbol  string `yaml:"symbol" toml:"symbol" json:"symbol"`
	Class   string `yaml:"class" toml:"class" json:"class"`
	Label   string `yaml:"label" toml:"label" json:"label"`
}

func (h *HeadingAnchorsConfig) set(f headingAnchorsFields) {
	*h = HeadingAnchorsConfig{
		Enabled: f.Enabled == nil || *f.Enabled,
		Levels:  f.Levels, Symbol: f.Symbol, Class: f.Class, Label: f.Label,
	}
}

// UnmarshalYAML accepts `heading_anchors: true` as well as the mapping.
func (h *HeadingAnchorsConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(&h.Enabled)
	}
	var f headingAnchorsFields
	if err := value.Decode(&f); err != nil {
		return err
	}
	h.set(f)
	return nil
}

// UnmarshalJSON accepts `"heading_anchors": true` as well as the object.
func (h *HeadingAnchorsConfig) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &h.Enabled); err == nil {
		return nil
	}
	var f headingAnchorsFields
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	h.set(f)
	return nil
}

// UnmarshalTOML accepts `heading_anchors = true` as well as the table.
func (h *HeadingAnchorsConfig) UnmarshalTOML(data interface{}) error {
	switch v := data.(type) {
	case bool:
		h.Enabled = v
		return nil
	case map[string]interface{}:
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return h.UnmarshalJSON(raw)
	}
	return fmt.Errorf("heading_anchors: expected true/false or a table")
}
