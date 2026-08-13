package generator

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func TestRootPage(t *testing.T) {
	pages := []models.Page{
		{Slug: "about", Link: "/about/"},
		{Slug: "home", Link: "/", SourceFile: "home.md", Lang: "en"},
		{Slug: "pl-home", Link: "/", Lang: "pl"},
	}

	// A single-language build does not filter on `lang:` — an exported page
	// carries one whether or not the site is multilingual.
	front := rootPage(pages, "")
	if front == nil || front.Slug != "home" {
		t.Fatalf("expected the page linked at /, got %+v", front)
	}
	// Each language has its own front page.
	if front := rootPage(pages, "pl"); front == nil || front.Slug != "pl-home" {
		t.Errorf("expected the pl front page, got %+v", front)
	}
	if front := rootPage(pages, "de"); front != nil {
		t.Errorf("a language with no front page yields nil, got %+v", front)
	}
	if front := rootPage([]models.Page{{Slug: "about", Link: "/about/"}}, ""); front != nil {
		t.Errorf("no page claims the root, got %+v", front)
	}
}

func TestPostsListingPrefix(t *testing.T) {
	g := newTestGen(t, "")

	// Nothing claims the root: the listing keeps it.
	if prefix, ok := g.postsListingPrefix("", false); !ok || prefix != "" {
		t.Errorf("listing should own the root, got %q ok=%v", prefix, ok)
	}
	// posts_page moves it even when the root is free — an explicit choice.
	g.config.PostsPage = "/blog/"
	if prefix, ok := g.postsListingPrefix("", false); !ok || prefix != "blog" {
		t.Errorf("posts_page should move the listing, got %q ok=%v", prefix, ok)
	}
	// A front page took the root, and posts_page says where the listing goes.
	if prefix, ok := g.postsListingPrefix("pl", true); !ok || prefix != "pl/blog" {
		t.Errorf("expected the language-prefixed listing, got %q ok=%v", prefix, ok)
	}
	// A front page took the root and nothing else is configured: no listing,
	// rather than one silently overwriting the front page.
	g.config.PostsPage = ""
	if prefix, ok := g.postsListingPrefix("", true); ok {
		t.Errorf("the listing has nowhere to go, got %q ok=%v", prefix, ok)
	}
}

func TestGetOutputPaths_FrontPageIsAlwaysRootIndex(t *testing.T) {
	g := newTestGen(t, "")

	for _, format := range []string{"", "directory", "flat", "both"} {
		g.config.PageFormat = format
		paths := g.getOutputPaths("")
		if len(paths) != 1 {
			t.Fatalf("page_format %q: expected one output path, got %v", format, paths)
		}
		// "flat" would otherwise write a file literally called ".html".
		if got := paths[0]; got != g.config.OutputDir+"/index.html" {
			t.Errorf("page_format %q: front page wrote to %q", format, got)
		}
	}
}

func TestReportFrontPage(t *testing.T) {
	g := newTestGen(t, "")
	page := &models.Page{Slug: "home", SourceFile: "home.md"}

	out, _ := capture(t, func() error {
		g.reportFrontPage(page, "blog", true)
		return nil
	})
	for _, want := range []string{"home.md", "/blog/"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the report:\n%s", want, out)
		}
	}

	// Posts with nowhere to be listed are called out, not hidden.
	g.siteData.Posts = []models.Page{{Slug: "a"}, {Slug: "b"}}
	out, _ = capture(t, func() error {
		g.reportFrontPage(page, "", false)
		return nil
	})
	if !strings.Contains(out, "2 post(s) are not listed") || !strings.Contains(out, "posts_page") {
		t.Errorf("expected the un-listed posts warning:\n%s", out)
	}

	// No front page, or a quiet build: nothing at all.
	if out, _ := capture(t, func() error { g.reportFrontPage(nil, "", false); return nil }); out != "" {
		t.Errorf("no front page should print nothing, got %q", out)
	}
	g.config.Quiet = true
	if out, _ := capture(t, func() error { g.reportFrontPage(page, "", false); return nil }); out != "" {
		t.Errorf("a quiet build should print nothing, got %q", out)
	}
}

func TestFrontPageLabel(t *testing.T) {
	if got := frontPageLabel(models.Page{SourceFile: "home.md", Title: "Home"}); got != "home.md" {
		t.Errorf("the source file names the page, got %q", got)
	}
	if got := frontPageLabel(models.Page{Title: "Home"}); got != "Home" {
		t.Errorf("without a file the title stands in, got %q", got)
	}
}
