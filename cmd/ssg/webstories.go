package main

// Wiring for web stories (1.8.72): `web_stories:` becomes the generator's
// options. Stories cut WebP images when the site publishes WebP, since the
// WebP pass does not rewrite AMP's poster and logo attributes.

import (
	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// buildWebStoriesOptions turns `web_stories:` into generator options.
func buildWebStoriesOptions(cfg *config.Config) generator.WebStoriesOptions {
	w := cfg.WebStories
	return generator.WebStoriesOptions{
		Enabled: w.Enabled, Sections: w.Sections, URL: w.URL, PublisherLogo: w.PublisherLogo,
		Pages: w.Pages, MinImages: w.MinImages, CTA: w.CTA,
		WebP: cfg.WebP || wantsFormat(cfg, "webp"),
	}
}
