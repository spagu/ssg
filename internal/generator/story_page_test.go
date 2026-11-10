package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestStoryBuild: a post selected by story: gets its settings as JSON —
// site, matching rule, side file and front matter merged, keys camelCased,
// the built-in pronunciation set and its Web Story link added — plus the
// styles and scripts, written once under /assets/ssg-story/.
func TestStoryBuild(t *testing.T) {
	cfg, _ := buildStorySite(t, func(c *Config) {
		site := filepath.Join(c.ContentDir, c.Source)
		side := "story:\n  slides:\n    - {image: /media/cake.png, at: \"text:Whisk\"}\n  narrator: {pronounce: {\"np.\": null}}\n"
		if err := os.WriteFile(filepath.Join(site, "posts", "recipes", "cake.story.yaml"), []byte(side), 0o600); err != nil {
			t.Fatal(err)
		}
		c.Story = StoryOptions{
			Enabled: true,
			Settings: map[string]any{
				"slides":    []any{map[string]any{"image": "/media/sliced.png"}},
				"narrator":  map[string]any{"duck": 0.3, "pronounce": map[string]any{"{num} tin|tins": "{num} baking tin|tins"}},
				"slideshow": map[string]any{"return_to_text": true},
				"labels":    map[string]any{"left_min": "{n} min"},
			},
			Rules: []StoryRule{
				{When: StoryWhen{Section: []string{"posts/recipes"}}, Set: map[string]any{"progress": map[string]any{"words_per_minute": 200}}},
				{When: StoryWhen{Tags: []string{"nope"}}, Set: map[string]any{"never": true}},
			},
		}
	})
	post := mustRead(t, filepath.Join(cfg.OutputDir, "chocolate-cake", "index.html"))
	settings := storyJSON(t, post)
	slides, _ := settings["slides"].([]any)
	if len(slides) != 2 {
		t.Errorf("slides add up across layers: %v", settings["slides"])
	}
	if sh, _ := settings["slideshow"].(map[string]any); sh["returnToText"] != true {
		t.Errorf("snake_case keys become camelCase: %v", settings["slideshow"])
	}
	if p, _ := settings["progress"].(map[string]any); p["wordsPerMinute"] != float64(200) {
		t.Errorf("the matching rule applies: %v", settings["progress"])
	}
	if settings["never"] != nil {
		t.Error("a rule that does not match applied")
	}
	if l, _ := settings["labels"].(map[string]any); l["left_min"] == nil {
		t.Errorf("labels keep their keys: %v", settings["labels"])
	}
	n, _ := settings["narrator"].(map[string]any)
	rules, _ := n["pronounce"].(map[string]any)
	if rules["{num} g"] == nil || rules["{num} tin|tins"] == nil || rules["e.g."] == nil {
		t.Errorf("built-in set plus the site's rules: %d rules", len(rules))
	}
	if share, _ := settings["share"].(map[string]any); share["webStory"] != "/web-stories/chocolate-cake/" {
		t.Errorf("the post links its Web Story: %v", settings["share"])
	}
	if settings["siteHost"] != "example.com" {
		t.Errorf("the script knows the site's own host: %v", settings["siteHost"])
	}
	for _, f := range append(append([]string{}, storyScripts...), storyStyles...) {
		if !strings.Contains(post, "/assets/ssg-story/"+f+"?v="+storyAssetVersion) {
			t.Errorf("page does not load %s", f)
		}
		if _, err := os.Stat(filepath.Join(cfg.OutputDir, "assets", "ssg-story", f)); err != nil {
			t.Errorf("%s not written: %v", f, err)
		}
	}
	if strings.Index(post, "story.css") > strings.Index(post, "</head>") {
		t.Error("styles belong in <head>")
	}
	about := mustRead(t, filepath.Join(cfg.OutputDir, "about", "index.html"))
	if strings.Contains(about, `id="ssg-story"`) {
		t.Error("pages are not in the default sections")
	}
	story := mustRead(t, filepath.Join(cfg.OutputDir, "web-stories", "chocolate-cake", "index.html"))
	if strings.Contains(story, `id="ssg-story"`) {
		t.Error("the AMP story must not get the presentation")
	}
}

// TestStoryBuildOff: without story: nothing is added and nothing written.
func TestStoryBuildOff(t *testing.T) {
	cfg, _ := buildStorySite(t, nil)
	if post := mustRead(t, filepath.Join(cfg.OutputDir, "chocolate-cake", "index.html")); strings.Contains(post, `id="ssg-story"`) {
		t.Error("story is off")
	}
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "assets", "ssg-story")); err == nil {
		t.Error("assets written for a site without story")
	}
}

// storyJSON is the settings a page carries.
func storyJSON(t *testing.T, page string) map[string]any {
	t.Helper()
	m := regexp.MustCompile(`<script type="application/json" id="ssg-story">(.*?)</script>`).FindStringSubmatch(page)
	if m == nil {
		t.Fatal("page has no story settings")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(strings.ReplaceAll(m[1], `<\/`, "</")), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestStorySettingsSelection: sections, `story: false` and rule conditions.
func TestStorySettingsSelection(t *testing.T) {
	g := &Generator{config: Config{Story: StoryOptions{Enabled: true}}, siteData: &models.SiteData{}}
	post := models.Page{Title: "x", Content: "Składniki: mąka", Tags: []string{"Cake"}, Category: "Desserts"}
	if _, ok := g.storySettings(post, true); !ok {
		t.Error("posts are the default section")
	}
	if _, ok := g.storySettings(post, false); ok {
		t.Error("a page is not a post")
	}
	off := post
	off.Extra = map[string]any{"story": false}
	if _, ok := g.storySettings(off, true); ok {
		t.Error("story: false keeps a post out")
	}
	own := post
	own.Extra = map[string]any{"story": map[string]any{"auto_play": false}}
	if s, ok := g.storySettings(own, true); !ok || s["autoPlay"] != false {
		t.Errorf("front matter settings = %v", s)
	}
	g.config.Story.Sections = []string{"pages", "posts/recipes"}
	if _, ok := g.storySettings(post, false); !ok {
		t.Error("sections: pages")
	}
	if _, ok := g.storySettings(post, true); ok {
		t.Error("a post outside posts/recipes")
	}
	for _, tt := range []struct {
		w    StoryWhen
		want bool
	}{
		{StoryWhen{}, true},
		{StoryWhen{Category: []string{"desserts"}}, true},
		{StoryWhen{Category: []string{"mains"}}, false},
		{StoryWhen{Tags: []string{"cake"}}, true},
		{StoryWhen{Tags: []string{"bread"}}, false},
		{StoryWhen{Section: []string{"posts"}}, false},
		{StoryWhen{Contains: "Składniki"}, true},
		{StoryWhen{Contains: "Ingredients"}, false},
	} {
		if got := g.storyRuleMatches(tt.w, post); got != tt.want {
			t.Errorf("%+v = %v, want %v", tt.w, got, tt.want)
		}
	}
	g.config.Story.Enabled = false
	if _, ok := g.storySettings(post, true); ok {
		t.Error("story off")
	}
}

// TestStorySideFile: with or without a top-level story:, .yml too; a broken
// file is reported and ignored.
func TestStorySideFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.story.yaml", "story: {k: 1}\n")
	write("b.story.yml", "k: 2\n")
	write("c.story.yaml", "k: [1\n")
	g := &Generator{config: Config{Quiet: true}}
	for name, want := range map[string]any{"a.md": 1, "b.md": 2, "c.md": nil, "d.md": nil} {
		got := g.storySideFile(models.Page{SourceDir: dir, SourceFile: name})
		if want == nil && got != nil || want != nil && got["k"] != want {
			t.Errorf("%s = %v", name, got)
		}
	}
	if g.storySideFile(models.Page{}) != nil {
		t.Error("a page without a source file has no side file")
	}
}

// TestStoryCamelMerge: key conversion and the merge rules.
func TestStoryCamelMerge(t *testing.T) {
	in := map[string]any{
		"a_b_c":     []any{map[string]any{"x_y": 1}},
		"tables":    []map[string]any{{"p_q": 1}},
		"pronounce": map[string]any{"{num} g_x": "y"},
		"empty":     nil,
		"_lead":     1,
	}
	got := storyCamel(in, "").(map[string]any)
	want := map[string]any{
		"aBC":       []any{map[string]any{"xY": 1}},
		"tables":    []any{map[string]any{"pQ": 1}},
		"pronounce": map[string]any{"{num} g_x": "y"},
		"empty":     map[string]any{},
		"Lead":      1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("storyCamel = %#v", got)
	}
	a := map[string]any{"slides": []any{1}, "other": []any{1}, "m": map[string]any{"x": 1, "y": 1}}
	b := map[string]any{"slides": []any{2}, "other": []any{2}, "m": map[string]any{"y": 2}, "n": 3}
	merged := storyMerge(a, b, "").(map[string]any)
	if !reflect.DeepEqual(merged, map[string]any{"slides": []any{1, 2}, "other": []any{2},
		"m": map[string]any{"x": 1, "y": 2}, "n": 3}) {
		t.Errorf("storyMerge = %#v", merged)
	}
}

// TestStoryHTMLStringEdges: pages the presentation leaves alone, a page
// without <head>, settings that cannot be JSON.
func TestStoryHTMLStringEdges(t *testing.T) {
	g := &Generator{config: Config{Quiet: true, Story: StoryOptions{Enabled: true}}, siteData: &models.SiteData{}}
	post := &models.Page{Title: "x", Content: "text"}
	page := "<html><head></head><body>x</body></html>"
	for name, tt := range map[string]struct {
		s    string
		page *models.Page
	}{
		"no page":      {page, nil},
		"empty":        {page, &models.Page{}},
		"amp":          {`<html amp lang="en"><head></head><body></body></html>`, post},
		"already done": {`<script type="application/json" id="ssg-story">{}</script>`, post},
	} {
		if got := g.storyHTMLString(tt.s, tt.page, true); got != tt.s {
			t.Errorf("%s: changed", name)
		}
	}
	if got := g.storyHTMLString("<p>x</p>", post, true); !strings.HasPrefix(got, `<link rel="stylesheet"`) ||
		!strings.Contains(got, `id="ssg-story"`) {
		t.Errorf("a fragment gets styles first and scripts last: %s", got)
	}
	g.config.Story.Settings = map[string]any{"bad": make(chan int)}
	if got := g.storyHTMLString(page, post, true); got != page {
		t.Error("settings that are not JSON leave the page alone")
	}
}

// TestStoryPronounceSets: the built-in set by language, the site's rules
// over it, null removing one, pronounce_defaults: false removing all.
func TestStoryPronounceSets(t *testing.T) {
	for lang, want := range map[string]string{"pl-PL": "{num} łyż.", "pl": "{num} łyż.", "en": "{num} tbsp", "EN_gb": "{num} tbsp"} {
		if builtinPronounce(lang)[want] == "" {
			t.Errorf("%s: no %q", lang, want)
		}
	}
	if builtinPronounce("xx") != nil || builtinPronounce("") != nil {
		t.Error("no set for an unknown language")
	}
	rules := storyPronounce(map[string]any{"pronounce": map[string]any{"np.": nil, "x": "y"}}, "pl")
	if rules["np."] != nil || rules["x"] != "y" || rules["{num} g"] == nil {
		t.Errorf("rules = %d", len(rules))
	}
	if rules := storyPronounce(map[string]any{"pronounceDefaults": false, "pronounce": map[string]any{"x": "y"}}, "pl"); len(rules) != 1 {
		t.Errorf("defaults off = %v", rules)
	}
	g := &Generator{siteData: &models.SiteData{}}
	settings := map[string]any{"narrator": map[string]any{"pronounceDefaults": true}}
	g.storyFinish(settings, models.Page{Lang: "xx"})
	if n := settings["narrator"].(map[string]any); len(n) != 0 || settings["share"] != nil {
		t.Errorf("nothing to add = %v", settings)
	}
	settings = map[string]any{}
	g.webStories.byPost = map[string]string{"/x/": "/web-stories/x/"}
	g.storyFinish(settings, models.Page{Slug: "x", Lang: "en"})
	if settings["narrator"].(map[string]any)["pronounce"] == nil {
		t.Error("a narrator section is made for the built-in set")
	}
}

// TestWriteStoryAssetsError: an output that cannot hold the directory fails
// the build loudly.
func TestWriteStoryAssetsError(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	if err := os.WriteFile(out, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &Generator{config: Config{OutputDir: out}}
	g.storyUsed.Store(true)
	if err := g.writeStoryAssets(); err == nil {
		t.Error("no error")
	}
	g.config.OutputDir = t.TempDir()
	if err := os.MkdirAll(filepath.Join(g.config.OutputDir, "assets", "ssg-story", "story.js"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := g.writeStoryAssets(); err == nil {
		t.Error("no error writing over a directory")
	}
}

// TestPendingImageExists: a link to x.webp resolves when x.jpg is in the
// output and the post-build pass writes WebP; never without that pass (#405).
func TestPendingImageExists(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "a.jpg"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &Generator{config: Config{OutputDir: out}}
	if g.refResolves("/a.webp", out) {
		t.Error("no pending pass, no .webp")
	}
	g = &Generator{config: Config{OutputDir: out, PendingImageFormats: []string{"webp"}}}
	if !g.refResolves("/a.webp", out) {
		t.Error("a.webp comes from a.jpg")
	}
	if g.refResolves("/a.avif", out) || g.refResolves("/b.webp", out) || g.refResolves("/a.gif", out) {
		t.Error("only the pending formats, and only from a source that exists")
	}
}
