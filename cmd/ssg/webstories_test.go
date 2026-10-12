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
