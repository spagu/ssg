package generator

import (
	"os"
	"path/filepath"
	"testing"
)

// check_links_ignore skips references that are optional by design: a worker
// panel loads the core worker's /core/nav.js and /core/theme.css when core is
// deployed and works without them, so a site without core (the docs site) must
// not fail its strict link check on them. Everything else is still checked.
func TestCheckLinksIgnore(t *testing.T) {
	g := newTestGen(t, "")
	out := g.config.OutputDir
	page := `<script src="/core/nav.js"></script><link rel="stylesheet" href="/core/theme.css?v=1"><a href="/missing/">x</a>`
	if err := os.WriteFile(filepath.Join(out, "index.html"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	g.config.CheckLinks = "strict"
	g.config.CheckLinksIgnore = []string{"/core/nav.js", " ", "/core/theme.css"}
	broken, err := g.checkLinks()
	if err != nil {
		t.Fatal(err)
	}
	if len(broken) != 1 || broken[0].href != "/missing/" {
		t.Fatalf("broken = %+v, want only /missing/", broken)
	}
	g.config.CheckLinksIgnore = []string{"/core/", "/missing/"}
	if err := g.checkLinksIfRequested(); err != nil {
		t.Errorf("a prefix ending in / skips everything under it: %v", err)
	}
	g.config.CheckLinksIgnore = nil
	if broken, _ := g.checkLinks(); len(broken) != 3 {
		t.Errorf("without ignores all three are reported, got %+v", broken)
	}
}

func TestLinkIgnoredMatching(t *testing.T) {
	cases := []struct {
		ref    string
		ignore []string
		want   bool
	}{
		{"/core/nav.js", []string{"/core/nav.js"}, true},
		{"/core/nav.js#x", []string{"/core/nav.js"}, true},
		{"/core/nav.jsx", []string{"/core/nav.js"}, false},
		{"/core/a/b.css", []string{"/core/"}, true},
		{"/corex/a", []string{"/core/"}, false},
		{"/core", []string{"/core/"}, false},
		{"/x", nil, false},
		{"/x", []string{""}, false},
	}
	for _, c := range cases {
		if got := linkIgnored(c.ref, c.ignore); got != c.want {
			t.Errorf("linkIgnored(%q, %q) = %v, want %v", c.ref, c.ignore, got, c.want)
		}
	}
}
