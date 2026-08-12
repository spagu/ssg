package main

// Tests for the config→generator converters and flag parsers (second
// project-wide coverage raise, 1.8.27). These converters are the seam every new
// config feature crosses — a dropped field here ships as a silently dead
// setting, which is exactly what a table test catches.

import (
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func TestBuildAIClient(t *testing.T) {
	// No models → nil (feature off).
	if c := buildAIClient(config.AIConfig{}); c != nil {
		t.Fatal("no models must yield a nil client")
	}
	cfg := config.AIConfig{
		Models: map[string]config.AIModel{
			"fast": {URL: "http://x", Key: "k", Model: "m", System: "s", MaxTok: 9, Temp: 0.7},
		},
		Agents: map[string]config.AIAgent{
			"writer": {Model: "fast", System: "p", Rules: []string{"r"}, Skills: []string{"s"}, MaxTok: 5, Temp: 0.2},
		},
		DefaultModel: "fast",
		Timeout:      "5s",
	}
	c := buildAIClient(cfg)
	if c == nil || !c.Enabled() {
		t.Fatal("configured models must yield an enabled client")
	}
	// Bad duration falls back to the client default (constructor handles 0).
	cfg.Timeout = "bogus"
	if c := buildAIClient(cfg); c == nil {
		t.Fatal("bad timeout must not kill the client")
	}
}

func TestBuildNotifier(t *testing.T) {
	cfg := &config.Config{}
	if n := buildNotifier(cfg); n != nil {
		t.Fatal("notify off → nil")
	}
	cfg.Notify = true
	if n := buildNotifier(cfg); n != nil {
		t.Fatal("no destinations → nil (never announce by accident)")
	}
	cfg.Notifications = []config.NotifyDest{{Name: "hook", URL: "http://x", Method: "POST"}}
	if n := buildNotifier(cfg); n == nil {
		t.Fatal("configured destination → notifier")
	}
}

func TestRedirectsOf(t *testing.T) {
	cfg := &config.Config{}
	if out := redirectsOf(cfg); out != nil {
		t.Fatal("no redirects → nil")
	}
	cfg.Redirects = []config.RedirectRule{{From: "/a", To: "/b", Status: 301, Force: true}}
	out := redirectsOf(cfg)
	if len(out) != 1 || out[0].From != "/a" || out[0].To != "/b" || out[0].Status != 301 || !out[0].Force {
		t.Fatalf("redirectsOf = %+v", out)
	}
}

func TestWorkersOf(t *testing.T) {
	cfg := &config.Config{}
	if out := workersOf(cfg); out != nil {
		t.Fatal("no workers → nil")
	}
	cfg.Workers = []config.WorkerConfig{{
		Name: "comments", Dir: "workers/comments", Mode: "functions",
		RoutesInclude: []string{"/api/*"},
	}}
	out := workersOf(cfg)
	if len(out) != 1 || out[0].Name != "comments" || out[0].Dir != "workers/comments" ||
		out[0].Mode != "functions" || out[0].RoutesInclude[0] != "/api/*" {
		t.Fatalf("workersOf = %+v", out)
	}
}

func TestContentSourcesOfAndRobotsRulesOf(t *testing.T) {
	cfg := &config.Config{}
	if contentSourcesOf(cfg) != nil || robotsRulesOf(cfg) != nil {
		t.Fatal("empty config → nils")
	}
	cfg.ContentSources = []config.ContentSource{{Path: "docs", Type: "page", Category: "Docs"}}
	cs := contentSourcesOf(cfg)
	if len(cs) != 1 || cs[0].Path != "docs" || cs[0].Type != "page" || cs[0].Category != "Docs" {
		t.Fatalf("contentSourcesOf = %+v", cs)
	}
	cfg.RobotsRules = []config.RobotsRule{{UserAgent: "GPTBot", Allow: []string{"/"}, Disallow: []string{"/x"}, CrawlDelay: 3}}
	rr := robotsRulesOf(cfg)
	if len(rr) != 1 || rr[0].UserAgent != "GPTBot" || rr[0].Allow[0] != "/" ||
		rr[0].Disallow[0] != "/x" || rr[0].CrawlDelay != 3 {
		t.Fatalf("robotsRulesOf = %+v", rr)
	}
}

// TestParseMiscEqualFlags drives the --flag=value dispatcher: valid values land
// in config, invalid enum values are ignored (never a crash, never a wrong mode).
func TestParseMiscEqualFlags(t *testing.T) {
	cfg := &config.Config{}
	for _, arg := range []string{
		"--image-sizes=480,960",
		"--permalink-post=/:year/:slug/",
		"--permalink-page=/:slug/",
		"--check-links=strict",
		"--check-images=strict-decorative",
		"--check-meta=warn",
		"--check-schema=strict",
		"--check-orphans=warn",
		"--check-redirects=strict",
		"--content-source=docs",
		"--content-source=blog",
		"--shortcode-errors=keep",
		"--outputs=html,json",
		"--languages=pl,en",
		"--page-format=flat",
	} {
		parseMiscEqualFlags(arg, cfg)
	}
	if len(cfg.ImageSizes) != 2 || cfg.ImageSizes[0] != 480 {
		t.Errorf("image-sizes = %v", cfg.ImageSizes)
	}
	if cfg.Permalinks["post"] != "/:year/:slug/" || cfg.Permalinks["page"] != "/:slug/" {
		t.Errorf("permalinks = %v", cfg.Permalinks)
	}
	if cfg.CheckLinks != "strict" || cfg.CheckImages != "strict-decorative" ||
		cfg.CheckMeta != "warn" || cfg.CheckSchema != "strict" ||
		cfg.CheckOrphans != "warn" || cfg.CheckRedirects != "strict" {
		t.Errorf("check modes = %+v", cfg)
	}
	if len(cfg.ContentSources) != 2 || cfg.ContentSources[1].Path != "blog" {
		t.Errorf("content sources = %v", cfg.ContentSources)
	}
	if cfg.ShortcodeErrors != "keep" || cfg.PageFormat != "flat" {
		t.Errorf("shortcode/page-format = %q %q", cfg.ShortcodeErrors, cfg.PageFormat)
	}
	if len(cfg.Outputs) != 2 || len(cfg.Languages) != 2 {
		t.Errorf("outputs/languages = %v %v", cfg.Outputs, cfg.Languages)
	}

}

// TestParseMiscEqualFlagsRejectsInvalid: invalid enum values are dropped and an
// empty content-source is ignored — never a crash, never a wrong mode.
func TestParseMiscEqualFlagsRejectsInvalid(t *testing.T) {
	bad := &config.Config{}
	for _, arg := range []string{
		"--check-links=loud", "--check-images=nope", "--page-format=circle",
		"--shortcode-errors=explode", "--content-source= ",
	} {
		parseMiscEqualFlags(arg, bad)
	}
	if bad.CheckLinks != "" || bad.CheckImages != "" || bad.PageFormat != "" ||
		bad.ShortcodeErrors != "" || len(bad.ContentSources) != 0 {
		t.Errorf("invalid values leaked: %+v", bad)
	}
}

// TestParseMiscEqualFlagsRunnerSelection: naming a runner's dir/config selects
// that runner (GO-054).
func TestParseMiscEqualFlagsRunnerSelection(t *testing.T) {
	w := &config.Config{}
	parseMiscEqualFlags("--wrangler-dir=apps/api", w)
	if w.WatchRunner != "wrangler" || w.WatchRunnerDir != "apps/api" {
		t.Errorf("wrangler-dir = %+v", w)
	}
	w2 := &config.Config{}
	parseMiscEqualFlags("--workerd-config=w.capnp", w2)
	if w2.WatchRunner != "workerd" || w2.WatchRunnerConfig != "w.capnp" {
		t.Errorf("workerd-config = %+v", w2)
	}
}

// TestParseBoolFlagsTable drives every boolean CLI toggle through the parser —
// the table IS the contract between --flags and config fields.
func TestParseBoolFlagsTable(t *testing.T) {
	cases := []struct {
		flag  string
		check func(*config.Config) bool
	}{
		{"--watch", func(c *config.Config) bool { return c.Watch }},
		{"--http", func(c *config.Config) bool { return c.HTTP }},
		{"--clean", func(c *config.Config) bool { return c.Clean }},
		{"--zip", func(c *config.Config) bool { return c.Zip }},
		{"--webp", func(c *config.Config) bool { return c.WebP }},
		{"--minify-all", func(c *config.Config) bool { return c.MinifyAll }},
		{"--sitemap-off", func(c *config.Config) bool { return c.SitemapOff }},
		{"--robots-off", func(c *config.Config) bool { return c.RobotsOff }},
		{"--feed", func(c *config.Config) bool { return c.Feed }},
		{"--seo", func(c *config.Config) bool { return c.SEO }},
		{"--toc", func(c *config.Config) bool { return c.TOC }},
		{"--math", func(c *config.Config) bool { return c.Math }},
		{"--highlight", func(c *config.Config) bool { return c.Highlight }},
		{"--search-index", func(c *config.Config) bool { return c.SearchIndex }},
		{"--fingerprint", func(c *config.Config) bool { return c.Fingerprint }},
		{"--images-gc", func(c *config.Config) bool { return c.ImagesGC }},
	}
	for _, tc := range cases {
		cfg := &config.Config{}
		if !parseBoolFlags(tc.flag, cfg) {
			t.Errorf("%s not handled", tc.flag)
			continue
		}
		if !tc.check(cfg) {
			t.Errorf("%s did not set its field", tc.flag)
		}
	}
	// Special shapes.
	cfg := &config.Config{}
	if !parseBoolFlags("--check-links", cfg) || cfg.CheckLinks != "warn" {
		t.Error("--check-links should mean warn")
	}
	cfg = &config.Config{}
	if !parseBoolFlags("--no-auto-reload", cfg) || cfg.AutoReload == nil || *cfg.AutoReload {
		t.Error("--no-auto-reload should pin false")
	}
	if parseBoolFlags("--definitely-not-a-flag", &config.Config{}) {
		t.Error("unknown flag must not be claimed")
	}
}
