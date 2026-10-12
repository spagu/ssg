package config

// Google Web Stories (1.8.72): `web_stories:` makes an AMP story from each
// selected post, plus a hub listing them. See docs/WEB_STORIES.md.

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// WebStoriesConfig is `web_stories:`. `web_stories: true` is shorthand for
// `web_stories: {enabled: true}`.
type WebStoriesConfig struct {
	Enabled bool `yaml:"enabled" toml:"enabled" json:"enabled"`
	// Sections picks the content: posts (default), pages, or a content
	// directory such as posts/recipes.
	Sections []string `yaml:"sections" toml:"sections" json:"sections"`
	// URL is the hub; stories live under it. Default /web-stories/.
	URL string `yaml:"url" toml:"url" json:"url"`
	// PublisherLogo is a square image of at least 96×96 px, as a path the
	// image pipeline finds (static/, assets/, the content or theme
	// directory). AMP requires it: without it no story is made.
	PublisherLogo string `yaml:"publisher_logo" toml:"publisher_logo" json:"publisher_logo"`
	// Pages is the number of photo pages between the cover and the last
	// page; default 4.
	Pages int `yaml:"pages" toml:"pages" json:"pages"`
	// MinImages is how many images, featured included, a post needs to get a
	// story; default 2.
	MinImages int `yaml:"min_images" toml:"min_images" json:"min_images"`
	// CTA is the last page's link text; default "Read the full article".
	CTA string `yaml:"cta" toml:"cta" json:"cta"`
}

// webStoriesFields decodes the mapping form without recursing into the
// custom decoders below.
type webStoriesFields WebStoriesConfig

// UnmarshalYAML accepts `web_stories: true` as well as the mapping.
func (w *WebStoriesConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(&w.Enabled)
	}
	return value.Decode((*webStoriesFields)(w))
}

// UnmarshalJSON accepts `"web_stories": true` as well as the object.
func (w *WebStoriesConfig) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &w.Enabled); err == nil {
		return nil
	}
	return json.Unmarshal(data, (*webStoriesFields)(w))
}

// UnmarshalTOML accepts `web_stories = true` as well as the table.
func (w *WebStoriesConfig) UnmarshalTOML(data interface{}) error {
	switch v := data.(type) {
	case bool:
		w.Enabled = v
		return nil
	case map[string]interface{}:
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, (*webStoriesFields)(w))
	}
	return fmt.Errorf("web_stories: expected true/false or a table")
}
