package generator

import "strings"

// codeBlockLines marks the Markdown lines that belong to a code block, so a
// line-based rewrite can leave them alone (#305). Fences (``` or ~~~) close
// only on the same character repeated at least as many times, as CommonMark
// says, and the fence lines themselves count as code.
//
// An indented code block is a run of lines indented four spaces (or a tab)
// that follows a blank line outside a list. Inside a list the same indentation
// is a nested item or a continuation paragraph, which is prose — so a list
// opened earlier keeps indented lines as text until a paragraph at the margin
// ends it.
func codeBlockLines(lines []string) []bool {
	code := make([]bool, len(lines))
	var fence string // the opening run while inside a fence, e.g. "````"
	inList, blankBefore, inIndented := false, true, false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			code[i] = true
			if isClosingFence(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if run := fenceRun(trimmed); run != "" && (inList || !indentedCode(line)) {
			code[i], fence, inIndented = true, run, false
			continue
		}
		switch {
		case trimmed == "":
			blankBefore = true
			code[i] = inIndented
			continue
		case indentedCode(line) && (inIndented || (blankBefore && !inList)):
			code[i], inIndented = true, true
		default:
			inIndented = false
			if extractListItemContent(line) != "" || isOrderedItem(trimmed) {
				inList = true
			} else if !indentedCode(line) && !strings.HasPrefix(line, " ") {
				inList = false
			}
		}
		blankBefore = false
	}
	return code
}

// fenceRun returns the backtick or tilde run that opens a fence ("```",
// "~~~~"), or "" when the line opens none.
func fenceRun(trimmed string) string {
	for _, c := range []byte{'`', '~'} {
		n := 0
		for n < len(trimmed) && trimmed[n] == c {
			n++
		}
		if n >= 3 {
			return trimmed[:n]
		}
	}
	return ""
}

// isClosingFence reports whether a line closes the fence opened by open: the
// same character, at least as many, and nothing after it.
func isClosingFence(trimmed, open string) bool {
	run := fenceRun(trimmed)
	return run != "" && run[0] == open[0] && len(run) >= len(open) && strings.TrimSpace(trimmed[len(run):]) == ""
}

// indentedCode reports a line indented enough to be code: four spaces or a tab.
func indentedCode(line string) bool {
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
}

// isOrderedItem reports an ordered list item ("1. x", "2) x").
func isOrderedItem(trimmed string) bool {
	n := 0
	for n < len(trimmed) && trimmed[n] >= '0' && trimmed[n] <= '9' {
		n++
	}
	return n > 0 && n < len(trimmed) && (trimmed[n] == '.' || trimmed[n] == ')')
}
