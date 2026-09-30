package generator

import (
	"strings"
	"testing"
)

// TestAutolinkSkipsCodeBlocks is #305: the list item is linked, the same text
// inside a fenced or an indented code block is not.
func TestAutolinkSkipsCodeBlocks(t *testing.T) {
	links := map[string]string{"Documentation": "/docs/"}
	in := strings.Join([]string{
		"- Documentation",
		"",
		"```yaml",
		"- Documentation",
		"```",
		"",
		"~~~~",
		"- Documentation",
		"~~~",
		"- Documentation",
		"~~~~",
		"",
		"Text:",
		"",
		"    - Documentation",
		"",
		"* Documentation",
	}, "\n")
	got := strings.Split(autolinkListItems(in, links), "\n")
	linked := "[Documentation](/docs/)"
	for i, want := range map[int]bool{0: true, 3: false, 7: false, 9: false, 14: false, 16: true} {
		if has := strings.Contains(got[i], linked); has != want {
			t.Errorf("line %d %q: linked=%v, want %v", i, got[i], has, want)
		}
	}
}

// TestCodeBlockLinesInsideLists: in a list, four spaces of indentation is a
// nested item, not code, and a fence may itself be indented.
func TestCodeBlockLinesInsideLists(t *testing.T) {
	lines := []string{
		"1. First",            // 0
		"",                    // 1
		"    - Nested",        // 2 nested item: prose
		"",                    // 3
		"    ```go",           // 4 fence inside the list
		"    - not an item",   // 5
		"    ```",             // 6
		"Paragraph at margin", // 7 ends the list
		"",                    // 8
		"    code line",       // 9 indented code
		"",                    // 10
		"    still code",      // 11
		"back to text",        // 12
		"\tTabbed after text", // 13 no blank line before: not code
	}
	want := map[int]bool{0: false, 2: false, 4: true, 5: true, 6: true, 7: false, 9: true, 10: true, 11: true, 12: false, 13: false}
	got := codeBlockLines(lines)
	for i, w := range want {
		if got[i] != w {
			t.Errorf("line %d %q: code=%v, want %v", i, lines[i], got[i], w)
		}
	}
}

func TestFenceHelpers(t *testing.T) {
	if fenceRun("``") != "" || fenceRun("````js") != "````" || fenceRun("~~~") != "~~~" {
		t.Error("fenceRun")
	}
	if isClosingFence("```", "````") || !isClosingFence("`````", "````") || isClosingFence("~~~", "```") || isClosingFence("``` x", "```") {
		t.Error("isClosingFence")
	}
	if !isOrderedItem("12. x") || !isOrderedItem("3) x") || isOrderedItem("12") || isOrderedItem("x. y") {
		t.Error("isOrderedItem")
	}
}
