package generator

// The "Built with ssg" notice (1.9.0).

import (
	"strings"
	"testing"
)

func attributionGen(t *testing.T, on *bool, version string) *Generator {
	t.Helper()
	g, err := New(Config{Domain: "example.com", Attribution: on, Version: version})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

const plainDoc = "<html><head><title>x</title></head><body><main>Hi</main></body></html>"

func TestAttributionDefaultsOn(t *testing.T) {
	g := attributionGen(t, nil, "1.9.0")
	out := g.injectAttribution(plainDoc)
	for _, want := range []string{
		`<meta name="generator" content="ssg">` + "\n</head>",
		`<p class="ssg-attribution" data-ssg-attribution>Built with <a href="https://ssg.tradik.com/" rel="generator">ssg</a></p>` + "\n</body>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	// The whole transform pipeline carries it too.
	if full := g.transformHTMLPage(plainDoc, nil, false); !strings.Contains(full, "data-ssg-attribution") {
		t.Errorf("transformHTMLPage dropped the notice:\n%s", full)
	}
}

func TestAttributionOff(t *testing.T) {
	off := false
	if out := attributionGen(t, &off, "1.9.0").injectAttribution(plainDoc); out != plainDoc {
		t.Errorf("attribution: false must leave the page alone:\n%s", out)
	}
}

// TestAttributionDefersToTheme: a theme's own notice, or its own generator
// meta, is not doubled.
func TestAttributionDefersToTheme(t *testing.T) {
	g := attributionGen(t, nil, "1.9.0")
	theme := `<html><head><meta name="generator" content="mine"></head><body><footer><p data-ssg-attribution>Built with <a href="https://ssg.tradik.com/">SSG</a>.</p></footer></body></html>`
	if out := g.injectAttribution(theme); out != theme {
		t.Errorf("theme notice must win:\n%s", out)
	}
	link := `<html><head></head><body><a href="https://ssg.tradik.com/">ssg</a></body></html>`
	if out := g.injectAttribution(link); strings.Contains(out, "ssg-attribution") {
		t.Errorf("a link to the project already credits it:\n%s", out)
	}
}

// TestAttributionSkipsStubsAndFragments: a redirect stub gets the meta tag
// only; a fragment without <body> is left alone.
func TestAttributionSkipsStubsAndFragments(t *testing.T) {
	g := attributionGen(t, nil, "1.9.0")
	stub := `<html><head><meta http-equiv="refresh" content="0; url=/new/"></head><body></body></html>`
	out := g.injectAttribution(stub)
	if strings.Contains(out, "ssg-attribution") || !strings.Contains(out, `name="generator"`) {
		t.Errorf("stub: meta only:\n%s", out)
	}
	if out := g.injectAttribution("<p>fragment</p>"); out != "<p>fragment</p>" {
		t.Errorf("fragment untouched:\n%s", out)
	}
}
