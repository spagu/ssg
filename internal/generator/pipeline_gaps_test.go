package generator

// Error propagation through Generate() and assetPhase(): every step's failure
// must abort the build with a contextual error — a pipeline that shrugs off a
// failed step ships incomplete output with exit code 0.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/notify"
)

// pipelineFixture writes the minimal buildable site (content + templates) and
// returns a Generate-ready config rooted at tmp.
func pipelineFixture(t *testing.T, tmp string) Config {
	t.Helper()
	contentDir := filepath.Join(tmp, "content", "site")
	mustWrite(t, filepath.Join(contentDir, "metadata.json"),
		`{"categories":[{"id":2,"name":"News","slug":"news"}],"exported_at":"","media":[],"users":[]}`)
	mustWrite(t, filepath.Join(contentDir, "posts", "news", "one.md"),
		"---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\ntags: [go]\n---\n\nBody [broken] text.\n")
	tmplDir := filepath.Join(tmp, "templates", "simple")
	writeSimpleTemplates(t, tmplDir)
	mustWrite(t, filepath.Join(tmplDir, "css", "style.css"), "body{color:red}")
	return Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir:   filepath.Join(tmp, "content"),
		TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir:    filepath.Join(tmp, "output"),
		Quiet:        true,
	}
}

// pipelineCase is one failing Generate() step: a config mutation, an optional
// output-tree obstruction, and the error context the step must report.
type pipelineCase struct {
	name    string
	mutate  func(cfg *Config, tmp string)
	preOut  func(t *testing.T, out string)
	wantErr string
}

// runPipelineCase builds the fixture, applies the case and asserts Generate()
// fails with the step's context.
func runPipelineCase(t *testing.T, c pipelineCase) {
	t.Helper()
	tmp := t.TempDir()
	cfg := pipelineFixture(t, tmp)
	if c.mutate != nil {
		c.mutate(&cfg, tmp)
	}
	if c.preOut != nil {
		c.preOut(t, cfg.OutputDir)
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = gen.Generate()
	if err == nil || !strings.Contains(err.Error(), c.wantErr) {
		t.Fatalf("Generate error = %v, want substring %q", err, c.wantErr)
	}
}

// mustSymlinkOrSkip creates a symlink or skips on filesystems without support.
func mustSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

// TestGeneratePipelineErrors: one failing step per case, each expected to
// surface through Generate() with its step context.
func TestGeneratePipelineErrors(t *testing.T) {
	cases := []pipelineCase{
		{"pre_build hook", func(cfg *Config, _ string) {
			cfg.Hooks = map[string][]string{"pre_build": {"/nonexistent-ssg-hook-xyz"}}
		}, nil, "pre_build hook"},
		{"post_build hook", func(cfg *Config, _ string) {
			cfg.Hooks = map[string][]string{"post_build": {"/nonexistent-ssg-hook-xyz"}}
		}, nil, "post_build hook"},
		{"content schema strict", func(cfg *Config, _ string) {
			cfg.Strict = true
			cfg.ContentSchemas = map[string]models.ContentSchema{"post": {Required: []string{"summary"}}}
		}, nil, "content schema"},
		{"shortcode strict", func(cfg *Config, tmp string) {
			cfg.ShortcodeErrors = "strict"
			cfg.ShortcodeBrackets = true
			cfg.Shortcodes = []Shortcode{{Name: "broken", Template: filepath.Join(tmp, "missing.tmpl")}}
		}, nil, "shortcode"},
		{"copy assets", nil, func(t *testing.T, out string) {
			mustWrite(t, filepath.Join(out, "css"), "not a dir")
		}, "copying assets"},
		{"static dir", func(cfg *Config, tmp string) {
			mustWrite(t, filepath.Join(tmp, "static", "s.txt"), "x")
			cfg.StaticDir = filepath.Join(tmp, "static")
		}, func(t *testing.T, out string) {
			mustMkdir(t, filepath.Join(out, "s.txt"))
		}, "copying static directory"},
		{"robots.txt", nil, func(t *testing.T, out string) {
			mustMkdir(t, filepath.Join(out, "robots.txt"))
		}, "robots.txt"},
		{"404 page", nil, func(t *testing.T, out string) {
			// A dangling symlink into a missing directory: Stat says "absent"
			// (so the default 404 is attempted) and the write then fails.
			mustMkdir(t, out)
			mustSymlinkOrSkip(t, "/nonexistent-ssg-dir/404.html", filepath.Join(out, "404.html"))
		}, "404.html"},
		{"llms.txt", func(cfg *Config, _ string) {
			cfg.MarkdownPublish = true
		}, func(t *testing.T, out string) {
			mustMkdir(t, filepath.Join(out, "llms.txt"))
		}, "llms.txt"},
		{"route manifest", func(cfg *Config, _ string) {
			cfg.RouteManifest = true
		}, func(t *testing.T, out string) {
			mustMkdir(t, filepath.Join(out, "routes.json"))
		}, "route manifest"},
		{"declared feeds", func(cfg *Config, _ string) {
			cfg.Feeds = []models.FeedSpec{{Path: "/x.xml", Format: "bogus"}}
		}, nil, "unsupported format"},
		{"builtin feeds", func(cfg *Config, _ string) {
			cfg.Feed = true
		}, func(t *testing.T, out string) {
			mustMkdir(t, filepath.Join(out, feedFileName))
		}, "generating feeds"},
		{"search index", func(cfg *Config, _ string) {
			cfg.SearchIndex = true
		}, func(t *testing.T, out string) {
			mustMkdir(t, filepath.Join(out, "search-index.json"))
		}, "search index"},
		{"cloudflare worker", func(cfg *Config, _ string) {
			cfg.Workers = []WorkerConfig{{}}
		}, nil, "neither dir nor source"},
		{"asset phase", func(cfg *Config, _ string) {
			cfg.Bundles = map[string][]string{"sub/b.js": {"a.js"}}
		}, func(t *testing.T, out string) {
			mustWrite(t, filepath.Join(out, "sub"), "not a dir")
		}, "bundling assets"},
		{"content sources", func(cfg *Config, _ string) {
			cfg.ContentSources = []ContentSource{{Path: "   "}}
		}, nil, "path is required"},
		{"taxonomy archives", nil, func(t *testing.T, out string) {
			mustWrite(t, filepath.Join(out, "tag"), "not a dir")
		}, "generating taxonomies"},
		{"notifications", func(cfg *Config, tmp string) {
			state := filepath.Join(tmp, "state.json")
			mustWrite(t, state, "{corrupt")
			cfg.Notify = notify.New([]notify.Dest{{Name: "hook", URL: "https://example.com/hook"}}, state)
		}, nil, "sending notifications"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { runPipelineCase(t, c) })
	}
}

// TestGenerateCleanOutputError: --clean failing to empty the output directory
// aborts the build instead of rendering into a half-stale tree.
func TestGenerateCleanOutputError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root deletes anything; the permission guard cannot trigger")
	}
	tmp := t.TempDir()
	cfg := pipelineFixture(t, tmp)
	cfg.Clean = true
	locked := filepath.Join(cfg.OutputDir, "locked")
	mustWrite(t, filepath.Join(locked, "f.txt"), "x")
	if err := os.Chmod(locked, 0o555); err != nil { // #nosec G302 -- read-only on a test temp dir
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) }) // #nosec G302 -- restoring perms on a test temp dir
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err == nil || !strings.Contains(err.Error(), "cleaning output") {
		t.Fatalf("Generate error = %v, want cleaning output", err)
	}
}

// TestAssetPhaseFailures: each post-render pass reports its own failure — the
// asset phase runs last, so a swallowed error here is the difference between
// exit 1 and shipping a broken site.
func TestAssetPhaseFailures(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(cfg *Config, tmp string)
		setup  func(t *testing.T, out string)
	}{
		{"scss scan", func(cfg *Config, tmp string) {
			cfg.SCSS = true
			bin := filepath.Join(tmp, "fake-sass")
			mustWrite(t, bin, "not a binary")
			cfg.SassBinary = bin
			cfg.OutputDir = filepath.Join(tmp, "never-built")
		}, nil},
		{"minify walk", func(cfg *Config, tmp string) {
			cfg.MinifyCSS = true
			cfg.OutputDir = filepath.Join(tmp, "never-built")
		}, nil},
		{"fingerprint walk", func(cfg *Config, tmp string) {
			cfg.Fingerprint = true
			cfg.OutputDir = filepath.Join(tmp, "never-built")
		}, nil},
		{"links strict", func(cfg *Config, _ string) { cfg.CheckLinks = "strict" },
			func(t *testing.T, out string) {
				mustWrite(t, filepath.Join(out, "index.html"), `<html><a href="/gone/">x</a></html>`)
			}},
		{"images strict", func(cfg *Config, _ string) { cfg.CheckImages = "strict" },
			func(t *testing.T, out string) {
				mustWrite(t, filepath.Join(out, "index.html"), `<html><img src="a.png"></html>`)
			}},
		{"meta strict", func(cfg *Config, _ string) { cfg.CheckMeta = "strict" },
			func(t *testing.T, out string) {
				mustWrite(t, filepath.Join(out, "index.html"), `<html><head></head><body>x</body></html>`)
			}},
		{"schema strict", func(cfg *Config, _ string) { cfg.CheckSchema = "strict" },
			func(t *testing.T, out string) {
				mustWrite(t, filepath.Join(out, "index.html"),
					`<html><head><script type="application/ld+json">{"@type":"Article"}</script></head></html>`)
			}},
		{"orphans strict", func(cfg *Config, _ string) { cfg.CheckOrphans = "strict" },
			func(t *testing.T, out string) {
				mustWrite(t, filepath.Join(out, "index.html"), `<html><body>x</body></html>`)
				mustWrite(t, filepath.Join(out, "lonely", "index.html"), `<html><body>y</body></html>`)
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tmp := t.TempDir()
			cfg := Config{Domain: "example.com", OutputDir: filepath.Join(tmp, "out"), Quiet: true}
			c.mutate(&cfg, tmp)
			if c.setup != nil {
				mustMkdir(t, cfg.OutputDir)
				c.setup(t, cfg.OutputDir)
			}
			g := &Generator{config: cfg, siteData: &models.SiteData{}}
			if err := g.assetPhase(); err == nil {
				t.Fatal("assetPhase must fail")
			}
		})
	}
}

// TestAssetPhaseMinifyUnreadable: a single unreadable asset fails minification
// with the file named, rather than shipping it un-minified in silence.
func TestAssetPhaseMinifyUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads anything; the permission guard cannot trigger")
	}
	out := t.TempDir()
	locked := filepath.Join(out, "style.css")
	mustWrite(t, locked, "body{}")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) }) // #nosec G302 -- restoring perms on a test temp file
	g := &Generator{config: Config{OutputDir: out, Quiet: true, MinifyCSS: true}, siteData: &models.SiteData{}}
	err := g.assetPhase()
	if err == nil || !strings.Contains(err.Error(), "style.css") {
		t.Fatalf("assetPhase error = %v, want the file named", err)
	}
}
