package mcp

// Anchored partial edits (#187).
//
// The write contract is full-file replacement, which is safe — nothing fuzzy,
// nothing half-applied — but it prices an edit by the size of the file rather
// than the size of the change. Measured on a migrated site, moving one CSS
// value cost a read, a whole-file write and a verifying read: ~4 300 tokens of
// file traffic for one line, and the same shape repeats for every tweak.
//
// The anchor keeps the safety and drops the cost. `old` must appear exactly
// once: zero matches or several is a refusal that names the count, so a model
// never guesses which occurrence was meant and nothing lands partially applied.
// The reply carries the change in context, which is also the
// verification — there is no second read.

import (
	"fmt"
	"os"
	"strings"
)

// editContextLines is how many lines either side of the change come back. Enough
// to recognise the place, small enough that the reply stays a fraction of the
// file.
const editContextLines = 3

// anchoredEdit is one exact-unique-match replacement.
type anchoredEdit struct {
	old, new string
}

// apply performs the replacement, returning the new content, the 1-based line
// the change starts on and its byte offset within that line. It refuses
// anything it cannot place unambiguously.
//
// The offset is what lets the report window a long line: on a minified
// stylesheet the changed line is the whole file, and "the changed line in
// context" was 7k tokens to move one hex value (#204).
func (e anchoredEdit) apply(content string) (string, int, int, error) {
	if e.old == "" {
		return "", 0, 0, fmt.Errorf("`old` is required — the exact text to replace (use designer_write/content_update to replace a whole file)")
	}
	switch n := strings.Count(content, e.old); {
	case n == 0:
		return "", 0, 0, fmt.Errorf("`old` does not appear in the file — read it and copy the exact text, whitespace included")
	case n > 1:
		return "", 0, 0, fmt.Errorf("`old` appears %d times — include enough surrounding text to make it unique, "+
			"or use a whole-file write if every occurrence should change", n)
	}
	at := strings.Index(content, e.old)
	lineStart := strings.LastIndexByte(content[:at], '\n') + 1
	return content[:at] + e.new + content[at+len(e.old):],
		1 + strings.Count(content[:at], "\n"), at - lineStart, nil
}

// editReport renders the changed region so the reply proves what landed instead
// of requiring a second read to find out.
//
// By line where lines are a sensible size, and by character where they are not:
// a minified file has one line, so printing "the changed line" printed the file
// (#204).
func editReport(content string, startLine, col, newLines, newLen int) string {
	lines := strings.Split(content, "\n")
	if startLine <= len(lines) && isLongLine(lines[startLine-1]) {
		frag, fromCol, toCol, _ := charWindow(lines[startLine-1], col, col+newLen)
		return capFragment(fmt.Sprintf("→ %4d:%d-%d | %s", startLine, fromCol, toCol, frag))
	}

	from := max(1, startLine-editContextLines)
	to := min(len(lines), startLine+newLines-1+editContextLines)

	var b strings.Builder
	for i := from; i <= to; i++ {
		marker := "  "
		if i >= startLine && i < startLine+newLines {
			marker = "→ "
		}
		fmt.Fprintf(&b, "%s%4d | %s\n", marker, i, trimLine(lines[i-1]))
	}
	return capFragment(strings.TrimRight(b.String(), "\n"))
}

// lineSpan is how many lines a replacement occupies once written.
func lineSpan(s string) int { return 1 + strings.Count(s, "\n") }

// runEdit is the body both edit tools share: resolve, read, replace the single
// occurrence, write, rebuild, and report the change in context.
func (s *Server) runEdit(args map[string]any, resolve func(string) (string, error), what string) toolResult {
	rel, _ := strArg(args, "path")
	oldText, ok := rawArg(args, "old")
	if !ok {
		return errResult("`old` is required (the exact text to replace)")
	}
	newText, ok := rawArg(args, "new")
	if !ok {
		return errResult("`new` is required (the replacement; pass \"\" to delete the anchored text)")
	}
	abs, err := resolve(rel)
	if err != nil {
		return errResult(err.Error())
	}
	before, err := readFile(abs)
	if err != nil {
		return errResult("read failed: " + err.Error())
	}
	after, line, col, err := anchoredEdit{old: oldText, new: newText}.apply(before)
	if err != nil {
		return errResult(fmt.Sprintf("%s: %v", rel, err))
	}
	if err := writeFile(abs, after); err != nil {
		return errResult("write failed: " + err.Error())
	}
	summary := fmt.Sprintf("%s edited %s at line %d\n\n%s",
		what, rel, line, editReport(after, line, col, lineSpan(newText), len(newText)))
	return s.afterMutate(summary)
}

// editSchema is the argument shape both edit tools take.
func editSchema(pathDesc string) map[string]any {
	return objectSchema(map[string]any{
		"path": stringProp(pathDesc),
		"old": stringProp("The exact existing text to replace, copied byte for byte including indentation. " +
			"Must appear EXACTLY ONCE in the file — if it appears more than once, include more " +
			"surrounding lines until it is unique."),
		"new": stringProp("The replacement text. Pass an empty string to delete the anchored text."),
	}, "path", "old", "new")
}

// readFile reads a file the section already resolved.
func readFile(abs string) (string, error) {
	b, err := os.ReadFile(abs) // #nosec G304 -- confined to the section's directories by resolveIn
	if err != nil {
		return "", err
	}
	return string(b), nil
}
