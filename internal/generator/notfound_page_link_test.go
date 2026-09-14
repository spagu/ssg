package generator

// A page slugged 404 and strict link checking (#284).

import (
	"strings"
	"testing"
)

// TestAPageSlugged404PassesStrictLinkChecking: the page is written to
// /404.html, so everything it says about itself must name that file. Before
// the fix its canonical named /404/, and check_links strict failed the build on
// the page's own head — the combination a site that owns its 404 would pick.
func TestAPageSlugged404PassesStrictLinkChecking(t *testing.T) {
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`,
		map[string]string{
			"pages/404.md": "---\ntitle: Not found\nslug: \"404\"\nstatus: publish\n---\n\nGone.\n",
		},
		func(name string) string {
			if name == "page.html" {
				return `<html><head><link rel="canonical" href="{{.CanonicalURL}}"></head>` +
					`<body><a href="{{.URL}}">self</a></body></html>`
			}
			return `<html><head></head><body><p>x</p></body></html>`
		})
	cfg.CheckLinks = "strict"
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatalf("strict link checking failed on the 404 page's own references: %v", err)
	}
	page := mustReadOutput(t, gen, "404.html")
	if !strings.Contains(page, `href="https://example.com/404.html"`) {
		t.Errorf("canonical does not name the file that was written:\n%s", page)
	}
}
