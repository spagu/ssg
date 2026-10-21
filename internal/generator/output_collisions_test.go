package generator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestPageAndRecordAtOneAddressStopTheBuild reproduces #346: a content page
// and an external-source record with the same link, i18n off. The record
// used to replace the page without a word.
func TestPageAndRecordAtOneAddressStopTheBuild(t *testing.T) {
	cfg := mappedTaxonomySite(t, `[{"origin":"benchmarks","slug":"cat-benchmarks","maintainerTerm":"x@example.com"}]`)
	mustWrite(t, filepath.Join(cfg.ContentDir, "site", "pages", "benchmarks.md"),
		"---\ntitle: Benchmarks\nslug: benchmarks\nstatus: publish\ntype: page\n---\nx\n")
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = gen.Generate()
	if err == nil {
		t.Fatal("two documents at /benchmarks/ must stop the build")
	}
	for _, want := range []string{"output collision at /benchmarks/", "benchmarks.md", `external-source record "benchmarks"`, "distinct link:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not say %q", err, want)
		}
	}
}

// TestRecordsAtOneAddressStopTheBuild: two records whose slugs differ but
// whose links are the same — the slug check never saw them.
func TestRecordsAtOneAddressStopTheBuild(t *testing.T) {
	cfg := mappedTaxonomySite(t, `[
		{"origin":"openbsd/lang/rust","slug":"port-a","maintainerTerm":""},
		{"origin":"openbsd/lang/rust","slug":"category-b","maintainerTerm":""}
	]`)
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err == nil || !strings.Contains(err.Error(), "output collision at /openbsd/lang/rust/") {
		t.Fatalf("err = %v", err)
	}
}

func TestDetectPathCollisions(t *testing.T) {
	g := newTestGen(t, "")
	cases := []struct {
		name  string
		pages []models.Page
		posts []models.Page
		want  string // "" = no error
	}{
		{"distinct", []models.Page{{Slug: "a", Type: "page"}, {Slug: "b", Type: "page"}}, nil, ""},
		{"page and page", []models.Page{{Slug: "a", Type: "page", SourceFile: "a.md"}, {Slug: "a", Type: "page", SourceFile: "a2.md"}}, nil, "a.md and a2.md"},
		{"page and post", []models.Page{{Slug: "news", Type: "page", SourceFile: "news.md"}},
			[]models.Page{{Slug: "news", Type: "post", URLFormat: "slug", SourceFile: "post.md"}}, "news.md and post.md"},
		{"link and slug", []models.Page{{Slug: "x", Type: "page", SourceFile: "x.md", Link: "/shared/"}, {Slug: "shared", Type: "page", Title: "Shared"}}, nil, `x.md and "Shared"`},
		// The root keeps #234: the second claimant is skipped with a warning.
		{"two roots", []models.Page{{Slug: "home", Type: "page", Link: "/"}, {Slug: "start", Type: "page", Link: "/"}}, nil, ""},
		// An alias meeting a page keeps its warning: the page is kept.
		{"alias on a page", []models.Page{{Slug: "a", Type: "page"}, {Slug: "b", Type: "page", Aliases: []string{"a"}}}, nil, ""},
	}
	for _, c := range cases {
		g.siteData.Pages, g.siteData.Posts = c.pages, c.posts
		err := g.detectPathCollisions()
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: unexpected %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: err = %v, want it to name %q", c.name, err, c.want)
		}
	}
}

func TestDocumentLabel(t *testing.T) {
	for want, p := range map[string]models.Page{
		"a.md":                             {SourceFile: "a.md", Title: "A"},
		`the external-source record "x/y"`: {FromRecord: true, Title: "x/y"},
		`"Loose"`:                          {Title: "Loose"},
	} {
		if got := documentLabel(p); got != want {
			t.Errorf("documentLabel = %q, want %q", got, want)
		}
	}
}

// TestI18nKeepsItsOwnCollisionCheck: with i18n on, the stricter check runs
// (aliases, the root) and its message is unchanged.
func TestI18nKeepsItsOwnCollisionCheck(t *testing.T) {
	g := newTestGen(t, "")
	g.siteData.Pages = []models.Page{{Slug: "same", Type: "page", SourceFile: "a.md"}, {Slug: "same", Type: "page", SourceFile: "b.md"}}
	if err := g.detectContentCollisions(); err == nil || !strings.Contains(err.Error(), "i18n output collision") {
		t.Errorf("err = %v", err)
	}
}
