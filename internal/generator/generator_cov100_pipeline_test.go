package generator

// The remaining Generate()/loadPhase()/assetPhase()/generateSite() failure
// paths, each driven through a real build: a step that fails must stop the
// build with its own context rather than ship partial output.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
	"github.com/spagu/ssg/internal/sitegraph"
)

// cachedCase keeps a case's dependency graph inside its own temp dir, so no
// build writes a .ssg-cache next to the package.
func cachedCase(c pipelineCase) pipelineCase {
	mutate := c.mutate
	c.mutate = func(cfg *Config, tmp string) {
		cfg.CacheDir = filepath.Join(tmp, "cache")
		if mutate != nil {
			mutate(cfg, tmp)
		}
	}
	return c
}

// skipIfRoot skips a test whose failure depends on file permissions.
func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
}

// lockDir makes dir unreadable for the rest of the test.
func lockDir(t *testing.T, dir string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(dir, mode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) // #nosec G302 -- restoring a test temp dir
}

// TestGenerateLateStepErrors: each output step after rendering reports its
// own failure through Generate().
func TestGenerateLateStepErrors(t *testing.T) {
	withExtractor(t, func(cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic, error) {
		return apiFixture(cfg.Name), nil, nil
	})
	srv := newTTSServer(t)
	cases := []pipelineCase{
		{"api model", func(cfg *Config, tmp string) {
			cfg.APIDocs = []APIDocsOptions{{Source: apisource.Config{Name: "core", Root: tmp}}}
		}, func(t *testing.T, out string) {
			mustMkdir(t, filepath.Join(out, apiDocsFile))
		}, "writing " + apiDocsFile},
		{"resume json", func(cfg *Config, _ string) {
			cfg.Resume = ResumeOptions{File: writeResume(t, resumeDoc)}
		}, func(t *testing.T, out string) {
			mustMkdir(t, filepath.Join(out, "cv", resumeJSONName))
		}, "writing " + resumeJSONName},
		{"podcast feed", func(cfg *Config, tmp string) {
			cfg.Audio = AudioOptions{Client: audioClient(t, srv.URL), CacheRoot: filepath.Join(tmp, "tts"),
				Feed: true, FeedPath: "../podcast.xml"}
		}, nil, "generating feeds"},
		{"web push", func(cfg *Config, _ string) {
			cfg.WebPush = WebPushOptions{Enabled: true}
		}, func(t *testing.T, out string) {
			mustMkdir(t, out)
			mustSymlinkOrSkip(t, "/nonexistent-ssg-dir/m.json", filepath.Join(out, webManifestName))
		}, "writing " + webManifestName},
		{"build report", func(cfg *Config, _ string) {
			cfg.BuildReport = "../build.json"
		}, nil, "writing build report"},
		{"component assets", func(cfg *Config, tmp string) {
			dir := filepath.Join(tmp, "components")
			mustWrite(t, filepath.Join(dir, "yt", "template.html"), `<figure>{{ .Props.id }}</figure>`)
			mustWrite(t, filepath.Join(dir, "yt", "assets", "yt.css"), ".yt{}")
			mustWrite(t, filepath.Join(tmp, "content", "site", "pages", "v.md"),
				"---\ntitle: V\nslug: v\nstatus: publish\ntype: page\n---\n\n{{< yt id=\"x\" >}}\n")
			cfg.ComponentsDir = dir
		}, func(t *testing.T, out string) {
			mustWrite(t, filepath.Join(out, "components"), "not a dir")
		}, "component"},
		{"markup strict", func(cfg *Config, tmp string) {
			mustWrite(t, filepath.Join(tmp, "content", "site", "pages", "b.md"),
				"---\ntitle: B\nslug: b\nstatus: publish\ntype: page\n---\n\n"+builderBody)
			cfg.CheckMarkup = "strict"
		}, nil, "render as literal text"},
		{"site graph", func(cfg *Config, _ string) {
			cfg.SiteGraph = true
		}, func(t *testing.T, out string) {
			mustMkdir(t, filepath.Join(out, sitegraph.FileName))
		}, "site graph"},
		{"api check strict", func(cfg *Config, tmp string) {
			spec := filepath.Join(tmp, "svc.yaml")
			mustWrite(t, spec, "openapi: 3.0.0\ninfo: {title: ' '}\npaths:\n  /a/{id}:\n    get: {}\n")
			cfg.APIDocs = []APIDocsOptions{{OpenAPI: spec}}
			cfg.CheckAPI = "strict"
		}, nil, "API documentation problem"},
		{"date archives", func(cfg *Config, _ string) {
			cfg.DateArchives = true
		}, func(t *testing.T, out string) {
			mustWrite(t, filepath.Join(out, "2024"), "not a dir")
		}, "generating date archives"},
		{"type archives", func(cfg *Config, tmp string) {
			mustWrite(t, filepath.Join(tmp, "content", "site", "pages", "job.md"),
				"---\ntitle: Job\nslug: job\nstatus: publish\ntype: realizacje\n---\n\nDone.\n")
			cfg.TypeArchives = map[string]bool{"realizacje": true}
		}, func(t *testing.T, out string) {
			mustWrite(t, filepath.Join(out, "realizacje"), "not a dir")
		}, "generating content-type archives"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { runPipelineCase(t, cachedCase(c)) })
	}
}

// TestLoadPhaseErrors: a source the build cannot read stops it before any
// page is written.
func TestLoadPhaseErrors(t *testing.T) {
	cases := []pipelineCase{
		{"api docs source", func(cfg *Config, tmp string) {
			empty := filepath.Join(tmp, "no-package")
			mustMkdir(t, empty)
			cfg.APIDocs = []APIDocsOptions{{Source: apisource.Config{Root: empty}}}
		}, nil, "reading code for API docs"},
		{"resume file", func(cfg *Config, tmp string) {
			cfg.Resume = ResumeOptions{File: filepath.Join(tmp, "missing-cv.json")}
		}, nil, "resume"},
		{"pages path", func(_ *Config, tmp string) {
			mustWrite(t, filepath.Join(tmp, "content", "site", "pages"), "not a dir")
		}, nil, "loading pages"},
		{"posts path", func(cfg *Config, tmp string) {
			mustWrite(t, filepath.Join(tmp, "content", "site", "flat"), "not a dir")
			cfg.PostsPath = "flat"
		}, nil, "loading posts"},
		{"partials", func(_ *Config, tmp string) {
			mustWrite(t, filepath.Join(tmp, "templates", "simple", "partials", "bad.html"), "{{end}}")
		}, nil, "parsing partials templates"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { runPipelineCase(t, cachedCase(c)) })
	}
}

// TestBasePathRewriteError: an output file base_path cannot read fails the
// build with the step named.
func TestBasePathRewriteError(t *testing.T) {
	skipIfRoot(t)
	runPipelineCase(t, cachedCase(pipelineCase{"base path", func(cfg *Config, _ string) {
		cfg.BasePath = "/site"
	}, func(t *testing.T, out string) {
		locked := filepath.Join(out, "locked")
		mustWrite(t, filepath.Join(locked, "a.html"), "<a href=\"/x/\">x</a>")
		lockDir(t, locked, 0o000)
	}, "applying base_path"}))
}

// TestWebStoryWriteError: a story that cannot be written fails the build.
// Story images go through the shared image cache, which lives under the
// working directory, so the test runs from its temp dir.
func TestWebStoryWriteError(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, map[string]string{
		"posts/recipes/cake.md": storyCakePost,
	}, nil)
	root := filepath.Dir(cfg.ContentDir)
	t.Chdir(root)
	site := filepath.Join(cfg.ContentDir, cfg.Source)
	writeTestPNG(t, filepath.Join(site, "media", "cake.png"), 400, 300)
	writeTestPNG(t, filepath.Join(site, "media", "sliced.png"), 300, 600)
	writeTestPNG(t, filepath.Join(site, "posts", "recipes", "batter.png"), 120, 90)
	writeTestPNG(t, filepath.Join(site, "media", "logo.png"), 64, 64)
	cfg.CacheDir = filepath.Join(root, "cache")
	cfg.PostURLFormat = "slug"
	cfg.WebStories = WebStoriesOptions{Enabled: true, PublisherLogo: "media/logo.png"}
	mustWrite(t, filepath.Join(cfg.OutputDir, "web-stories"), "not a dir")
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err == nil || !strings.Contains(err.Error(), "web story") {
		t.Fatalf("Generate error = %v, want the web story named", err)
	}
}

// TestBuildCoversRenderHelpers: one successful build that reaches the
// template helpers and render paths no other build test does — hasTranslation
// in a theme, strip_md_link_text in content, a page posts_page replaces, and a
// paginated page whose tail cannot be written (a warning, not a failure).
func TestBuildCoversRenderHelpers(t *testing.T) {
	files := map[string]string{
		"pages/blog.md":  "---\ntitle: Blog\nslug: blog\nstatus: publish\ntype: page\n---\n\nReplaced.\n",
		"pages/list.md":  "---\ntitle: List\nslug: list\nstatus: publish\ntype: page\npaginate: 1\n---\n\nIntro.\n",
		"pages/about.md": "---\ntitle: About\nslug: about\nstatus: publish\ntype: page\n---\n\nSee [guide.md](guide.md).\n",
	}
	for _, n := range []string{"one", "two"} {
		files["posts/news/"+n+".md"] = "---\ntitle: P\nslug: " + n + "\nstatus: publish\ntype: post\ndate: 2024-01-02\n---\n\nBody.\n"
	}
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, func(name string) string {
		if name == "page.html" {
			return `<html><body>{{if hasTranslation "pl" .}}pl{{end}}{{.Content}}</body></html>`
		}
		return `<html><body><p>x</p></body></html>`
	})
	cfg.CacheDir = filepath.Join(t.TempDir(), "cache")
	cfg.PostsPage = "blog"
	cfg.StripMdLinkText = true
	mustWrite(t, filepath.Join(cfg.OutputDir, "list", "page"), "not a dir")
	buildSiteFixture(t, cfg)
	about := mustRead(t, filepath.Join(cfg.OutputDir, "about", indexHTMLName))
	if strings.Contains(about, ">guide.md<") {
		t.Errorf("strip_md_link_text must drop .md from the link text:\n%s", about)
	}
}
