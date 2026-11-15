package generator

import (
	"fmt"
	"strings"
	"testing"
)

// GO-183 (security review of editor-ui): a front-matter alias becomes a
// `_redirects` line "<from> <to> <status>". A quoted YAML alias can hold a
// newline or spaces ("x https://example.com/ 302\n/* https://example.com/ 301!"),
// and before the review it went into the file as written — one more rule,
// a site-wide redirect, from anyone allowed to publish an article. Config
// `redirects:` (plain in the settings overlay) took the same path.
func TestRedirectRulesWithWhitespaceAreDropped(t *testing.T) {
	rules := normalizeRedirects([]RedirectRule{
		{From: "/old", To: "/new"},
		{From: "/x https://example.com/ 302\n/*", To: "/target"},
		{From: "/a\tb", To: "/c"},
		{From: "/ok", To: "https://example.com/ 302\n/* https://example.com/"},
		{From: "/u v", To: "/w"},
		{From: "  /trimmed  ", To: " /fine "},
	})
	if len(rules) != 2 || rules[0].From != "/old" || rules[1].From != "/trimmed" || rules[1].To != "/fine" {
		t.Fatalf("rules = %+v, want only /old and /trimmed", rules)
	}
	out := renderRedirectsFile(rules)
	if strings.Contains(out, "example.com") || strings.Count(out, "\n/") != 2 {
		t.Errorf("injected line in _redirects:\n%s", out)
	}
}

// The two branches of this file the existing tests left open (GO-183 brings
// every file it touches to full coverage): the status tie-break of the sort,
// and a chain longer than maxRedirectChainHops.
func TestRedirectSortTieBreakAndLongChain(t *testing.T) {
	got := sortedRedirects([]RedirectRule{{From: "/a", To: "/b", Status: 302}, {From: "/a", To: "/b", Status: 301}})
	if got[0].Status != 301 || got[1].Status != 302 {
		t.Errorf("same from and to: want status order, got %+v", got)
	}
	var chain []RedirectRule
	for i := 0; i <= maxRedirectChainHops+1; i++ {
		chain = append(chain, RedirectRule{From: fmt.Sprintf("/r%d", i), To: fmt.Sprintf("/r%d", i+1), Status: 301})
	}
	if _, err := flattenRedirectChains(chain); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("a chain of %d hops: err = %v", len(chain), err)
	}
	g := &Generator{aliasRedirects: chain}
	if _, _, err := g.collectRedirects(); err == nil {
		t.Error("collectRedirects: the chain error must reach the build")
	}
	ext := &Generator{aliasRedirects: []RedirectRule{{From: "/away", To: "https://example.com/"}}}
	if rules, warnings, err := ext.collectRedirects(); err != nil || len(rules) != 1 || len(warnings) != 0 {
		t.Errorf("an external target is not looked up in the output: %v %v %v", rules, warnings, err)
	}
}
