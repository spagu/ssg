package main

// Wiring for web stories (1.8.72) and story mode (1.9.4): `web_stories:` and
// `story:` become the generator's options. Stories cut WebP images when the site publishes WebP, since the
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
		Pages: w.Pages, MinImages: w.MinImages, CTA: w.CTA, Narration: w.Narration,
		WebP: cfg.WebP || wantsFormat(cfg, "webp"),
	}
}

// buildStoryOptions turns `story:` (1.9.4) into generator options.
func buildStoryOptions(s config.StoryConfig) generator.StoryOptions {
	rules := make([]generator.StoryRule, 0, len(s.Rules))
	for _, r := range s.Rules {
		rules = append(rules, generator.StoryRule{Set: r.Set, When: generator.StoryWhen(r.When)})
	}
	return generator.StoryOptions{Enabled: s.Enabled, Sections: s.Sections, Rules: rules, Settings: s.Settings}
}
