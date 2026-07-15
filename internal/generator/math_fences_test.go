package generator

import (
	"strings"
	"testing"
)

// GO-055: fenced ```math becomes display math; nested fences stay code.
func TestMathFencesToDisplay(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string // substrings expected in output
		not  []string // substrings that must be gone/absent
	}{
		{
			name: "simple fence",
			in:   "intro\n```math\nE = mc^2\n```\noutro",
			want: []string{"$$\nE = mc^2\n$$", "intro", "outro"},
			not:  []string{"```math"},
		},
		{
			name: "fence inside code block untouched",
			in:   "```go\nfmt.Println(\"```math\")\n```\n",
			want: []string{"```go"},
			not:  []string{"$$"},
		},
		{
			name: "unclosed math fence gets closing pair",
			in:   "```math\na+b",
			want: []string{"$$\na+b\n$$"},
		},
		{
			name: "no math fence is a no-op",
			in:   "plain **markdown**",
			want: []string{"plain **markdown**"},
			not:  []string{"$$"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mathFencesToDisplay(tc.in)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("output missing %q:\n%s", w, got)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(got, n) {
					t.Errorf("output should not contain %q:\n%s", n, got)
				}
			}
		})
	}
}

// End-to-end: with math enabled, a ```math-only page converts to HTML that
// carries $$ (so mathHTMLString injects KaTeX) and no language-math code block.
func TestConvertMarkdownFencedMathEndToEnd(t *testing.T) {
	g := &Generator{config: Config{Math: true}}
	html := g.convertMarkdownToHTML("# T\n\n```math\n\\frac{a}{b}\n```\n")
	if !strings.Contains(html, "$$") {
		t.Errorf("converted HTML must contain $$ for the KaTeX gate, got:\n%s", html)
	}
	if strings.Contains(html, "language-math") {
		t.Errorf("fenced math must not render as a code block, got:\n%s", html)
	}
	if got := mathHTMLString(html); !strings.Contains(got, "katex.min.css") {
		t.Errorf("mathHTMLString should inject KaTeX for fenced math, got:\n%s", got)
	}

	// Math disabled: fence stays a code block (no silent content rewriting).
	gOff := &Generator{config: Config{Math: false}}
	off := gOff.convertMarkdownToHTML("```math\nx\n```\n")
	if strings.Contains(off, "$$") {
		t.Errorf("math disabled must not rewrite fences, got:\n%s", off)
	}
}
