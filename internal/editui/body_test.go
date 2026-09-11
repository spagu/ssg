package editui

// Click-to-edit for the body of a page (GO-102, phase 2).

import (
	"strings"
	"testing"
)

const bodyDoc = `The first paragraph, with a [link](/x/) and some **bold**.

The second paragraph is different.

- one item
- another item

> A quotation.

## A heading here

` + "```go\nfunc main() {\n\n\tprintln(1)\n}\n```" + `

The last paragraph.
`

// TestSplitBlocksFindsWhatAPersonClicks.
func TestSplitBlocksFindsWhatAPersonClicks(t *testing.T) {
	blocks := SplitBlocks(bodyDoc)
	kinds := map[string]int{}
	for _, b := range blocks {
		kinds[b.Kind]++
	}
	if kinds["paragraph"] != 3 || kinds["list-item"] != 2 || kinds["quote"] != 1 ||
		kinds["heading"] != 1 || kinds["code"] != 1 {
		t.Errorf("kinds = %v", kinds)
	}
	// A block's source is the anchor a save is made against, so it must be
	// exactly what is in the file — no leading blank line.
	for _, b := range blocks {
		if b.Source != strings.Trim(b.Source, "\n") {
			t.Errorf("block source carries surrounding newlines: %q", b.Source)
		}
		if !strings.Contains(bodyDoc, b.Source) {
			t.Errorf("block source is not in the document: %q", b.Source)
		}
	}
	// A fenced block keeps its blank line rather than being cut in half.
	for _, b := range blocks {
		if b.Kind == "code" && !strings.Contains(b.Source, "println(1)") {
			t.Errorf("the fence was split: %q", b.Source)
		}
	}
}

// TestPlainTextMatchesWhatABrowserShows: the comparison that makes
// click-to-edit possible at all.
func TestPlainTextMatchesWhatABrowserShows(t *testing.T) {
	cases := map[string]string{
		"The first paragraph, with a [link](/x/) and some **bold**.": "The first paragraph, with a link and some bold.",
		"## A heading here":         "A heading here",
		"> A quotation.":            "A quotation.",
		"- one item":                "one item",
		"1. numbered":               "numbered",
		"Text with ![alt](/a.jpg).": "Text with .",
		"A line\nwrapped in source": "A line wrapped in source",
		"`code` and _em_":           "code and em",
	}
	for src, want := range cases {
		if got := plainText(src); got != want {
			t.Errorf("plainText(%q) = %q, want %q", src, got, want)
		}
	}
}

// TestNormalizeTextSurvivesTheBuild: the typographic passes rewrite punctuation
// on the way to the page, and the browser is reading the result.
func TestNormalizeTextSurvivesTheBuild(t *testing.T) {
	cases := map[string]string{
		"It’s here":       "It's here",
		"“quoted”":        `"quoted"`,
		"a – b — c":       "a - b - c",
		"and so…":         "and so...",
		"spaced\n\n  out": "spaced out",
		"non breaking":    "non breaking",
		"  trimmed  ":     "trimmed",
	}
	for in, want := range cases {
		if got := normalizeText(in); got != want {
			t.Errorf("normalizeText(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestFindBlockRefusesRatherThanGuesses: the whole safety property. A wrong
// silent save is the failure this exists to prevent.
func TestFindBlockRefusesRatherThanGuesses(t *testing.T) {
	got, err := FindBlock(bodyDoc, "The second paragraph is different.")
	if err != nil || got.Source != "The second paragraph is different." {
		t.Fatalf("got %+v, %v", got, err)
	}
	// A paragraph with markup is found by its rendered text and returns its
	// source, which is what the editor shows.
	got, err = FindBlock(bodyDoc, "The first paragraph, with a link and some bold.")
	if err != nil || !strings.Contains(got.Source, "**bold**") {
		t.Errorf("markup was lost: %+v, %v", got, err)
	}
	// Text the theme generated, not the file.
	if _, err := FindBlock(bodyDoc, "Read more"); err == nil ||
		!strings.Contains(err.Error(), "not a block of the source") {
		t.Errorf("got %v", err)
	}
	// The same sentence twice: a refusal with the count.
	twice := "Same sentence.\n\nSomething else.\n\nSame sentence.\n"
	if _, err := FindBlock(twice, "Same sentence."); err == nil || !strings.Contains(err.Error(), "2 times") {
		t.Errorf("got %v", err)
	}
	// Nothing to edit.
	if _, err := FindBlock(bodyDoc, "   "); err == nil {
		t.Error("empty text should be refused")
	}
}

// TestBlocksOfADocumentWithoutFences takes the fast path and still splits.
func TestBlocksOfADocumentWithoutFences(t *testing.T) {
	blocks := SplitBlocks("One.\n\nTwo.\n")
	if len(blocks) != 2 || blocks[1].Source != "Two." {
		t.Errorf("blocks = %+v", blocks)
	}
	if got := SplitBlocks("   \n\n  "); len(got) != 0 {
		t.Errorf("an empty body has no blocks: %+v", got)
	}
}

// TestListItemsAreSeparateBlocks, because that is what a reader clicks.
func TestListItemsAreSeparateBlocks(t *testing.T) {
	blocks := SplitBlocks("- one\n- two\n- three\n")
	if len(blocks) != 3 {
		t.Fatalf("blocks = %+v", blocks)
	}
	for i, want := range []string{"- one", "- two", "- three"} {
		if blocks[i].Source != want || blocks[i].Kind != "list-item" {
			t.Errorf("block %d = %+v", i, blocks[i])
		}
	}
	// A continuation line belongs to its item.
	wrapped := SplitBlocks("- one\n  continued\n- two\n")
	if len(wrapped) != 2 || !strings.Contains(wrapped[0].Source, "continued") {
		t.Errorf("wrapped = %+v", wrapped)
	}
	// A numbered list too.
	if got := SplitBlocks("1. one\n2. two\n"); len(got) != 2 {
		t.Errorf("numbered = %+v", got)
	}
}

// TestIsListLineDistinguishesMarkers: a heading and a quote are not list items,
// whatever their leading punctuation looks like.
func TestIsListLineDistinguishesMarkers(t *testing.T) {
	for _, yes := range []string{"- item", "* item", "+ item", "1. item", "  - indented"} {
		if !isListLine(yes) {
			t.Errorf("%q should be a list line", yes)
		}
	}
	for _, no := range []string{"", "   ", "# heading", "> quote", "plain text", "-no space"} {
		if isListLine(no) {
			t.Errorf("%q should not be a list line", no)
		}
	}
}

// TestAIActionPrompts: each button asks for the thing its label says.
func TestAIActionPrompts(t *testing.T) {
	actions := aiActions(160)
	if len(actions) != 4 {
		t.Fatalf("actions = %v", actions)
	}
	if got := actions["shorten"].Prompt("text", 160); !strings.Contains(got, "160 characters") {
		t.Errorf("shorten = %q", got)
	}
	if got := actions["title"].Prompt("text", 60); !strings.Contains(got, "specific to this text") {
		t.Errorf("title = %q", got)
	}
	if got := actions["alt"].Prompt("an office", 0); !strings.Contains(got, "screen reader") {
		t.Errorf("alt = %q", got)
	}
	if got := actions["translate"].Prompt("text", 0); !strings.Contains(got, "Markdown") {
		t.Errorf("translate = %q", got)
	}
	// The list the panel renders names every action, in a fixed order.
	list := aiActionList(160)
	if len(list) != 4 || list[0]["name"] != "shorten" || list[3]["name"] != "translate" {
		t.Errorf("list = %v", list)
	}
	if list[0]["limit"] != 160 {
		t.Errorf("the excerpt budget should reach the panel: %v", list[0])
	}
}
