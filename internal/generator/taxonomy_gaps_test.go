package generator

// Taxonomy driver gaps (#44): validation errors on pages (not only posts),
// metadata/slug overlay guards, and write failures propagating out of the
// single generateTaxonomies driver instead of vanishing.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/taxonomy"
)

// taxGapGen builds a generator with the given taxonomy config and posts, with
// the registry already built. Output goes to a temp dir.
func taxGapGen(t *testing.T, taxes map[string]taxonomy.DefinitionConfig, posts []models.Page) *Generator {
	t.Helper()
	g, err := New(Config{Domain: "example.com", OutputDir: t.TempDir(), Quiet: true, Taxonomies: taxes})
	if err != nil {
		t.Fatal(err)
	}
	g.siteData.Posts = posts
	if err := g.buildTaxonomies(); err != nil {
		t.Fatal(err)
	}
	return g
}

// TestBuildTaxonomiesPageViolation: cardinality is enforced for PAGES too — a
// single-value taxonomy with two values on a page fails the build, not only on
// posts (pages carry .Taxonomies for template helpers).
func TestBuildTaxonomiesPageViolation(t *testing.T) {
	g, err := New(Config{Domain: "example.com", Quiet: true,
		Taxonomies: map[string]taxonomy.DefinitionConfig{"difficulty": {Multiple: taxBool(false)}}})
	if err != nil {
		t.Fatal(err)
	}
	g.siteData.Pages = []models.Page{{Slug: "p",
		Extra: map[string]interface{}{"difficulty": []interface{}{"A", "B"}}}}
	if err := g.buildTaxonomies(); err == nil || !strings.Contains(err.Error(), "single-value") {
		t.Fatalf("page cardinality violation must fail the build, got %v", err)
	}
}

// TestAssignUnknownTaxonomyErrors: an assignment for a name the registry does
// not define is a hard error, never a silent drop of the author's data.
func TestAssignUnknownTaxonomyErrors(t *testing.T) {
	g := taxGapGen(t, nil, nil)
	g.taxonomies.Names = append(g.taxonomies.Names, "ghost")
	p := models.Page{Slug: "p", TaxonomiesFM: map[string]interface{}{"ghost": "x"}}
	if err := g.assignPageTaxonomies(&p, true); err == nil || !strings.Contains(err.Error(), "unknown taxonomy") {
		t.Fatalf("unregistered taxonomy assignment must error, got %v", err)
	}
}

// TestTaxonomyMetadataGuards: incomplete category records and non-map term
// metadata are skipped without disturbing the valid entries.
func TestTaxonomyMetadataGuards(t *testing.T) {
	g := taxGapGen(t, nil, []models.Page{{Slug: "a", Categories: []int{1}, Tags: []string{"Go"}}})
	// categoryNames appends the direct frontmatter category string.
	names := g.categoryNames(&models.Page{Category: "Ops"})
	if len(names) != 1 || names[0] != "Ops" {
		t.Errorf("categoryNames = %v, want [Ops]", names)
	}
	// A category with no name or no slug cannot be applied — skipped, no panic.
	g.siteData.Categories[2] = models.Category{ID: 2, Name: "", Slug: "b"}
	g.siteData.Categories[3] = models.Category{ID: 3, Name: "c", Slug: ""}
	g.applyCategorySlugs()
	// Term metadata that is not a map is skipped; the term keeps its slug.
	g.data = map[string]interface{}{"taxonomies": map[string]interface{}{
		"tag": map[string]interface{}{"go": "not-a-map"},
	}}
	g.applyTermMetadata()
	if term := g.taxonomies.Term("tag", "", "Go"); term == nil || term.Slug != "go" {
		t.Errorf("non-map metadata must leave the term alone, got %+v", term)
	}
}

// TestTakenContentURLsIncludesAliases: alias URLs count as taken so an archive
// can never overwrite a redirect a page declared.
func TestTakenContentURLsIncludesAliases(t *testing.T) {
	g := taxGapGen(t, nil, nil)
	g.siteData.Pages = []models.Page{{Slug: "new", Title: "N", Aliases: []string{"/old-path/"}}}
	taken := g.takenContentURLs()
	if owner := taken["/old-path/"]; !strings.Contains(owner, "alias of page new") {
		t.Fatalf("alias URL not claimed: %v", taken)
	}
}

// TestCheckTaxonomyURLsTermCollision: a term archive URL owned by real content
// fails the build with both parties named.
func TestCheckTaxonomyURLsTermCollision(t *testing.T) {
	g := taxGapGen(t, map[string]taxonomy.DefinitionConfig{"technology": {}},
		[]models.Page{{Slug: "a", Extra: map[string]interface{}{"technology": "Go"}}})
	def := g.taxonomies.Definitions["technology"]
	err := g.checkTaxonomyURLs(def, map[string]string{"/technology/go/": "page tech-go"})
	if err == nil || !strings.Contains(err.Error(), "collides with page tech-go") {
		t.Fatalf("term URL collision must error naming the owner, got %v", err)
	}
}

// TestGenerateTaxonomiesSkipsArchiveOff: archive:false means no output tree at
// all for that taxonomy.
func TestGenerateTaxonomiesSkipsArchiveOff(t *testing.T) {
	g := taxGapGen(t, map[string]taxonomy.DefinitionConfig{"internal": {Archive: taxBool(false)}},
		[]models.Page{{Slug: "a", Extra: map[string]interface{}{"internal": "Secret"}}})
	if err := g.generateTaxonomies(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "internal")); err == nil {
		t.Error("archive:false taxonomy must not emit an archive tree")
	}
}

// TestGenerateTaxonomiesWriteErrors: a filesystem refusing an archive directory
// is a build error from every branch of the driver — folded built-ins, custom
// index pages, custom term archives and the folded author archive alike.
func TestGenerateTaxonomiesWriteErrors(t *testing.T) {
	cases := []struct {
		name  string
		posts []models.Page
		block string // path under OutputDir replaced by a plain file
		dirs  string // path under OutputDir created as a real dir first
	}{
		{"folded tag archive", []models.Page{{Slug: "a", Tags: []string{"Go"}}}, "tag", ""},
		{"custom index page", []models.Page{{Slug: "a", Extra: map[string]interface{}{"technology": "Go"}}}, "technology", ""},
		{"custom term archive", []models.Page{{Slug: "a", Extra: map[string]interface{}{"technology": "Go"}}}, "technology/go", "technology"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := taxGapGen(t, map[string]taxonomy.DefinitionConfig{"technology": {}}, c.posts)
			if c.dirs != "" {
				mustMkdir(t, filepath.Join(g.config.OutputDir, filepath.FromSlash(c.dirs)))
			}
			mustWrite(t, filepath.Join(g.config.OutputDir, filepath.FromSlash(c.block)), "in the way")
			if err := g.generateTaxonomies(); err == nil {
				t.Fatal("a blocked archive directory must fail the build")
			}
		})
	}
}

// TestGenerateTaxonomiesAuthorError: the folded author archive propagates its
// write failure through the same driver.
func TestGenerateTaxonomiesAuthorError(t *testing.T) {
	g := taxGapGen(t, nil, []models.Page{{Slug: "a", Author: 1}})
	g.siteData.Authors[1] = models.Author{ID: 1, Name: "Ed", Slug: "ed"}
	mustWrite(t, filepath.Join(g.config.OutputDir, "author"), "in the way")
	if err := g.generateTaxonomies(); err == nil {
		t.Fatal("a blocked author archive must fail the build")
	}
}

// TestPaginateTermPartialLastPage: 3 posts at 2 per page yield chunks of 2 and
// 1, linked both ways.
func TestPaginateTermPartialLastPage(t *testing.T) {
	posts := []models.Page{{Slug: "a"}, {Slug: "b"}, {Slug: "c"}}
	chunks := paginateTerm(posts, 2, "/t/x/")
	if len(chunks) != 2 || len(chunks[0].Posts) != 2 || len(chunks[1].Posts) != 1 {
		t.Fatalf("chunks = %d/%d posts", len(chunks[0].Posts), len(chunks[1].Posts))
	}
	if chunks[0].Pager.NextURL != "/t/x/page/2/" || chunks[1].Pager.PrevURL != "/t/x/" {
		t.Errorf("pager links = %q / %q", chunks[0].Pager.NextURL, chunks[1].Pager.PrevURL)
	}
}

// TestRenderTermArchiveEmptySlug: a term whose slug sanitizes to nothing is
// skipped — there is no URL it could live at.
func TestRenderTermArchiveEmptySlug(t *testing.T) {
	g := taxGapGen(t, map[string]taxonomy.DefinitionConfig{"technology": {}}, nil)
	def := g.taxonomies.Definitions["technology"]
	err := g.renderTermArchive(def, "", TaxonomyInfo{Name: "technology"},
		TaxonomyTerm{Name: "X", Slug: ""}, nil, "/technology/")
	if err != nil {
		t.Fatalf("empty slug must be a silent skip, got %v", err)
	}
	if entries, _ := os.ReadDir(g.config.OutputDir); len(entries) != 0 {
		t.Errorf("no output expected, got %v", entries)
	}
}

// TestWriteTaxonomyTermFeedsGaps: a metadata-only term (count 0) gets no feed,
// and a blocked term directory fails the feed pass loudly.
func TestWriteTaxonomyTermFeedsGaps(t *testing.T) {
	// generate_empty keeps metadata-only terms visible, which is exactly the
	// case the count-0 feed guard exists for.
	cfg := map[string]taxonomy.DefinitionConfig{"technology": {Feed: taxBool(true), GenerateEmpty: taxBool(true)}}
	posts := []models.Page{{Slug: "a", Title: "A", Extra: map[string]interface{}{"technology": "Go"}}}

	g := taxGapGen(t, cfg, posts)
	g.taxonomies.ApplyTermMeta("technology", "Ghost", map[string]interface{}{"description": "empty"}, "")
	if err := g.generateTaxonomyFeeds(10); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "technology", "go", "feed.xml")); err != nil {
		t.Error("term with posts must get a feed")
	}
	if _, err := os.Stat(filepath.Join(g.config.OutputDir, "technology", "ghost")); err == nil {
		t.Error("count-0 term must not emit a feed")
	}

	g2 := taxGapGen(t, cfg, posts)
	mustMkdir(t, filepath.Join(g2.config.OutputDir, "technology"))
	mustWrite(t, filepath.Join(g2.config.OutputDir, "technology", "go"), "in the way")
	if err := g2.generateTaxonomyFeeds(10); err == nil {
		t.Fatal("a blocked term feed directory must fail the build")
	}
}
