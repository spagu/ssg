package generator

import "testing"

// GO-056: a .md link carrying an anchor or query never matched the rewrite
// regex (which required the href to END with ".md"), so cross-document deep
// links silently shipped as dead hrefs while the same link without an anchor
// was rewritten correctly.
func TestRewriteMdLinksKeepsAnchorsAndQueries(t *testing.T) {
	g := &Generator{config: Config{RewriteMdLinks: true}}
	mdLinks := map[string]map[string]string{
		"CONFIGURATION.md": {"": "/configuration/"},
		"guide":            {"": "/guide/"},
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"plain link",
			`<a href="CONFIGURATION.md">cfg</a>`,
			`<a href="/configuration/">cfg</a>`,
		},
		{
			"anchor is carried over",
			`<a href="CONFIGURATION.md#mddb-content">cfg</a>`,
			`<a href="/configuration/#mddb-content">cfg</a>`,
		},
		{
			"relative path with anchor",
			`<a href="../docs/CONFIGURATION.md#data-and-variables">cfg</a>`,
			`<a href="/configuration/#data-and-variables">cfg</a>`,
		},
		{
			"query string is carried over",
			`<a href="CONFIGURATION.md?v=2">cfg</a>`,
			`<a href="/configuration/?v=2">cfg</a>`,
		},
		{
			"extension-less key still resolves, with its anchor",
			`<a href="guide.md#top">g</a>`,
			`<a href="/guide/#top">g</a>`,
		},
		{
			"unknown target is left untouched",
			`<a href="NOPE.md#x">n</a>`,
			`<a href="NOPE.md#x">n</a>`,
		},
		{
			"non-markdown links are not touched",
			`<a href="/style.css">s</a><a href="https://example.com/a.md.html">x</a>`,
			`<a href="/style.css">s</a><a href="https://example.com/a.md.html">x</a>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := g.rewriteMdLinks(tc.in, mdLinks); got != tc.want {
				t.Errorf("rewriteMdLinks(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
