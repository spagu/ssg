package main

import (
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestBuildWebStoriesOptions: every key reaches the generator, and stories
// cut WebP exactly when the site publishes WebP.
func TestBuildWebStoriesOptions(t *testing.T) {
	cfg := &config.Config{WebStories: config.WebStoriesConfig{Enabled: true, Sections: []string{"pages"}, URL: "/s/",
		PublisherLogo: "logo.png", Pages: 3, MinImages: 1, CTA: "Go"}}
	o := buildWebStoriesOptions(cfg)
	if !o.Enabled || o.URL != "/s/" || o.PublisherLogo != "logo.png" || o.Pages != 3 || o.MinImages != 1 || o.CTA != "Go" ||
		len(o.Sections) != 1 || o.WebP {
		t.Errorf("options = %+v", o)
	}
	cfg.ImageFormats = []string{"avif", "webp"}
	if !buildWebStoriesOptions(cfg).WebP {
		t.Error("image_formats with webp means WebP stories")
	}
	cfg.ImageFormats, cfg.WebP = nil, true
	if !buildWebStoriesOptions(cfg).WebP {
		t.Error("webp: true means WebP stories")
	}
}

// TestBuildStoryOptions: story: reaches the generator with its rules.
func TestBuildStoryOptions(t *testing.T) {
	o := buildStoryOptions(config.StoryConfig{
		Enabled: true, Sections: []string{"posts"},
		Rules:    []config.StoryRule{{When: config.StoryWhen{Tags: []string{"x"}}, Set: map[string]any{"a": 1}}},
		Settings: map[string]any{"narrator": map[string]any{}},
	})
	if !o.Enabled || len(o.Sections) != 1 || len(o.Rules) != 1 || o.Rules[0].When.Tags[0] != "x" ||
		o.Rules[0].Set["a"] != 1 || o.Settings["narrator"] == nil {
		t.Errorf("options = %+v", o)
	}
	if o := buildWebStoriesOptions(&config.Config{WebStories: config.WebStoriesConfig{Narration: true, BackgroundAudio: "a.mp3"}}); !o.Narration || o.BackgroundAudio != "a.mp3" {
		t.Error("narration is passed on")
	}
}

// TestPendingImageFormats: what the post-build pass will write.
func TestPendingImageFormats(t *testing.T) {
	if got := pendingImageFormats(&config.Config{}); len(got) != 0 {
		t.Errorf("none = %v", got)
	}
	if got := pendingImageFormats(&config.Config{WebP: true, ImageFormats: []string{"AVIF"}}); len(got) != 2 || got[0] != "webp" || got[1] != "avif" {
		t.Errorf("webp + avif = %v", got)
	}
	if got := pendingImageFormats(&config.Config{ImageFormats: []string{"webp"}}); len(got) != 1 {
		t.Errorf("image_formats webp = %v", got)
	}
}
