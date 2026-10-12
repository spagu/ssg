package generator

// Web stories through a real build: the story, its images, the hub, the
// sitemap, the slot in the post, and what stays out of feeds and search.

import (
	"image"
	_ "image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// storyCakePost is a recipe post with a featured image, two photographs in
// the body (one beside the post, one in media/) and a method.
const storyCakePost = `---
title: Chocolate Cake
slug: chocolate-cake
status: publish
type: post
date: 2026-03-01
featured_image: /media/cake.png
tags: [chocolate, cake, baking, dessert]
excerpt: A dark, damp cake for a Sunday.
schema:
  "@type": Recipe
  totalTime: PT1H25M
  recipeYield: "16"
---

![Batter](batter.png)

## Method

Whisk the eggs with the sugar until pale.

<img src="/media/sliced-300x200.png" alt="Sliced">

<span data-ssg-story-slot></span>
`

// storyPlainPost has no featured image, so no story.
const storyPlainPost = `---
title: Plain Note
slug: plain-note
status: publish
type: post
date: 2026-02-01
---

Nothing to see. <span data-ssg-story-slot>Story</span>
`

// buildStorySite builds the fixture site with web stories on.
func buildStorySite(t *testing.T, apply func(*Config)) (Config, string) {
	t.Helper()
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"posts/recipes/cake.md": storyCakePost,
		"posts/notes/plain.md":  storyPlainPost,
		"pages/about.md":        "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nAbout.\n",
	}, func(name string) string {
		if name == "post.html" || name == "page.html" {
			return `<html><head><title>{{.Title}}</title></head><body>{{.Content}}</body></html>`
		}
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	})
	site := filepath.Join(cfg.ContentDir, cfg.Source)
	writeTestPNG(t, filepath.Join(site, "media", "cake.png"), 400, 300)
	writeTestPNG(t, filepath.Join(site, "media", "sliced.png"), 300, 600)
	writeTestPNG(t, filepath.Join(site, "posts", "recipes", "batter.png"), 120, 90)
	writeTestPNG(t, filepath.Join(site, "media", "logo.png"), 64, 64)
	cfg.PostURLFormat = "slug"
	cfg.Feed = true
	cfg.SearchIndex = true
	cfg.WebStories = WebStoriesOptions{Enabled: true, PublisherLogo: "media/logo.png"}
	if apply != nil {
		apply(&cfg)
	}
	out := captureGeneratorStdout(t, func() { buildSiteFixture(t, cfg) })
	return cfg, out
}

// TestWebStoryBuild: a recipe post becomes a valid-shaped amp-story, cut
// from its own pictures, captioned with hooks and never with its method.
func TestWebStoryBuild(t *testing.T) {
	cfg, _ := buildStorySite(t, nil)
	story := mustRead(t, filepath.Join(cfg.OutputDir, "web-stories", "chocolate-cake", "index.html"))
	assertAMPStory(t, story)
	for _, want := range []string{
		`<link rel="canonical" href="https://example.com/web-stories/chocolate-cake/">`,
		"chocolate · cake · baking", "Ready in 1 hr 25 min", "Makes 16 servings",
		"A dark, damp cake for a Sunday.", `href="https://example.com/chocolate-cake/"`,
		"Read the full article", `<amp-story-page id="page-4">`,
	} {
		if !strings.Contains(story, want) {
			t.Errorf("story lacks %q", want)
		}
	}
	if strings.Contains(story, "Whisk") || strings.Contains(story, "dessert") {
		t.Error("a story must not carry the method, nor more than three tags")
	}
	sizes := map[string][2]int{}
	for _, m := range regexp.MustCompile(`(?:src|poster-portrait-src|publisher-logo-src)="https://example.com(/processed_images/[^"]+)"`).FindAllStringSubmatch(story, -1) {
		sizes[m[1]] = imageSize(t, filepath.Join(cfg.OutputDir, filepath.FromSlash(m[1])))
	}
	want := map[[2]int]bool{{640, 853}: false, {720, 1280}: false, {192, 192}: false}
	for _, s := range sizes {
		if _, ok := want[s]; !ok {
			t.Errorf("unexpected derived size %v", s)
		}
		want[s] = true
	}
	for s, seen := range want {
		if !seen {
			t.Errorf("no image of %v was derived", s)
		}
	}
	if len(sizes) != 5 { // logo, poster, cover (reused by page 3), two photographs
		t.Errorf("derived images = %d, want 5: %v", len(sizes), sizes)
	}
}

// TestWebStoryHubSitemapSlot: the hub lists the story, both reach the
// sitemap, the post links its story, and nothing leaks into feeds or search.
func TestWebStoryHubSitemapSlot(t *testing.T) {
	cfg, log := buildStorySite(t, func(c *Config) { c.Quiet = false })
	hub := mustRead(t, filepath.Join(cfg.OutputDir, "web-stories", "index.html"))
	if !strings.Contains(hub, `<a href="/web-stories/chocolate-cake/">`) || !strings.Contains(hub, `width="640" height="853"`) {
		t.Errorf("hub = %s", hub)
	}
	sitemap := mustRead(t, filepath.Join(cfg.OutputDir, "sitemap.xml"))
	for _, loc := range []string{"https://example.com/web-stories/", "https://example.com/web-stories/chocolate-cake/"} {
		if !strings.Contains(sitemap, "<loc>"+loc+"</loc>") {
			t.Errorf("sitemap lacks %s", loc)
		}
	}
	post := mustRead(t, filepath.Join(cfg.OutputDir, "chocolate-cake", "index.html"))
	if !strings.Contains(post, `<span data-ssg-story-slot><a class="ssg-story-link" href="/web-stories/chocolate-cake/">Watch as a Web Story</a></span>`) {
		t.Errorf("slot not filled:\n%s", post)
	}
	plain := mustRead(t, filepath.Join(cfg.OutputDir, "plain-note", "index.html"))
	if strings.Contains(plain, "data-ssg-story-slot") || strings.Contains(plain, ">Story<") {
		t.Errorf("slot not removed:\n%s", plain)
	}
	for _, f := range []string{"feed.xml", "search-index.json"} {
		if strings.Contains(mustRead(t, filepath.Join(cfg.OutputDir, f)), "/web-stories/chocolate-cake/") {
			t.Errorf("%s lists the story", f)
		}
	}
	if !strings.Contains(log, "1 skipped, no featured image: Plain Note") || !strings.Contains(log, "web stories: 1 made, 1 hub page(s)") {
		t.Errorf("build log:\n%s", log)
	}
}

// TestWebStoryOffAndNoLogo: off, nothing is made; on without a logo, the
// build says why and makes nothing.
func TestWebStoryOffAndNoLogo(t *testing.T) {
	cfg, _ := buildStorySite(t, func(c *Config) { c.WebStories.Enabled = false })
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "web-stories")); err == nil {
		t.Error("stories were made with the feature off")
	}
	cfg, log := buildStorySite(t, func(c *Config) { c.WebStories.PublisherLogo = ""; c.Quiet = false })
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "web-stories")); err == nil {
		t.Error("stories were made without a publisher logo")
	}
	if !strings.Contains(log, "publisher_logo is not set") {
		t.Errorf("no warning:\n%s", log)
	}
}

// imageSize decodes a derived image's dimensions.
func imageSize(t *testing.T, path string) [2]int {
	t.Helper()
	f, err := os.Open(path) // #nosec G304 -- test output
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	c, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return [2]int{c.Width, c.Height}
}

// TestWebStoryAMPValidator runs the official AMP validator over a story when
// one is available: amphtml-validator on PATH, or the command named by
// SSG_AMP_VALIDATOR (e.g. "npx --yes amphtml-validator"). The validator
// fetches its rules from cdn.ampproject.org, so it is opt-in, never a CI
// dependency.
func TestWebStoryAMPValidator(t *testing.T) {
	command := strings.Fields(os.Getenv("SSG_AMP_VALIDATOR"))
	if len(command) == 0 {
		if path, err := exec.LookPath("amphtml-validator"); err == nil {
			command = []string{path}
		}
	}
	if len(command) == 0 {
		t.Skip("no AMP validator: install amphtml-validator or set SSG_AMP_VALIDATOR")
	}
	cfg, _ := buildStorySite(t, nil)
	file := filepath.Join(cfg.OutputDir, "web-stories", "chocolate-cake", "index.html")
	// #nosec G204 -- test-only, the developer's own validator command
	out, err := exec.Command(command[0], append(command[1:], file)...).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "PASS") {
		t.Fatalf("AMP validation failed: %v\n%s", err, out)
	}
}
