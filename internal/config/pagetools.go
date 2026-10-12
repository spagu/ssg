package config

// The reader's page tools (1.8.72): `print:` — the Print / PDF button and
// print stylesheet — and `cook_mode:` — the step-by-step recipe view. See
// docs/PRINT_AND_COOK_MODE.md.

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// PrintConfig is the Print / PDF button.
type PrintConfig struct {
	Enabled bool   `yaml:"enabled" toml:"enabled" json:"enabled"`
	Label   string `yaml:"label" toml:"label" json:"label"` // button text; default "Print / PDF"
	// Sections limits the button to posts, pages, or both ("posts" by default).
	Sections []string `yaml:"sections" toml:"sections" json:"sections"`
	// Auto places the button after the first </h1> on pages whose theme
	// has no <span data-ssg-print-slot></span>. On unless set to false.
	Auto *bool `yaml:"auto" toml:"auto" json:"auto"`
	// Labels replaces the other texts: recipe, ingredients, steps, yield,
	// prep, cook, total, printed.
	Labels map[string]string `yaml:"labels" toml:"labels" json:"labels"`
}

// CookModeConfig is the Cook Mode button. `cook_mode: true` is shorthand
// for `cook_mode: {enabled: true}`.
type CookModeConfig struct {
	Enabled bool   `yaml:"enabled" toml:"enabled" json:"enabled"`
	Label   string `yaml:"label" toml:"label" json:"label"` // button text; default "Cook mode"
	// Auto places the button after the first </h1> on recipe pages whose
	// theme has no <span data-ssg-cook-slot></span>. On unless set to false.
	Auto *bool `yaml:"auto" toml:"auto" json:"auto"`
	// Labels replaces the dialog's texts: title, ingredients, step
	// ("Step {n} of {total}"), next, back, close, restart, done, finished.
	Labels map[string]string `yaml:"labels" toml:"labels" json:"labels"`
}

// cookModeFields decodes the mapping form without recursing into the
// custom decoders below.
type cookModeFields CookModeConfig

// UnmarshalYAML accepts `cook_mode: true` as well as the mapping.
func (c *CookModeConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(&c.Enabled)
	}
	return value.Decode((*cookModeFields)(c))
}

// UnmarshalJSON accepts `"cook_mode": true` as well as the object.
func (c *CookModeConfig) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &c.Enabled); err == nil {
		return nil
	}
	return json.Unmarshal(data, (*cookModeFields)(c))
}

// UnmarshalTOML accepts `cook_mode = true` as well as the table.
func (c *CookModeConfig) UnmarshalTOML(data interface{}) error {
	switch v := data.(type) {
	case bool:
		c.Enabled = v
		return nil
	case map[string]interface{}:
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, (*cookModeFields)(c))
	}
	return fmt.Errorf("cook_mode: expected true/false or a table")
}
