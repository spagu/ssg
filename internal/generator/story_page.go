package generator

// Story mode (1.9.4): which pages get the presentation, and the settings each
// one hands its script. A page's settings are, in order: the site's `story:`,
// every rule whose `when` matches it, the file <name>.story.yaml beside its
// source, and its own `story:` front matter. Mappings merge key by key; a few
// lists (slides, widgets, figure_skip, hide) add up instead of replacing, so
// a post can add a slide without restating the site's. `story: false` in
// front matter keeps a page out.

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/spagu/ssg/internal/models"
)

// StoryOptions configures story: (see config.StoryConfig).
type StoryOptions struct {
	Enabled  bool
	Sections []string
	Rules    []StoryRule
	Settings map[string]any // passed to the page script
}

// StoryRule adds Set to the pages When matches.
type StoryRule struct {
	When StoryWhen
	Set  map[string]any
}

// StoryWhen matches a page: every condition given must hold, any value of a
// list will do; empty matches every page.
type StoryWhen struct {
	Category []string
	Tags     []string
	Section  []string
	Contains string
}

// storyListsAdd are the lists that add up across layers (camelCase, after
// storyCamel).
var storyListsAdd = map[string]bool{"slides": true, "widgets": true, "figureSkip": true, "hide": true}

// storyVerbatim are mappings whose keys are data, not option names: they
// are never camelCased.
var storyVerbatim = map[string]bool{"pronounce": true, "labels": true, "theme": true}

// storySettings is the settings for a page's presentation, and false when the
// page has none.
func (g *Generator) storySettings(page models.Page, isPost bool) (map[string]any, bool) {
	sc := g.config.Story
	if !sc.Enabled || !g.inStorySections(page, isPost, sc.Sections) {
		return nil, false
	}
	own, hasOwn := page.Extra["story"]
	if off, ok := own.(bool); hasOwn && ok && !off {
		return nil, false
	}
	out := storyCamel(sc.Settings, "").(map[string]any)
	for _, r := range sc.Rules {
		if g.storyRuleMatches(r.When, page) {
			out = storyMerge(out, storyCamel(r.Set, ""), "").(map[string]any)
		}
	}
	if side := g.storySideFile(page); side != nil {
		out = storyMerge(out, storyCamel(side, ""), "").(map[string]any)
	}
	if m, ok := own.(map[string]any); ok {
		out = storyMerge(out, storyCamel(m, ""), "").(map[string]any)
	}
	return out, true
}

// inStorySections reports whether the page is in the configured sections
// (posts when none are named) — the same reading as web_stories.sections.
func (g *Generator) inStorySections(page models.Page, isPost bool, sections []string) bool {
	if len(sections) == 0 {
		return isPost
	}
	for _, s := range sections {
		key := strings.Trim(strings.ToLower(strings.TrimSpace(s)), "/")
		switch {
		case isPost && (key == "posts" || key == "post"),
			!isPost && (key == "pages" || key == "page"),
			g.inStorySection(page, key):
			return true
		}
	}
	return false
}

// storyRuleMatches applies one rule's `when` to a page.
func (g *Generator) storyRuleMatches(w StoryWhen, page models.Page) bool {
	if len(w.Category) > 0 && !anyFold(w.Category, g.storyCategories(page)) {
		return false
	}
	if len(w.Tags) > 0 && !anyFold(w.Tags, page.Tags) {
		return false
	}
	if len(w.Section) > 0 {
		in := false
		for _, s := range w.Section {
			in = in || g.inStorySection(page, strings.Trim(strings.ToLower(s), "/"))
		}
		if !in {
			return false
		}
	}
	return w.Contains == "" || strings.Contains(page.Content, w.Contains)
}

// storyCategories is every name and slug a page is filed under.
func (g *Generator) storyCategories(page models.Page) []string {
	var out []string
	for _, n := range append(g.categoryNames(&page), page.Taxonomies["categories"]...) {
		out = append(out, n, models.Slugify(n))
	}
	return out
}

// anyFold reports whether any of want is in have, ignoring case.
func anyFold(want, have []string) bool {
	for _, w := range want {
		for _, h := range have {
			if strings.EqualFold(strings.TrimSpace(w), strings.TrimSpace(h)) {
				return true
			}
		}
	}
	return false
}

// storySideFile reads <name>.story.yaml beside the page's source, with or
// without a top-level `story:` key; nil when there is none or it is broken
// (a broken file is reported, the page keeps the site's settings).
func (g *Generator) storySideFile(page models.Page) map[string]any {
	if page.SourceFile == "" {
		return nil
	}
	base := strings.TrimSuffix(page.SourceFile, filepath.Ext(page.SourceFile))
	for _, ext := range []string{".story.yaml", ".story.yml"} {
		file := filepath.Join(page.SourceDir, base+ext)
		data, err := os.ReadFile(file) // #nosec G304 -- beside a content file the site owns
		if err != nil {
			continue
		}
		var m map[string]any
		if err := yaml.Unmarshal(data, &m); err != nil {
			g.log("   ⚠️  story: " + file + ": " + err.Error())
			return nil
		}
		if inner, ok := m["story"].(map[string]any); ok {
			return inner
		}
		return m
	}
	return nil
}

// storyCamel copies a decoded YAML/TOML/JSON value with every mapping key in
// camelCase (snake_case is ssg's config style, camelCase the script's), but
// leaves the keys of storyVerbatim mappings alone: those are data.
func storyCamel(v any, parent string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			key := k
			if !storyVerbatim[parent] {
				key = snakeToCamel(k)
			}
			out[key] = storyCamel(x, key)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = storyCamel(x, "")
		}
		return out
	case []map[string]any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = storyCamel(x, "")
		}
		return out
	case nil:
		return map[string]any{}
	}
	return v
}

// snakeToCamel turns publisher_logo into publisherLogo; other keys pass.
func snakeToCamel(k string) string {
	if !strings.Contains(k, "_") {
		return k
	}
	parts := strings.Split(k, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// storyMerge lays b over a: mappings merge, the storyListsAdd lists append,
// anything else in b replaces a.
func storyMerge(a, b any, key string) any {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if aok && bok {
		out := make(map[string]any, len(am)+len(bm))
		for k, v := range am {
			out[k] = v
		}
		for k, v := range bm {
			if old, ok := out[k]; ok {
				out[k] = storyMerge(old, v, k)
			} else {
				out[k] = v
			}
		}
		return out
	}
	al, aok := a.([]any)
	bl, bok := b.([]any)
	if aok && bok && storyListsAdd[key] {
		return append(append([]any{}, al...), bl...)
	}
	return b
}
