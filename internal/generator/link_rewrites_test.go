package generator

import "testing"

// LINK-002: documentation links to repository files the site never publishes.
func TestApplyLinkRewrites(t *testing.T) {
	g := &Generator{config: Config{LinkRewrites: map[string]string{
		"../examples/":          "https://github.com/spagu/ssg/tree/main/examples/",
		"../examples/one-file/": "https://example.com/specific/",
		"../LICENSE":            "https://github.com/spagu/ssg/blob/main/LICENSE",
	}}}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"prefix rewrite keeps the remainder",
			`<a href="../examples/dynamic-taxonomies/">x</a>`,
			`<a href="https://github.com/spagu/ssg/tree/main/examples/dynamic-taxonomies/">x</a>`,
		},
		{
			"longest prefix wins regardless of map order",
			`<a href="../examples/one-file/a.md">x</a>`,
			`<a href="https://example.com/specific/a.md">x</a>`,
		},
		{
			"exact file rule",
			`<a href="../LICENSE">x</a>`,
			`<a href="https://github.com/spagu/ssg/blob/main/LICENSE">x</a>`,
		},
		{
			"unrelated links are untouched",
			`<a href="/configuration/">x</a><a href="https://example.com">y</a>`,
			`<a href="/configuration/">x</a><a href="https://example.com">y</a>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := g.applyLinkRewrites(tc.in); got != tc.want {
				t.Errorf("applyLinkRewrites(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// No configuration must mean no work and no change — the backward-compatible
// default for every existing site.
func TestApplyLinkRewritesDisabled(t *testing.T) {
	g := &Generator{config: Config{}}
	in := `<a href="../examples/x/">x</a>`
	if got := g.applyLinkRewrites(in); got != in {
		t.Errorf("applyLinkRewrites without config = %q, want it unchanged", got)
	}
	// An empty prefix key is ignored rather than matching everything.
	g = &Generator{config: Config{LinkRewrites: map[string]string{"": "https://evil/"}}}
	if got := g.applyLinkRewrites(in); got != in {
		t.Errorf("empty prefix rewrote %q", got)
	}
}
