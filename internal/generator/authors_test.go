package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAuthorsFixture builds a site with an author (metadata.json users block),
// posts assigned by ID and by name, and explicit pages claiming the /author/…
// and /tag/… archive URLs (GO-050).
func writeAuthorsFixture(t *testing.T, tmp string) {
	t.Helper()
	mustWrite(t, filepath.Join(tmp, "content", "site", "metadata.json"),
		`{"categories":[],"exported_at":"","media":[],"users":[{"id":1,"name":"Ian Zane","slug":"ian-zane"}]}`)
	posts := filepath.Join(tmp, "content", "site", "posts", "news")
	mustWrite(t, filepath.Join(posts, "one.md"),
		"---\ntitle: Post by ID\nslug: post-by-id\nstatus: publish\ntype: post\ndate: 2026-07-01\nauthor: 1\ntags: [cli]\n---\n\nBody.\n")
	mustWrite(t, filepath.Join(posts, "two.md"),
		"---\ntitle: Post by name\nslug: post-by-name\nstatus: publish\ntype: post\ndate: 2026-07-02\nauthor: Ian Zane\n---\n\nBody.\n")
	mustWrite(t, filepath.Join(tmp, "content", "site", "pages", "ian.md"),
		"---\ntitle: Custom Ian page\nslug: ian-custom\nlink: /author/ian-zane/\nstatus: publish\ntype: page\n---\n\nCUSTOM-AUTHOR-PAGE\n")
	mustWrite(t, filepath.Join(tmp, "content", "site", "pages", "cli.md"),
		"---\ntitle: Custom tag page\nslug: cli-custom\nlink: /tag/cli/\nstatus: publish\ntype: page\n---\n\nCUSTOM-TAG-PAGE\n")
}

// TestExplicitPageWinsOverAutoArchives: a hand-written page that owns an
// archive URL suppresses the auto-generated archive instead of being silently
// overwritten, and the suppressed archive stays out of the sitemap (GO-050).
func TestExplicitPageWinsOverAutoArchives(t *testing.T) {
	tmp := t.TempDir()
	writeAuthorsFixture(t, tmp)
	writeSimpleTemplates(t, filepath.Join(tmp, "templates", "simple"))
	gen, out := buildSite(t, tmp)
	// The explicit pages survive at the archive URLs.
	// (writeSimpleTemplates renders a static body, so presence + no archive
	// overwrite is asserted via the sitemap and slug maps below.)
	wantFiles(t,
		filepath.Join(out, "author", "ian-zane", "index.html"),
		filepath.Join(out, "tag", "cli", "index.html"))
	// Suppressed archives stay out of the sitemap: exactly one entry per URL.
	sitemap := mustRead(t, filepath.Join(out, "sitemap.xml"))
	if got := strings.Count(sitemap, "/author/ian-zane/"); got != 1 {
		t.Fatalf("author URL sitemap entries = %d, want 1", got)
	}
	if got := strings.Count(sitemap, "/tag/cli/"); got != 1 {
		t.Fatalf("tag URL sitemap entries = %d, want 1", got)
	}
	// The slug maps used for sitemap/feeds exclude the suppressed archives.
	if _, present := gen.authorSlugs["ian-zane"]; present {
		t.Fatal("suppressed author archive must not register its slug")
	}
	if _, present := gen.tagSlugs["cli"]; present {
		t.Fatal("suppressed tag archive must not register its slug")
	}
}

// TestAuthorArchiveGeneratedWithoutCollision: the normal case keeps working —
// both ID- and name-assigned posts land in /author/<slug>/ (users block).
func TestAuthorArchiveGeneratedWithoutCollision(t *testing.T) {
	tmp := t.TempDir()
	writeAuthorsFixture(t, tmp)
	// Remove the colliding pages: the auto archive should now generate.
	for _, p := range []string{"ian.md", "cli.md"} {
		if err := os.Remove(filepath.Join(tmp, "content", "site", "pages", p)); err != nil {
			t.Fatal(err)
		}
	}
	tmplDir := filepath.Join(tmp, "templates", "simple")
	writeSimpleTemplates(t, tmplDir)
	mustWrite(t, filepath.Join(tmplDir, "author.html"),
		`{{define "author.html"}}<html><body>AUTHOR {{.Name}}{{range .Posts}}[{{.Title}}]{{end}}</body></html>{{end}}`)
	gen, out := buildSite(t, tmp)
	archive := mustRead(t, filepath.Join(out, "author", "ian-zane", "index.html"))
	wantContains(t, "author archive", archive, "AUTHOR Ian Zane", "[Post by ID]", "[Post by name]")
	if gen.authorSlugs["ian-zane"] != "ian-zane" {
		t.Fatalf("authorSlugs = %+v", gen.authorSlugs)
	}
	wantContains(t, "sitemap", mustRead(t, filepath.Join(tmp, "output", "sitemap.xml")),
		"/author/ian-zane/")
}

// TestDefineShellTemplateFallsBack: author.html copied from category.html with
// the {{define "category.html"}} left unchanged is a whitespace-only shell —
// it must fall back to category.html instead of writing a blank archive, and
// a correctly-named define must activate the file (GO-051).
func TestDefineShellTemplateFallsBack(t *testing.T) {
	tmp := t.TempDir()
	writeAuthorsFixture(t, tmp)
	for _, p := range []string{"ian.md", "cli.md"} {
		if err := os.Remove(filepath.Join(tmp, "content", "site", "pages", p)); err != nil {
			t.Fatal(err)
		}
	}
	tmplDir := filepath.Join(tmp, "templates", "simple")
	writeSimpleTemplates(t, tmplDir)
	mustWrite(t, filepath.Join(tmplDir, "category.html"),
		`{{define "category.html"}}<html><body>CATEGORY-BODY {{.Name}}</body></html>{{end}}`)
	// The esj-style probe: file author.html, define still "category.html".
	mustWrite(t, filepath.Join(tmplDir, "author.html"),
		`{{define "category.html"}}<html><body>AUTHORTPL {{.Name}}</body></html>{{end}}`)
	cfg := Config{Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: filepath.Join(tmp, "content"), TemplatesDir: filepath.Join(tmp, "templates"),
		OutputDir: filepath.Join(tmp, "output"), Quiet: true}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	archive := mustRead(t, filepath.Join(tmp, "output", "author", "ian-zane", "index.html"))
	if strings.TrimSpace(archive) == "" {
		t.Fatal("shell template must not produce a blank archive")
	}
	// The shell "author.html" is treated as absent, so the archive renders
	// through the category.html fallback (whichever file's define parsed last
	// supplies that body) — never as a whitespace-only page.
	wantContains(t, "fallback archive", archive, "Ian Zane")

	// A correctly-named define activates the file.
	mustWrite(t, filepath.Join(tmplDir, "author.html"),
		`{{define "author.html"}}<html><body>REAL-AUTHOR {{.Name}}</body></html>{{end}}`)
	gen2, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen2.Generate(); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	wantContains(t, "activated author.html",
		mustRead(t, filepath.Join(tmp, "output", "author", "ian-zane", "index.html")),
		"REAL-AUTHOR Ian Zane")
}

// TestHasPrefixSuffixAliases: the Hugo-compatible aliases resolve alongside
// startsWith/endsWith (v1.8.5).
func TestHasPrefixSuffixAliases(t *testing.T) {
	tmp := t.TempDir()
	writeAuthorsFixture(t, tmp)
	tmplDir := filepath.Join(tmp, "templates", "simple")
	writeSimpleTemplates(t, tmplDir)
	mustWrite(t, filepath.Join(tmplDir, "index.html"),
		`{{define "index.html"}}<html><body>`+
			`p:{{hasPrefix "/author/ian" "/author/"}} s:{{hasSuffix "file.md" ".md"}} `+
			`legacy:{{startsWith "abc" "a"}}{{endsWith "abc" "c"}}</body></html>{{end}}`)
	_, out := buildSite(t, tmp)
	wantContains(t, "index", mustRead(t, filepath.Join(out, "index.html")),
		"p:true", "s:true", "legacy:truetrue")
}
