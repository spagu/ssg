package generator

// A custom post type's own archive (#165).
//
// The reported site is the whole test: one custom type serves /realizacje/ and
// another 404s at its own section, so the rule cannot be "every folder of
// documents gets an index" — it has to follow what the type declares. Half of
// these tests are therefore about NOT building one.

import (
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/models"
)

// cptGen builds a generator holding the reported arrangement: three entries of
// one custom type, one of another.
func cptGen(t *testing.T) *Generator {
	t.Helper()
	g := newTestGen(t, `{{define "category.html"}}<h1>{{.Name}}</h1>`+
		`<p>kind={{.Kind}} type={{.ContentType}} {{.Pager.Current}}/{{.Pager.Total}}</p>`+
		`<ul>{{range .Posts}}<li>{{.Title}}</li>{{end}}</ul>{{end}}`)
	for i, city := range []string{"wroclaw", "sobotka", "konin"} {
		g.siteData.Pages = append(g.siteData.Pages, models.Page{
			Title: "Montaz " + city, Slug: "montaz-" + city, Type: "realizacje",
			Status: "publish", Link: "/realizacje/montaz-" + city + "/",
			Date: time.Date(2024, time.March, i+1, 0, 0, 0, 0, time.UTC),
		})
	}
	g.siteData.Pages = append(g.siteData.Pages, models.Page{
		Title: "Opinia", Slug: "opinia", Type: "reviews", Status: "publish",
		Link: "/reviews/opinia/",
	})
	return g
}

// TestNothingDeclaredNothingBuilt: every site that has not asked. A folder of
// documents is not evidence that its section exists — the reported site proves
// it, since the source 404s at one of its two type sections.
func TestNothingDeclaredNothingBuilt(t *testing.T) {
	g := cptGen(t)
	if err := g.generateTypeArchives(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"realizacje/index.html", "reviews/index.html"} {
		if fileExists(t, g, path) {
			t.Errorf("%s must not be built without a declaration", path)
		}
	}
}

// TestExportHintBuildsTheArchive: the case that needs no configuration at all —
// metadata.json says which types the source served a section for, so a migration
// produces a site whose own menu links resolve.
func TestExportHintBuildsTheArchive(t *testing.T) {
	g := cptGen(t)
	g.siteData.CustomTypes = []models.CustomType{
		{Slug: "realizacje", Name: "Realizacje", HasArchive: true},
		{Slug: "reviews", Name: "Reviews", HasArchive: false},
	}
	if err := g.generateTypeArchives(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(t, g, "realizacje/index.html") {
		t.Fatal("has_archive: true must build the section")
	}
	if fileExists(t, g, "reviews/index.html") {
		t.Error("has_archive: false must not — the source 404s there too")
	}

	body := mustReadOutput(t, g, "realizacje/index.html")
	if !strings.Contains(body, "<h1>Realizacje</h1>") {
		t.Errorf("the type's own name must title it: %s", body)
	}
	if !strings.Contains(body, "kind=type type=realizacje") {
		t.Errorf("a theme must be able to tell which archive this is: %s", body)
	}
	for _, city := range []string{"wroclaw", "sobotka", "konin"} {
		if !strings.Contains(body, "Montaz "+city) {
			t.Errorf("%s is missing from the listing: %s", city, body)
		}
	}
	// Newest first, like every other archive.
	if i, j := strings.Index(body, "konin"), strings.Index(body, "wroclaw"); i > j {
		t.Errorf("the listing must be newest-first: %s", body)
	}
}

// TestConfigDeclaresAndRefuses: the config is the answer for an export that does
// not carry the field, and an explicit false outranks the export — the operator
// has looked at the source and the export has not.
func TestConfigDeclaresAndRefuses(t *testing.T) {
	g := cptGen(t)
	g.config.TypeArchives = map[string]bool{"reviews": true}
	if err := g.generateTypeArchives(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(t, g, "reviews/index.html") {
		t.Fatal("a declared type must get its section")
	}
	if fileExists(t, g, "realizacje/index.html") {
		t.Error("an undeclared type must not")
	}
	// The name falls back to the slug, readably.
	if body := mustReadOutput(t, g, "reviews/index.html"); !strings.Contains(body, "<h1>reviews</h1>") {
		t.Errorf("name = %s", body)
	}

	// false wins over the export's own claim.
	g2 := cptGen(t)
	g2.siteData.CustomTypes = []models.CustomType{{Slug: "realizacje", HasArchive: true}}
	g2.config.TypeArchives = map[string]bool{"realizacje": false}
	if err := g2.generateTypeArchives(); err != nil {
		t.Fatal(err)
	}
	if fileExists(t, g2, "realizacje/index.html") {
		t.Error("an explicit false must refuse the archive")
	}
}

// TestDeclaredTypeWithNoEntries: an archive of nothing is a page nobody linked.
func TestDeclaredTypeWithNoEntries(t *testing.T) {
	g := cptGen(t)
	g.config.TypeArchives = map[string]bool{"portfolio": true}
	if err := g.generateTypeArchives(); err != nil {
		t.Fatal(err)
	}
	if fileExists(t, g, "portfolio/index.html") {
		t.Error("a type with no entries must not get an empty listing")
	}
}

// TestArchiveYieldsToRealContent: a hand-written section page keeps its URL —
// the same rule category and date archives follow, and the workaround people
// used before this existed.
func TestArchiveYieldsToRealContent(t *testing.T) {
	g := cptGen(t)
	g.config.TypeArchives = map[string]bool{"realizacje": true}
	g.siteData.Pages = append(g.siteData.Pages, models.Page{
		Title: "Realizacje", Slug: "realizacje", Type: "page", Status: "publish",
		Link: "/realizacje/",
	})
	if err := g.generateTypeArchives(); err != nil {
		t.Fatal(err)
	}
	if fileExists(t, g, "realizacje/index.html") {
		t.Error("a generated archive must not overwrite the page that owns the URL")
	}
}

// TestArchivePaginates: the reported site had many entries, and `paginate`
// applies here the way it does to a category archive.
func TestArchivePaginates(t *testing.T) {
	g := cptGen(t)
	g.config.TypeArchives = map[string]bool{"realizacje": true}
	g.config.Paginate = 2
	if err := g.generateTypeArchives(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"realizacje/index.html", "realizacje/page/2/index.html"} {
		if !fileExists(t, g, path) {
			t.Errorf("missing %s", path)
		}
	}
	if first := mustReadOutput(t, g, "realizacje/index.html"); !strings.Contains(first, "1/2") {
		t.Errorf("page 1 pager = %s", first)
	}
	if last := mustReadOutput(t, g, "realizacje/page/2/index.html"); !strings.Contains(last, "2/2") {
		t.Errorf("page 2 pager = %s", last)
	}
}

// TestArchivedTypesResolution covers the declaration rules on their own, where
// the precedence is easiest to read.
func TestArchivedTypesResolution(t *testing.T) {
	g := newTestGen(t, "")
	if got := g.archivedTypes(); len(got) != 0 {
		t.Errorf("nothing declared: %v", got)
	}

	g.siteData.CustomTypes = []models.CustomType{
		{Slug: "Realizacje", Name: "Realizacje", HasArchive: true}, // case is not the key
		{Slug: "reviews", HasArchive: false},
		{Slug: "", HasArchive: true}, // a nameless type declares nothing
	}
	g.config.TypeArchives = map[string]bool{
		"portfolio": true,
		"":          true, // ignored
	}
	got := g.archivedTypes()
	if len(got) != 2 {
		t.Fatalf("archivedTypes = %v, want realizacje and portfolio", got)
	}
	if got["realizacje"] != "Realizacje" {
		t.Errorf("the export's own name must be used: %q", got["realizacje"])
	}
	if got["portfolio"] != "portfolio" {
		t.Errorf("a config-only type falls back to its slug: %q", got["portfolio"])
	}
}

// TestTypeArchiveName: the title a template shows.
func TestTypeArchiveName(t *testing.T) {
	cases := map[[2]string]string{
		{"Realizacje", "realizacje"}:  "Realizacje",
		{"", "case-studies"}:          "case studies",
		{"", "team_members"}:          "team members",
		{"  ", "portfolio"}:           "portfolio",
		{" Nasze prace ", "anything"}: "Nasze prace",
	}
	for in, want := range cases {
		if got := typeArchiveName(in[0], in[1]); got != want {
			t.Errorf("typeArchiveName(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

// TestUnsafeSlugIsRefused: a type slug is data, and data must not escape the
// output directory.
func TestUnsafeSlugIsRefused(t *testing.T) {
	g := cptGen(t)
	g.siteData.Pages = append(g.siteData.Pages, models.Page{
		Title: "Evil", Slug: "evil", Type: "../../etc", Status: "publish",
	})
	g.config.TypeArchives = map[string]bool{"../../etc": true}
	if err := g.generateTypeArchives(); err != nil {
		t.Fatal(err)
	}
	// Nothing outside the output directory, and no panic.
	if fileExists(t, g, "../../etc/index.html") {
		t.Error("a traversing slug must not be written")
	}
}

// TestArchiveSlugMovesTheSection: WordPress lets `has_archive` BE a slug, so a
// type called `realizacje` can serve its archive at /nasze-prace/ (wpexporter
// 1.8.11 already decodes that form). Building it at the type's own slug would
// publish a section nothing links to and leave the real address a 404.
func TestArchiveSlugMovesTheSection(t *testing.T) {
	g := cptGen(t)
	g.siteData.CustomTypes = []models.CustomType{
		{Slug: "realizacje", Name: "Realizacje", HasArchive: true, ArchiveSlug: "nasze-prace"},
	}
	if err := g.generateTypeArchives(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(t, g, "nasze-prace/index.html") {
		t.Fatal("the archive must be published where the source served it")
	}
	if fileExists(t, g, "realizacje/index.html") {
		t.Error("and not at the type's own slug, which nothing links to")
	}
	// The type is still what a theme reads, so styling keys off the type and
	// not off the address.
	if body := mustReadOutput(t, g, "nasze-prace/index.html"); !strings.Contains(body, "type=realizacje") {
		t.Errorf(".ContentType must stay the type: %s", body)
	}
}

// TestArchiveSlugDefaultsToTheType: the ordinary case, and every spelling of
// "not set" behaves the same.
func TestArchiveSlugDefaultsToTheType(t *testing.T) {
	for _, slug := range []string{"", "   ", "/realizacje/"} {
		g := cptGen(t)
		g.siteData.CustomTypes = []models.CustomType{
			{Slug: "realizacje", HasArchive: true, ArchiveSlug: slug},
		}
		if err := g.generateTypeArchives(); err != nil {
			t.Fatal(err)
		}
		if !fileExists(t, g, "realizacje/index.html") {
			t.Errorf("archive_slug %q must land at /realizacje/", slug)
		}
	}
}
