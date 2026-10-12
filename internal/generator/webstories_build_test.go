package generator

// Which documents get a story and which are skipped, and why.

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// storyBuildGen is a generator whose static dir holds a logo, a photo, a
// broken PNG (a valid header over no pixels) and a broken logo.
func storyBuildGen(t *testing.T) *Generator {
	t.Helper()
	g := storyGen(t)
	static := t.TempDir()
	writeTestPNG(t, filepath.Join(static, "logo.png"), 32, 32)
	writeTestPNG(t, filepath.Join(static, "a.png"), 40, 30)
	writeTestPNG(t, filepath.Join(static, "b.png"), 30, 40)
	full := mustReadBytes(t, filepath.Join(static, "a.png"))
	mustWrite(t, filepath.Join(static, "broken.png"), string(full[:40]))
	g.config.StaticDir = static
	g.config.WebStories.PublisherLogo = "logo.png"
	return g
}

// mustReadBytes reads a fixture file.
func mustReadBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- test fixture
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStoryBuildLogo(t *testing.T) {
	g := storyBuildGen(t)
	for logo, want := range map[string]string{"missing.png": "is not an image of this site", "broken.png": "publisher_logo:"} {
		g.config.WebStories.PublisherLogo = logo
		g.config.Quiet = false
		var ok bool
		out := captureGeneratorStdout(t, func() { ok = newStoryBuild(g, withStoryDefaults(g.config.WebStories)).prepareLogo() })
		if ok || !strings.Contains(out, want) {
			t.Errorf("%s: ok=%v log=%q", logo, ok, out)
		}
	}
}

func TestStoryBuildSkips(t *testing.T) {
	g := storyBuildGen(t)
	b := newStoryBuild(g, withStoryDefaults(g.config.WebStories))
	if !b.prepareLogo() {
		t.Fatal("logo")
	}
	b.taken["/web-stories/taken/"] = true
	post := func(slug, featured, body string) storyCandidate {
		return storyCandidate{page: models.Page{Title: slug, Slug: slug, Type: "post", URLFormat: "slug", FeaturedImage: featured, Content: body}, isPost: true}
	}
	for _, c := range []storyCandidate{
		post("remote", "https://elsewhere.example.org/a.png", ""),
		post("alone", "/a.png", ""),
		post("taken", "/a.png", "![x](b.png)"),
		post("broken", "/broken.png", "![x](b.png)"),
		post("broken-photo", "/a.png", "![x](broken.png)"),
		post("ok", "/a.png", `<img src="/b.png"> ![again](b.png)`),
		{page: models.Page{Slug: "untitled", Type: "post", URLFormat: "slug"}},
	} {
		b.add(c)
	}
	if len(b.stories) != 1 || b.byPost["/ok/"] != "/web-stories/ok/" {
		t.Fatalf("stories = %d, byPost = %v", len(b.stories), b.byPost)
	}
	want := map[string]string{storySkipNotLocal: "remote", storySkipFewImages: "alone", storySkipURLTaken: "taken",
		storySkipImageFailed: "broken, broken-photo", storySkipNoFeatured: "/untitled/"}
	for reason, names := range want {
		if got := strings.Join(b.skips[reason], ", "); got != names {
			t.Errorf("%s: %q, want %q", reason, got, names)
		}
	}
	for i := 0; i < 7; i++ {
		b.skip(storySkipNoFeatured, models.Page{Title: "x"})
	}
	g.config.Quiet = false
	if out := captureGeneratorStdout(t, b.reportSkips); !strings.Contains(out, "8 skipped, no featured image: /untitled/, x, x, x, x and 3 more") {
		t.Errorf("report = %s", out)
	}
}

func TestStoryWebPFallback(t *testing.T) {
	g := storyBuildGen(t)
	g.config.WebStories.WebP = true
	t.Setenv("PATH", "")
	g.config.Quiet = false
	b := newStoryBuild(g, withStoryDefaults(g.config.WebStories))
	var img StoryImage
	var err error
	out := captureGeneratorStdout(t, func() { img, err = b.cut("a.png", 72, 128) })
	if err != nil || !strings.HasSuffix(img.Path, ".jpg") || b.format != "jpeg" || !strings.Contains(out, "stories use JPEG") {
		t.Errorf("fallback: %+v %v %q", img, err, out)
	}
}

func TestGenerateWebStoriesWriteError(t *testing.T) {
	g := storyBuildGen(t)
	g.siteData.Posts = []models.Page{{Title: "Ok", Slug: "ok", Type: "post", URLFormat: "slug", FeaturedImage: "/a.png", Content: "![x](b.png)"}}
	g.tmpl = template.Must(template.New("t").Parse(`{{define "story.html"}}{{.Missing}}{{end}}`))
	if err := g.generateWebStories(); err == nil || !strings.Contains(err.Error(), "/web-stories/ok/") {
		t.Errorf("err = %v", err)
	}
}
