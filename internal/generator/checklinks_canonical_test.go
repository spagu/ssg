package generator

// The link checker and absolute own-domain URLs (#229): a canonical is always
// absolute, so skipping every absolute URL exempted exactly the reference three
// production sites shipped broken.

import (
	"path/filepath"
	"strings"
	"testing"
)

// buildBrokenCanonicalSite builds a site whose post template canonicalises
// every post to an own-domain URL the build does not write — the pattern found
// live: a category.html hardcoding /category/<slug>/ while serving tags too.
func buildBrokenCanonicalSite(t *testing.T, strict bool) error {
	t.Helper()
	tmp := t.TempDir()
	contentDir := filepath.Join(tmp, "content")
	tmplDir := filepath.Join(tmp, "templates")

	mustWrite(t, filepath.Join(contentDir, "site", "metadata.json"),
		`{"categories":[{"id":2,"name":"News","slug":"news"}],"media":[],"users":[]}`)
	mustWrite(t, filepath.Join(contentDir, "site", "posts", "news", "one.md"),
		"---\ntitle: One\nslug: one\nstatus: publish\ntype: post\ndate: 2024-01-02\ncategories: [News]\n---\n\nBody.\n")

	for _, name := range []string{"base.html", "index.html", "post.html", "page.html",
		"category.html", "tag.html", "taxonomy.html"} {
		body := `<html><head></head><body><p>x</p></body></html>`
		if name == "post.html" {
			// The canonical names a document that does not exist, absolutely —
			// and a genuinely external link stays exempt beside it.
			body = `<html><head>` +
				`<link rel="canonical" href="https://example.com/category/phantom/">` +
				`<a href="https://other.example.net/never-checked/">elsewhere</a>` +
				`</head><body><p>x</p></body></html>`
		}
		mustWrite(t, filepath.Join(tmplDir, "simple", name),
			`{{define "`+name+`"}}`+body+`{{end}}`)
	}

	cfg := Config{
		Source: "site", Template: "simple", Domain: "example.com",
		ContentDir: contentDir, TemplatesDir: tmplDir,
		OutputDir: filepath.Join(tmp, "output"), Quiet: true,
		CheckLinks: "warn",
	}
	if strict {
		cfg.CheckLinks = "strict"
	}
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return gen.Generate()
}

// TestCheckLinksSeesAnAbsoluteOwnDomainCanonical: with check_links strict, a
// canonical naming an own-domain document the build never wrote must fail the
// build — it is a dead reference like any other, just one only crawlers follow.
func TestCheckLinksSeesAnAbsoluteOwnDomainCanonical(t *testing.T) {
	err := buildBrokenCanonicalSite(t, true)
	if err == nil {
		t.Fatal("a canonical to a nonexistent own-domain URL passed a strict build")
	}
	if !strings.Contains(err.Error(), "broken internal link") {
		t.Errorf("failed for a different reason: %v", err)
	}
}

// TestCheckLinksStillIgnoresForeignHosts: the widening must stop at the site's
// own domain — the checker never touches the network, so a foreign URL is not
// something it can verify, only something it must not report.
func TestCheckLinksStillIgnoresForeignHosts(t *testing.T) {
	// warn mode: the foreign link beside the broken canonical must not fail
	// anything, and the build as a whole succeeds.
	if err := buildBrokenCanonicalSite(t, false); err != nil {
		t.Fatalf("warn mode must not fail the build: %v", err)
	}
}

// TestStripOwnDomain pins the reference forms: scheme-full, scheme-relative,
// mixed-case host, bare domain root — and the ones that must pass through.
func TestStripOwnDomain(t *testing.T) {
	g := &Generator{config: Config{Domain: "example.com"}}
	cases := map[string]string{
		"https://example.com/a/":     "/a/",
		"http://example.com/a":       "/a",
		"//example.com/a/":           "/a/",
		"https://EXAMPLE.com/a/":     "/a/",
		"https://example.com":        "/",
		"https://example.com/":       "/",
		"https://example.com?x=1":    "/?x=1",
		"https://example.com#top":    "/#top",
		"https://other.example.net/": "https://other.example.net/",
		"https://example.com.evil/":  "https://example.com.evil/",
		"/plain/":                    "/plain/",
		"mailto:x@example.com":       "mailto:x@example.com",
	}
	for in, want := range cases {
		if got := g.stripOwnDomain(in); got != want {
			t.Errorf("stripOwnDomain(%q) = %q, want %q", in, got, want)
		}
	}

	// With no configured domain nothing can be "own", so nothing is rewritten.
	bare := &Generator{}
	if got := bare.stripOwnDomain("https://example.com/a/"); got != "https://example.com/a/" {
		t.Errorf("no domain: %q was rewritten to %q", "https://example.com/a/", got)
	}
}
