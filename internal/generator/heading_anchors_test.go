package generator

// Self-links on headings (#364).

import (
	"strings"
	"testing"
)

const anchorDoc = "# Title\n\n## Overview\n\nText.\n\n### 1. Deterministic ids\n\n#### Deep\n\n## R&D \"quoted\"\n"

func anchorGen(t *testing.T, o HeadingAnchorsOptions) *Generator {
	t.Helper()
	g, err := New(Config{Domain: "example.com", TOC: true, HeadingAnchors: o})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestHeadingAnchorsInThePageBody: h1–h3 get a link to their own id, h4 does
// not, and the label names the section.
func TestHeadingAnchorsInThePageBody(t *testing.T) {
	g := anchorGen(t, HeadingAnchorsOptions{Enabled: true})
	out := string(g.tmplSafeHTML(nil, nil)(anchorDoc))
	for _, want := range []string{
		`<h2 id="overview">Overview <a class="heading-anchor" href="#overview" aria-label="Link to the section Overview">#</a></h2>`,
		`href="#1-deterministic-ids"`,
		`aria-label="Link to the section R&amp;D &#34;quoted&#34;"`,
		`<h1 id="title">Title <a `,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Contains(out, `href="#deep"`) {
		t.Error("h4 is not in the default levels")
	}
	// The table of contents reads the Markdown, so no "#" leaks into it.
	if toc := string(g.tocContext(anchorDoc)); strings.Contains(toc, "heading-anchor") || !strings.Contains(toc, "Overview") {
		t.Errorf("toc = %s", toc)
	}
}

// TestHeadingAnchorsOptions: levels, symbol, class and label are the site's,
// and markup in them is escaped.
func TestHeadingAnchorsOptions(t *testing.T) {
	g := anchorGen(t, HeadingAnchorsOptions{Enabled: true, Levels: []int{4}, Symbol: "¶", Class: `a"b`, Label: "Odnośnik: {text}"})
	out := string(g.tmplSafeHTML(nil, nil)(anchorDoc))
	if !strings.Contains(out, `<a class="a&#34;b" href="#deep" aria-label="Odnośnik: Deep">¶</a></h4>`) {
		t.Errorf("custom options:\n%s", out)
	}
	if strings.Contains(out, `href="#overview"`) {
		t.Error("h2 is not in levels: [4]")
	}
}

// TestHeadingAnchorsOff: the default changes nothing, and a heading that
// already links to itself is not given a second link.
func TestHeadingAnchorsOff(t *testing.T) {
	off := anchorGen(t, HeadingAnchorsOptions{})
	if out := string(off.tmplSafeHTML(nil, nil)(anchorDoc)); strings.Contains(out, "heading-anchor") {
		t.Errorf("off by default:\n%s", out)
	}
	g := anchorGen(t, HeadingAnchorsOptions{Enabled: true})
	own := `<h2 id="x">X <a href="#x">§</a></h2>`
	if got := g.addHeadingAnchors(own); got != own {
		t.Errorf("a heading with its own link: %s", got)
	}
	if got := g.addHeadingAnchors("<p>no headings</p>"); got != "<p>no headings</p>" {
		t.Errorf("no ids: %s", got)
	}
	mismatched := `<h2 id="x">X</h3>`
	if got := g.addHeadingAnchors(mismatched); got != mismatched {
		t.Errorf("mismatched tags: %s", got)
	}
}
