package config

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestStoryShorthand: `story: true` works in every config format.
func TestStoryShorthand(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"a.yaml", "story: true\n"},
		{"b.json", `{"story": true}`},
		{"c.toml", "story = true\n"},
	} {
		if cfg := loadPageTools(t, tt.name, tt.body); !cfg.Story.Enabled {
			t.Errorf("%s: shorthand not enabled", tt.name)
		}
	}
}

// TestStoryMapping: a mapping turns story on, keeps sections and rules for
// the build and passes every other key on untouched.
func TestStoryMapping(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"a.yaml", "story:\n  sections: [posts/recipes]\n  narrator: {rate: 1.1}\n  rules:\n    - when: {category: recipes, contains: Składniki}\n      set: {slideshow: {interval: 6s}}\n"},
		{"b.json", `{"story": {"sections": ["posts/recipes"], "narrator": {"rate": 1.1}, "rules": [{"when": {"category": "recipes", "contains": "Składniki"}, "set": {"slideshow": {"interval": "6s"}}}]}}`},
		{"c.toml", "[story]\nsections = [\"posts/recipes\"]\n[story.narrator]\nrate = 1.1\n[[story.rules]]\n[story.rules.when]\ncategory = \"recipes\"\ncontains = \"Składniki\"\n[story.rules.set.slideshow]\ninterval = \"6s\"\n"},
	} {
		s := loadPageTools(t, tt.name, tt.body).Story
		if !s.Enabled || !reflect.DeepEqual(s.Sections, []string{"posts/recipes"}) {
			t.Errorf("%s: enabled/sections = %+v", tt.name, s)
		}
		if n, _ := s.Settings["narrator"].(map[string]any); n["rate"] != 1.1 {
			t.Errorf("%s: settings = %#v", tt.name, s.Settings)
		}
		if len(s.Rules) != 1 || !reflect.DeepEqual(s.Rules[0].When.Category, []string{"recipes"}) ||
			s.Rules[0].When.Contains != "Składniki" || s.Rules[0].Set["slideshow"] == nil {
			t.Errorf("%s: rules = %+v", tt.name, s.Rules)
		}
	}
}

// TestStoryOff: `enabled: false` and null leave story off.
func TestStoryOff(t *testing.T) {
	if s := loadPageTools(t, "a.yaml", "story:\n  enabled: false\n  narrator: {rate: 1}\n").Story; s.Enabled {
		t.Error("enabled: false is on")
	}
	var s StoryConfig
	if err := s.fromRaw(nil); err != nil || s.Enabled {
		t.Errorf("null = %+v, %v", s, err)
	}
}

// TestStoryDecodeErrors: a value of the wrong shape is reported.
func TestStoryDecodeErrors(t *testing.T) {
	var s StoryConfig
	for name, raw := range map[string]any{
		"string":          "yes",
		"enabled":         map[string]any{"enabled": "yes"},
		"rules not list":  map[string]any{"rules": "x"},
		"rule not a map":  map[string]any{"rules": []any{"x"}},
		"toml not a bool": 3,
	} {
		if err := s.fromRaw(raw); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if err := s.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Error("broken JSON: no error")
	}
	if err := s.UnmarshalTOML(true); err != nil || !s.Enabled {
		t.Errorf("TOML true = %+v, %v", s, err)
	}
	var y StoryConfig
	if err := yaml.Unmarshal([]byte("story: [1, 2"), &struct {
		Story *StoryConfig `yaml:"story"`
	}{&y}); err == nil {
		t.Error("broken YAML: no error")
	}
	if err := yaml.Unmarshal([]byte("story: !!int abc"), &struct {
		Story *StoryConfig `yaml:"story"`
	}{&y}); err == nil {
		t.Error("a bad YAML tag: no error")
	}
	if err := yaml.Unmarshal([]byte("story: [1, 2]"), &struct {
		Story *StoryConfig `yaml:"story"`
	}{&y}); err == nil {
		t.Error("a YAML list: no error")
	}
}

// TestStoryRuleKeys: when accepts the singular and plural spellings, and a
// rule without when matches everything.
func TestStoryRuleKeys(t *testing.T) {
	rules, err := storyRules([]any{
		map[string]any{"when": map[string]any{"categories": []any{"a", "b"}, "tag": "x", "sections": "posts"}},
		map[string]any{"set": map[string]any{"k": 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := StoryWhen{Category: []string{"a", "b"}, Tags: []string{"x"}, Section: []string{"posts"}}
	if !reflect.DeepEqual(rules[0].When, want) || !reflect.DeepEqual(rules[1].When, StoryWhen{}) {
		t.Errorf("rules = %+v", rules)
	}
}

// TestStringList: a string, a list of scalars, or one other scalar.
func TestStringList(t *testing.T) {
	for _, tt := range []struct {
		in   any
		want []string
	}{
		{nil, nil},
		{"a", []string{"a"}},
		{[]string{"a"}, []string{"a"}},
		{[]any{"a", 2}, []string{"a", "2"}},
		{7, []string{"7"}},
	} {
		if got := StringList(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("StringList(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
