package repair

import (
	"strings"
	"testing"
)

// elementorPage is the shape a WordPress page builder exports: the closing tags
// of one widget, a blank line where `</p>` used to be, then more tab-indented
// markup — which CommonMark reads as an indented code block and prints verbatim.
const elementorPage = `---
title: "Logistics"
meta:
  "generator": "WordPress 7.0.3"
---

## Content

<div class="elementor-widget-container">
			Our logistics consulting services are personalised.

			We specialise in KPI systems.

				</div>
			</div>
		</section>
`

func TestScan_FindsIndentedMarkupBlock(t *testing.T) {
	findings := Scan(elementorPage)

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Lines != 3 {
		t.Errorf("block should span 3 lines, got %d", f.Lines)
	}
	if !strings.HasPrefix(f.Sample, "</div>") {
		t.Errorf("sample should show the offending markup, got %q", f.Sample)
	}
	// Line numbers count from the top of the FILE so an editor can jump there.
	if got := strings.Split(elementorPage, "\n")[f.Line-1]; strings.TrimSpace(got) != "</div>" {
		t.Errorf("Line %d points at %q, not the block start", f.Line, got)
	}
}

func TestApply_DedentsMarkupAndKeepsFrontMatter(t *testing.T) {
	out, changed := Apply(elementorPage)

	if changed != 3 {
		t.Errorf("expected 3 dedented lines, got %d", changed)
	}
	if !strings.Contains(out, "\n  \"generator\": \"WordPress 7.0.3\"\n") {
		t.Errorf("front-matter indentation must survive:\n%s", out)
	}
	if !strings.Contains(out, "\n</div>\n</div>\n</section>") {
		t.Errorf("closing tags should be flush left:\n%s", out)
	}
	// The prose lines still sit inside an unbroken HTML block, so they were
	// never code and are left exactly as they were.
	if !strings.Contains(out, "\t\t\tOur logistics consulting services are personalised.") {
		t.Errorf("prose inside the open HTML block should be untouched:\n%s", out)
	}
}

func TestApply_Idempotent(t *testing.T) {
	once, _ := Apply(elementorPage)
	twice, changed := Apply(once)

	if changed != 0 {
		t.Errorf("second pass changed %d lines, should be a no-op", changed)
	}
	if twice != once {
		t.Error("second pass altered the content")
	}
}

func TestApply_LeavesCleanContentByteIdentical(t *testing.T) {
	src := "---\ntitle: \"Post\"\n---\n\nA normal paragraph.\n\n- one\n- two\n"

	out, changed := Apply(src)

	if changed != 0 || out != src {
		t.Errorf("clean content was rewritten (%d lines):\n%s", changed, out)
	}
	if len(Scan(src)) != 0 {
		t.Error("clean content should produce no findings")
	}
}

func TestScan_IgnoresFencedCode(t *testing.T) {
	src := "Intro:\n\n```html\n    <div class=\"demo\">\n    </div>\n```\n\nDone.\n"

	if f := Scan(src); len(f) != 0 {
		t.Errorf("fenced markup is deliberate, got findings: %+v", f)
	}
	if out, changed := Apply(src); changed != 0 || out != src {
		t.Errorf("fenced block was rewritten:\n%s", out)
	}
}

func TestScan_IgnoresIndentedCodeWithoutMarkup(t *testing.T) {
	src := "Example:\n\n    go build ./...\n    go test ./...\n"

	if f := Scan(src); len(f) != 0 {
		t.Errorf("a real indented code block must not be a finding: %+v", f)
	}
}

func TestScan_IgnoresListContinuation(t *testing.T) {
	// A list item's continuation is indented on purpose, and may carry inline
	// markup; dedenting it would break the list.
	src := "- first item\n\n    <em>continued</em> under the item\n\n- second item\n"

	if f := Scan(src); len(f) != 0 {
		t.Errorf("list continuation must not be a finding: %+v", f)
	}
}

func TestScan_UnterminatedFrontMatterIsLeftAlone(t *testing.T) {
	src := "---\ntitle: \"Broken\"\n\t\t<div>never closed</div>\n"

	if f := Scan(src); len(f) != 0 {
		t.Errorf("unterminated front matter must not be rewritten: %+v", f)
	}
}

func TestScan_NoFrontMatter(t *testing.T) {
	src := "\t\t<div class=\"x\">\n\t\t</div>\n"

	findings := Scan(src)

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %+v", findings)
	}
	if findings[0].Line != 1 {
		t.Errorf("finding should start at line 1, got %d", findings[0].Line)
	}
}

func TestScan_EmptyContent(t *testing.T) {
	if f := Scan(""); len(f) != 0 {
		t.Errorf("empty content should produce no findings: %+v", f)
	}
	if out, changed := Apply(""); changed != 0 || out != "" {
		t.Errorf("empty content should come back unchanged, got %q", out)
	}
}

func TestScan_SampleIsTruncated(t *testing.T) {
	long := "\t<div class=\"" + strings.Repeat("elementor-element ", 10) + "\">\n"

	findings := Scan(long)

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %+v", findings)
	}
	if !strings.HasSuffix(findings[0].Sample, "…") {
		t.Errorf("long sample should be truncated: %q", findings[0].Sample)
	}
	if n := len([]rune(findings[0].Sample)); n != sampleWidth+1 {
		t.Errorf("sample should be %d runes plus the ellipsis, got %d", sampleWidth, n)
	}
}

func TestIndentColumns_TabStops(t *testing.T) {
	cases := map[string]int{
		"x":       0,
		"  x":     2,
		"\tx":     4,
		"  \tx":   4, // a tab advances to the next multiple of four, not by four
		"\t\tx":   8,
		"     x":  5,
		"\t \t x": 9,
		"":        0,
	}
	for in, want := range cases {
		if got := indentColumns(in); got != want {
			t.Errorf("indentColumns(%q) = %d, want %d", in, got, want)
		}
	}
}
