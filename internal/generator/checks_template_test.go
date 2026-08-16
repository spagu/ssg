package generator

// Naming a <title> that sits inside a script block (#152) — the defect that
// cost a reporter the better part of a day with byte-clean inputs, because
// html/template escapes by context and the theme looked right in isolation.

import (
	"strings"
	"testing"
)

func TestCheckTemplateScriptContextFindsIt(t *testing.T) {
	src := "<!doctype html><html><head>\n" +
		`<script src="/js/analytics.js">` + "\n" +
		"<title>{{.Site.Title}}</title>\n" +
		"</script>\n</head><body>{{.Site.Title}}</body></html>\n"

	got := checkTemplateScriptContext("index.html", src)
	if len(got) != 1 {
		t.Fatalf("findings = %+v, want one", got)
	}
	f := got[0]
	if f.TitleLine != 3 || f.ScriptLine != 2 || f.CloseLine != 4 {
		t.Fatalf("lines = title %d, script %d, close %d", f.TitleLine, f.ScriptLine, f.CloseLine)
	}
	msg := f.Message()
	for _, want := range []string{"index.html", "line 3", "opened line 2", "closed line 4", "quoted"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q: %s", want, msg)
		}
	}
}

// TestCheckTemplateScriptContextUnclosed: the commonest shape in a scraped
// theme is a script whose end tag was cut away entirely.
func TestCheckTemplateScriptContextUnclosed(t *testing.T) {
	src := "<head>\n<script>\n<title>{{.Site.Title}}</title>\n</head>"
	got := checkTemplateScriptContext("index.html", src)
	if len(got) != 1 || got[0].CloseLine != 0 {
		t.Fatalf("findings = %+v, want one with no close line", got)
	}
	if !strings.Contains(got[0].Message(), "never closed") {
		t.Errorf("message must say the script is never closed: %s", got[0].Message())
	}
}

// TestCheckTemplateScriptContextQuietOnHealthyThemes: the check must not cry
// wolf, or a theme author learns to ignore it.
func TestCheckTemplateScriptContextQuiet(t *testing.T) {
	cases := map[string]string{
		"ordinary head": "<head>\n<title>{{.Site.Title}}</title>\n<script src=\"/a.js\"></script>\n</head>",
		"script after the title with its own close": "<head><title>{{.Site.Title}}</title></head>" +
			"<body><script>var a = 1;</script></body>",
		"json-ld block, properly closed": `<head><script type="application/ld+json">{"@type":"WebSite"}</script>` +
			"<title>{{.Site.Title}}</title></head>",
		"a static title inside a script": "<head><script>\n<title>Nothing interpolated</title>\n</script></head>",
		"no title at all":                "<head><script>var a = 1;</script></head>",
		"an end tag with nothing before": "</script><title>{{.Site.Title}}</title>",
		"empty file":                     "",
	}
	for name, src := range cases {
		if got := checkTemplateScriptContext("t.html", src); len(got) != 0 {
			t.Errorf("%s must not be reported: %+v", name, got)
		}
	}
}

// TestCheckTemplateScriptContextSecondBlock: a healthy script earlier in the
// file must not mask a broken one later.
func TestCheckTemplateScriptContextSecondBlock(t *testing.T) {
	src := "<head>\n<script>var ok = 1;</script>\n<script src=\"/b.js\">\n<title>{{.Title}}</title>\n</script>\n</head>"
	got := checkTemplateScriptContext("index.html", src)
	if len(got) != 1 || got[0].ScriptLine != 3 || got[0].TitleLine != 4 {
		t.Fatalf("findings = %+v", got)
	}
}

func TestLineOf(t *testing.T) {
	src := "a\nb\nc"
	for offset, want := range map[int]int{0: 1, 2: 2, 4: 3, 999: 3} {
		if got := lineOf(src, offset); got != want {
			t.Errorf("lineOf(%d) = %d, want %d", offset, got, want)
		}
	}
}
