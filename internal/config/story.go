package config

// Story mode (1.9.4): `story:` turns an article into a reading companion and
// a fullscreen presentation — reading bar, photo stage, widgets, narrator,
// music, share, deep links — and feeds the same slides to the AMP Web Story
// (`web_stories:`). See docs/STORY.md.
//
// Only enabled, sections and rules mean anything to the build; every other
// key is handed to the page script as it is (snake_case or camelCase), so a
// new presentation option needs no Go change.

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// StoryConfig is `story:`. `story: true` turns it on with the defaults; a
// mapping turns it on unless it says `enabled: false`.
type StoryConfig struct {
	Enabled bool
	// Sections picks the content: posts (default), pages, or a content
	// directory such as posts/recipes.
	Sections []string
	// Rules add settings to the pages they match, in order, before a post's
	// own `story:` front matter and its <name>.story.yaml.
	Rules []StoryRule
	// Settings is everything else, for the page script.
	Settings map[string]any
}

// StoryRule is one entry of `story.rules`.
type StoryRule struct {
	When StoryWhen
	Set  map[string]any
}

// StoryWhen matches a page. Every condition given must hold; within a list
// any value will do. An empty When matches every page.
type StoryWhen struct {
	Category []string // the page's category or categories
	Tags     []string
	Section  []string // a content directory, posts/recipes holding posts/recipes/x.md
	Contains string   // text the article contains
}

// UnmarshalYAML accepts `story: true` as well as the mapping.
func (s *StoryConfig) UnmarshalYAML(value *yaml.Node) error {
	var raw any
	if err := value.Decode(&raw); err != nil {
		return err
	}
	return s.fromRaw(raw)
}

// UnmarshalJSON accepts `"story": true` as well as the object.
func (s *StoryConfig) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	return s.fromRaw(raw)
}

// UnmarshalTOML accepts `story = true` as well as the table.
func (s *StoryConfig) UnmarshalTOML(data any) error {
	return s.fromRaw(data)
}

// fromRaw fills the config from a decoded value of any of the three formats.
func (s *StoryConfig) fromRaw(raw any) error {
	switch v := raw.(type) {
	case bool:
		*s = StoryConfig{Enabled: v}
		return nil
	case nil:
		*s = StoryConfig{}
		return nil
	case map[string]any:
		return s.fromMap(v)
	}
	return fmt.Errorf("story: expected true/false or a mapping")
}

// fromMap splits a mapping into what the build reads and what it passes on.
func (s *StoryConfig) fromMap(m map[string]any) error {
	*s = StoryConfig{Enabled: true, Settings: map[string]any{}}
	for k, v := range m {
		switch k {
		case "enabled":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("story.enabled: expected true or false")
			}
			s.Enabled = b
		case "sections":
			s.Sections = StringList(v)
		case "rules":
			rules, err := storyRules(v)
			if err != nil {
				return err
			}
			s.Rules = rules
		default:
			s.Settings[k] = v
		}
	}
	return nil
}

// storyRules reads `story.rules`: a list of {when: …, set: …}.
func storyRules(v any) ([]StoryRule, error) {
	var list []any
	switch t := v.(type) {
	case []any:
		list = t
	case []map[string]any: // TOML's array of tables
		for _, m := range t {
			list = append(list, m)
		}
	default:
		return nil, fmt.Errorf("story.rules: expected a list")
	}
	out := make([]StoryRule, 0, len(list))
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("story.rules[%d]: expected a mapping with when and set", i)
		}
		set, _ := m["set"].(map[string]any)
		when, _ := m["when"].(map[string]any)
		contains, _ := when["contains"].(string)
		out = append(out, StoryRule{Set: set, When: StoryWhen{
			Category: StringList(firstOf(when, "category", "categories")),
			Tags:     StringList(firstOf(when, "tags", "tag")),
			Section:  StringList(firstOf(when, "section", "sections")),
			Contains: contains,
		}})
	}
	return out, nil
}

// firstOf is the first of keys present in m.
func firstOf(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

// StringList reads a string or a list of scalars as []string.
func StringList(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return []string{t}
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			out = append(out, fmt.Sprint(x))
		}
		return out
	}
	return []string{fmt.Sprint(v)}
}
