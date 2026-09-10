package editui

// Editing a document's frontmatter without touching its body (GO-102).

import (
	"strings"
	"testing"
)

const doc = `---
title: Hello World   # the headline
slug: hello
status: publish
type: post
date: 2026-01-15
tags: [go, ssg]

description: A first post
---

The body, which has --- in it and must survive.

## A heading
`

// TestSetFrontmatterLeavesEverythingElseAlone: the body, the comments and the
// blank line inside the frontmatter all come back unchanged.
func TestSetFrontmatterLeavesEverythingElseAlone(t *testing.T) {
	out, err := setFrontmatter(doc, "title", "Hello, edited")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "title: Hello, edited") {
		t.Errorf("value not written:\n%s", out)
	}
	if !strings.Contains(out, "# the headline") {
		t.Errorf("the key's comment was lost:\n%s", out)
	}
	if !strings.Contains(out, "The body, which has --- in it and must survive.") {
		t.Errorf("the body was damaged:\n%s", out)
	}
	if !strings.Contains(out, "tags: [go, ssg]\n\ndescription:") {
		t.Errorf("the blank line inside the frontmatter was lost:\n%s", out)
	}
	before, after := strings.Split(doc, "\n"), strings.Split(out, "\n")
	if len(before) != len(after) {
		t.Fatalf("line count changed: %d → %d", len(before), len(after))
	}
	changed := 0
	for i := range before {
		if before[i] != after[i] {
			changed++
		}
	}
	if changed != 1 {
		t.Errorf("%d lines changed, want 1", changed)
	}
}

// TestSetFrontmatterKeepsAListInline: a list written as [a, b] is edited as
// [a, b, c] rather than being rewritten as a block.
func TestSetFrontmatterKeepsAListInline(t *testing.T) {
	out, err := setFrontmatter(doc, "tags", []interface{}{"go", "ssg", "editing"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "tags: [go, ssg, editing]") {
		t.Errorf("the list style changed:\n%s", out)
	}
}

// TestSetFrontmatterAddsAKey that the document did not have.
func TestSetFrontmatterAddsAKey(t *testing.T) {
	out, err := setFrontmatter(doc, "sticky", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sticky: true") {
		t.Errorf("new key not written:\n%s", out)
	}
	if !strings.Contains(out, "The body") {
		t.Errorf("the body was damaged:\n%s", out)
	}
	if strings.Count(out, "---") != strings.Count(doc, "---") {
		t.Errorf("the fences moved:\n%s", out)
	}
}

// TestUnsetFrontmatter removes a key, and says so when there is none.
func TestUnsetFrontmatter(t *testing.T) {
	out, err := unsetFrontmatter(doc, "description")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "description:") {
		t.Errorf("key not removed:\n%s", out)
	}
	if !strings.Contains(out, "The body") {
		t.Error("the body was damaged")
	}
	if _, err := unsetFrontmatter(doc, "nothing_here"); err == nil {
		t.Error("removing what is not there must be an error")
	}
}

// TestDocumentsWithoutFrontmatter: a bare Markdown file has nothing to edit,
// and says so rather than growing a frontmatter block nobody asked for.
func TestDocumentsWithoutFrontmatter(t *testing.T) {
	bare := "# Just a heading\n\nAnd a paragraph.\n"
	if _, err := setFrontmatter(bare, "title", "x"); err == nil {
		t.Error("a file with no frontmatter cannot take a frontmatter edit")
	}
	if _, err := unsetFrontmatter(bare, "title"); err == nil {
		t.Error("same for unset")
	}
	fm, err := readFrontmatter(bare)
	if err != nil || len(fm) != 0 {
		t.Errorf("readFrontmatter = %v, %v", fm, err)
	}
	d, err := splitDocument(bare)
	if err != nil || !d.bare || d.join() != bare {
		t.Errorf("a bare document must round-trip: %q", d.join())
	}
}

// TestUnclosedFrontmatterIsAnError: the same rule the parser applies, so the
// editor cannot save a file the build would refuse.
func TestUnclosedFrontmatterIsAnError(t *testing.T) {
	if _, err := splitDocument("---\ntitle: x\n\nno closing fence\n"); err == nil {
		t.Error("an unclosed frontmatter must be an error")
	}
	if _, err := setFrontmatter("---\ntitle: x\n", "title", "y"); err == nil {
		t.Error("set must refuse it too")
	}
	if _, err := unsetFrontmatter("---\ntitle: x\n", "title"); err == nil {
		t.Error("unset must refuse it too")
	}
	if _, err := readFrontmatter("---\ntitle: x\n"); err == nil {
		t.Error("read must refuse it too")
	}
}

// TestSplitDocumentTolerances: Windows line endings and leading blank lines,
// both of which real files have.
func TestSplitDocumentTolerances(t *testing.T) {
	d, err := splitDocument("\n\n---\r\ntitle: x\r\n---\r\nBody\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if d.lead != "\n\n" {
		t.Errorf("lead = %q", d.lead)
	}
	if strings.TrimSpace(d.frontmatter) != "title: x" {
		t.Errorf("frontmatter = %q", d.frontmatter)
	}
	if strings.TrimSpace(d.body) != "Body" {
		t.Errorf("body = %q", d.body)
	}
	if !strings.HasPrefix(d.join(), "\n\n---\n") {
		t.Errorf("join = %q", d.join())
	}
}

// TestReadFrontmatterValues: what the form fills its controls from.
func TestReadFrontmatterValues(t *testing.T) {
	fm, err := readFrontmatter(doc)
	if err != nil {
		t.Fatal(err)
	}
	if fm["title"] != "Hello World" || fm["type"] != "post" {
		t.Errorf("frontmatter = %v", fm)
	}
	if tags, ok := fm["tags"].([]interface{}); !ok || len(tags) != 2 {
		t.Errorf("tags = %#v", fm["tags"])
	}
	if _, err := readFrontmatter("---\n: : :\n---\n"); err == nil {
		t.Error("unparsable frontmatter must be reported")
	}
}

// TestSetFrontmatterReportsAnImpossiblePath rather than writing something else.
func TestSetFrontmatterReportsAnImpossiblePath(t *testing.T) {
	if _, err := setFrontmatter(doc, "title.deeper", "x"); err == nil {
		t.Error("a path through a scalar must be reported")
	}
	if _, err := unsetFrontmatter(doc, "title.deeper"); err == nil {
		t.Error("same for unset")
	}
}
