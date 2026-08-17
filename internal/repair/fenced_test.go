package repair

// Markup swallowed by a code fence (#166). The reported page carried an opening
// fence in the middle of a <div> — a plugin's <template>, fenced by the exporter
// — and from that line on the document rendered as source, while `repair`
// reported the site clean because it only looked for indentation.
//
// Half of these tests are about what must NOT be touched. An author writing
// about HTML fences it on purpose, and a --fix that dedented those would be a
// worse defect than the one being fixed.

import (
	"strings"
	"testing"
)

// reportedPage is the shape from the issue: markup, an unclosed fence, more
// markup after it.
const reportedPage = `---
title: "Front"
---

<div class="elementor-section">
  <h2>Nasze realizacje</h2>
` + "```" + `
<template id="slider">
  <div class="slide">One</div>
</template>
<section class="elementor-section">
  <p>More.</p>
</section>
</div>`

// kinds returns the kinds of every finding, so a test states what was found
// rather than counting.
func kinds(findings []Finding) []Kind {
	out := make([]Kind, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.Kind)
	}
	return out
}

// TestScanFindsTheReportedFence: the page that shipped a grey box a screen and a
// half tall while repair called the site clean.
func TestScanFindsTheReportedFence(t *testing.T) {
	findings := Scan(reportedPage)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want the one fence", findings)
	}
	f := findings[0]
	if f.Kind != KindFenced {
		t.Errorf("kind = %v, want fenced", f.Kind)
	}
	// Line 7: three lines of front matter, a blank, the div, the h2, the fence.
	if f.Line != 7 {
		t.Errorf("line = %d, want 7 (the fence marker)", f.Line)
	}
	// The sample shows the markup that was swallowed, not the bare marker —
	// a report of "```" says nothing about what broke.
	if !strings.Contains(f.Sample, "<template") {
		t.Errorf("sample = %q, want the swallowed markup", f.Sample)
	}
}

// TestApplyDropsTheFenceAndKeepsTheMarkup: the fix is the same shape as the
// indented one — remove what swallowed the markup, leave the markup.
func TestApplyDropsTheFenceAndKeepsTheMarkup(t *testing.T) {
	got, changed := Apply(reportedPage)
	if changed == 0 {
		t.Fatal("the fence must be removed")
	}
	if strings.Contains(got, "```") {
		t.Errorf("the fence marker survived:\n%s", got)
	}
	for _, want := range []string{"<template id=\"slider\">", "</template>", "<section", "</div>"} {
		if !strings.Contains(got, want) {
			t.Errorf("the markup must survive, %q is gone:\n%s", want, got)
		}
	}
	// Front matter is untouched.
	if !strings.HasPrefix(got, "---\ntitle: \"Front\"\n---") {
		t.Errorf("front matter changed:\n%s", got)
	}
	// And the result is clean, so `repair --fix` twice is the same as once.
	if second := Scan(got); len(second) != 0 {
		t.Errorf("the fix must be complete: %+v", second)
	}
}

// TestFenceWithALanguageIsDeliberate: ```html is an author showing HTML. It is
// the commonest legitimate fence in any documentation site and must never be
// reported, whatever it holds.
func TestFenceWithALanguageIsDeliberate(t *testing.T) {
	doc := "---\ntitle: Docs\n---\n\n<p>Intro with real markup.</p>\n\n" +
		"```html\n<div class=\"card\">Hello</div>\n```\n\nDone.\n"
	if got := Scan(doc); len(got) != 0 {
		t.Fatalf("a named fence is deliberate: %+v", got)
	}
	if out, changed := Apply(doc); changed != 0 || out != doc {
		t.Errorf("a named fence must come back byte-identical (changed=%d)", changed)
	}
}

// TestBareFenceInProseIsLeftAlone: a bare fence holding markup, in a document
// with no other markup, is someone showing a tag in a sentence. The page-builder
// case is a document already made of divs — that is the discriminator.
func TestBareFenceInProseIsLeftAlone(t *testing.T) {
	doc := "---\ntitle: Prose\n---\n\nExample:\n\n```\n<b>bold</b>\n```\n\nDone.\n"
	if got := Scan(doc); len(got) != 0 {
		t.Fatalf("prose with one sample is not a broken export: %+v", got)
	}
}

// TestBareFenceInAMarkupDocumentIsReported: the same fence, in a page whose body
// is markup, is the export defect — and it is closed, so only this rule catches
// it.
func TestBareFenceInAMarkupDocumentIsReported(t *testing.T) {
	doc := "---\ntitle: Page\n---\n\n<div class=\"row\">\n  <p>Real markup.</p>\n</div>\n\n" +
		"```\n<template id=\"t\"><span>x</span></template>\n```\n\n<div>More markup.</div>\n"
	findings := Scan(doc)
	if len(findings) != 1 || findings[0].Kind != KindFenced {
		t.Fatalf("findings = %+v, want one fenced block", findings)
	}
	out, changed := Apply(doc)
	if changed == 0 || strings.Contains(out, "```") {
		t.Errorf("both markers must go:\n%s", out)
	}
	if !strings.Contains(out, "<template id=\"t\">") {
		t.Errorf("the markup must survive:\n%s", out)
	}
}

// TestUnclosedFenceOfProseIsNotOurs: an unclosed fence is broken Markdown
// whatever it holds, but repair only touches markup — rewriting a half-finished
// code sample would be guessing at content.
func TestUnclosedFenceOfProseIsNotOurs(t *testing.T) {
	doc := "---\ntitle: Snippet\n---\n\n```\nSELECT 1;\nnot closed\n"
	if got := Scan(doc); len(got) != 0 {
		t.Fatalf("no markup, not our business: %+v", got)
	}
}

// TestTildeFencesCountToo: ~~~ is a fence, and an exporter that emits it breaks
// a page exactly the same way.
func TestTildeFencesCountToo(t *testing.T) {
	doc := "---\nt: x\n---\n\n<div>a</div>\n\n~~~\n<span>b</span>\n"
	findings := Scan(doc)
	if len(findings) != 1 || findings[0].Kind != KindFenced {
		t.Fatalf("findings = %+v", findings)
	}
	if out, _ := Apply(doc); strings.Contains(out, "~~~") {
		t.Errorf("the tilde marker survived:\n%s", out)
	}
}

// TestIndentedAndFencedTogether: a page can carry both, and both are reported
// and fixed in one pass — the fence removal must not disturb the line numbers
// the dedent worked from.
func TestIndentedAndFencedTogether(t *testing.T) {
	doc := "---\nt: x\n---\n\n<div>open</div>\n\n" +
		"    <div class=\"indented\">soup</div>\n    <p>more</p>\n\n" +
		"```\n<template>fenced</template>\n"
	findings := Scan(doc)
	if len(findings) != 2 {
		t.Fatalf("findings = %+v, want one of each", findings)
	}
	seen := map[Kind]bool{}
	for _, k := range kinds(findings) {
		seen[k] = true
	}
	if !seen[KindIndented] || !seen[KindFenced] {
		t.Fatalf("both kinds must be reported: %v", kinds(findings))
	}

	out, changed := Apply(doc)
	if changed < 3 {
		t.Errorf("changed = %d, want the dedents and the marker", changed)
	}
	if strings.Contains(out, "    <div class=\"indented\"") {
		t.Errorf("the indented block was not dedented:\n%s", out)
	}
	if strings.Contains(out, "```") {
		t.Errorf("the fence marker survived:\n%s", out)
	}
	if rest := Scan(out); len(rest) != 0 {
		t.Errorf("one pass must be enough: %+v", rest)
	}
}

// TestKindString names the cause in the words a report uses.
func TestKindString(t *testing.T) {
	if got := KindFenced.String(); got != "fenced as code" {
		t.Errorf("KindFenced = %q", got)
	}
	// The zero value is the indented case, so a Finding built before fences were
	// understood still means what it did.
	var zero Kind
	if got := zero.String(); got != "indented as code" {
		t.Errorf("the zero Kind = %q", got)
	}
}

// TestFenceInfo reads the language off a marker, which is what separates a
// deliberate sample from a swallowed body.
func TestFenceInfo(t *testing.T) {
	cases := map[string]string{
		"```html":     "html",
		"~~~go":       "go",
		"``` html":    "html",
		"```":         "",
		"~~~":         "",
		"   ```json":  "json",
		"````":        "",
		"not a fence": "",
	}
	for in, want := range cases {
		if got := fenceInfo(in); got != want {
			t.Errorf("fenceInfo(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestEmptyAndTinyDocuments must not panic or invent findings.
func TestEmptyAndTinyDocuments(t *testing.T) {
	for _, doc := range []string{"", "\n", "---\n", "---\nt: x\n---\n", "```\n", "```\n```\n", "<div>a</div>\n"} {
		if got := Scan(doc); len(got) != 0 {
			t.Errorf("Scan(%q) = %+v, want nothing", doc, got)
		}
		if out, changed := Apply(doc); changed != 0 || out != doc {
			t.Errorf("Apply(%q) changed %d line(s)", doc, changed)
		}
	}
}
