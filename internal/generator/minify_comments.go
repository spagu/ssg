package generator

// Comment stripping that knows what a comment is (#106).
//
// This was a pair of regexes — `/\*[\s\S]*?\*/` and `^\s*//.*$` — which cannot
// tell a comment from the same characters inside a string. So
//
//	function f() { return "/*" + "x" + "*/"; }
//
// minified to `return "";`: the scan started at the `/*` in the first literal
// and ran to the `*/` in the third, taking the closing quote with it. That
// example still parses, which is the dangerous shape — the build reports
// success and the behaviour changes silently. On a real library it is louder:
// stylis' CSS-comment parser holds `/*` and `*/` in string literals, so a
// vendored mermaid bundle came out with an unterminated string and the browser
// refused the file. Nothing in the build noticed: minify, fingerprint and every
// check_* validator passed.
//
// The rule this scanner follows, wherever it is unsure: **keep the comment**.
// Leaving one behind costs bytes. Deleting something that was not a comment
// costs correctness, and does it silently.

import "strings"

// commentStyle selects how much lexical structure the scanner must respect.
type commentStyle int

const (
	// styleCSS: quoted strings only. `content: "/*"` is legal CSS.
	styleCSS commentStyle = iota
	// styleJS: strings, template literals (with ${} nesting) and regex literals.
	styleJS
)

// stripComments removes /* */ and (JS only) // comments, leaving every comment
// character that lives inside a string, template literal or regex alone.
//
// keepLines replaces a removed block comment with the newlines it spanned, so a
// line-level source map stays exact.
func stripComments(s string, style commentStyle, keepLines bool) string {
	var b strings.Builder
	b.Grow(len(s))

	// prev is the last significant byte written, used only to decide whether a
	// `/` can begin a regex literal.
	prev := byte(0)
	// templateDepth tracks `${ ... }` interpolations so a `}` closing one
	// returns to template-literal scanning rather than to code.
	var templateStack []int
	braceDepth := 0

	for i := 0; i < len(s); {
		c := s[i]

		switch {
		case c == '"' || c == '\'':
			j := scanQuoted(s, i)
			b.WriteString(s[i:j])
			prev = '"'
			i = j

		case style == styleJS && c == '`':
			j, interp := scanTemplate(s, i)
			b.WriteString(s[i:j])
			if interp {
				templateStack = append(templateStack, braceDepth)
				braceDepth++
			}
			prev = '`'
			i = j

		case style == styleJS && c == '}' && len(templateStack) > 0 && braceDepth == templateStack[len(templateStack)-1]+1:
			// Closing a ${...}: resume the template literal that opened it.
			braceDepth--
			templateStack = templateStack[:len(templateStack)-1]
			b.WriteByte(c)
			j, interp := scanTemplate(s, i)
			b.WriteString(s[i+1 : j])
			if interp {
				templateStack = append(templateStack, braceDepth)
				braceDepth++
			}
			prev = '`'
			i = j

		case c == '{':
			braceDepth++
			b.WriteByte(c)
			prev = c
			i++

		case c == '}':
			if braceDepth > 0 {
				braceDepth--
			}
			b.WriteByte(c)
			prev = c
			i++

		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				// Unterminated: not a comment we can safely remove, so keep the
				// rest verbatim rather than truncating the file.
				b.WriteString(s[i:])
				i = len(s)
				break
			}
			end := i + 2 + j + 2
			if keepLines {
				b.WriteString(strings.Repeat("\n", strings.Count(s[i:end], "\n")))
			}
			i = end

		case style == styleJS && c == '/' && i+1 < len(s) && s[i+1] == '/':
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				i = len(s)
				break
			}
			i += j // leave the newline for the writer below

		case style == styleJS && c == '/' && regexCanStart(prev):
			j := scanRegex(s, i)
			b.WriteString(s[i:j])
			prev = '/'
			i = j

		default:
			b.WriteByte(c)
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				prev = c
			}
			i++
		}
	}
	return b.String()
}

// scanQuoted returns the index just past a '...' or "..." literal, honouring
// backslash escapes. An unterminated literal consumes to end of line, which is
// where a real one would have failed to parse anyway.
func scanQuoted(s string, i int) int {
	quote := s[i]
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case quote:
			return j + 1
		case '\n':
			return j
		}
	}
	return len(s)
}

// scanTemplate returns the index just past a `...` literal, or just past the
// `${` that interrupts it — reported by interp, so the caller resumes code
// scanning inside the interpolation.
func scanTemplate(s string, i int) (end int, interp bool) {
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '`':
			return j + 1, false
		case '$':
			if j+1 < len(s) && s[j+1] == '{' {
				return j + 2, true
			}
		}
	}
	return len(s), false
}

// scanRegex returns the index just past a /.../flags literal. A `/` inside a
// character class does not close it, which is the case that matters here:
// /[/*]/ holds the characters that would otherwise open a comment.
func scanRegex(s string, i int) int {
	inClass := false
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '\n':
			// A regex literal cannot span lines, so this was division after all.
			// Returning one past the slash copies it and moves on, which cannot
			// delete anything.
			return i + 1
		case '/':
			if !inClass {
				for j++; j < len(s) && isRegexFlag(s[j]); j++ {
				}
				return j
			}
		}
	}
	return i + 1
}

// isRegexFlag reports whether c is one of the regex flag letters.
func isRegexFlag(c byte) bool {
	return strings.IndexByte("dgimsuvy", c) >= 0
}

// regexCanStart reports whether a `/` following prev begins a regex literal
// rather than a division.
//
// The distinction cannot be made without parsing, so this uses the usual
// heuristic: division follows a value, a regex follows an operator. Guessing
// "regex" when it was division copies the span verbatim — at worst a comment
// survives. That is the safe direction, so ambiguity resolves that way.
func regexCanStart(prev byte) bool {
	switch {
	case prev == 0:
		return true
	case prev >= 'a' && prev <= 'z', prev >= 'A' && prev <= 'Z',
		prev >= '0' && prev <= '9',
		prev == '_' || prev == '$':
		return false
	case prev == ')' || prev == ']' || prev == '"' || prev == '\'' || prev == '`':
		return false
	}
	return true
}
