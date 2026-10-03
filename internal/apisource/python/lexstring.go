package python

import "strings"

// stringPrefixes are the letter prefixes a string literal may carry, in
// lower case (Python accepts any case).
var stringPrefixes = map[string]bool{
	"r": true, "u": true, "b": true, "f": true, "t": true,
	"br": true, "rb": true, "fr": true, "rf": true, "tr": true, "rt": true,
}

// str scans a string literal whose prefix starts at start and whose quote
// is at lx.pos. An unterminated literal is a diagnostic; scanning resumes
// at the line break (single quotes) or the end of the file (triple).
func (lx *lexer) str(start int) {
	line := lx.line
	prefix := strings.ToLower(lx.src[start:lx.pos])
	q := lx.src[lx.pos]
	triple := strings.HasPrefix(lx.src[lx.pos:], strings.Repeat(string(q), 3))
	if !lx.scanString(q, triple, strings.ContainsAny(prefix, "ft")) {
		lx.diags = append(lx.diags, lexDiag{line, "unterminated string"})
	}
	lx.emit(tString, start, line)
}

// scanString moves past a string body from its opening quote and reports
// whether the closing quote was found. In an f-string, replacement fields
// are skipped as opaque, nested strings included.
func (lx *lexer) scanString(q byte, triple, format bool) bool {
	n := 1
	if triple {
		n = 3
	}
	closing := strings.Repeat(string(q), n)
	lx.pos += n
	for lx.pos < len(lx.src) {
		c := lx.src[lx.pos]
		switch {
		case c == '\\':
			lx.pos++
			if lx.atNewline(lx.pos) {
				lx.skipNewline()
			} else if lx.pos < len(lx.src) {
				lx.pos++
			}
		case strings.HasPrefix(lx.src[lx.pos:], closing):
			lx.pos += n
			return true
		case c == '\n' || c == '\r':
			if !triple {
				return false
			}
			lx.skipNewline()
		case format && c == '{' && strings.HasPrefix(lx.src[lx.pos:], "{{"):
			lx.pos += 2
		case format && c == '{':
			if !lx.scanField() {
				return false
			}
		default:
			lx.pos++
		}
	}
	return false
}

// scanField moves past an f-string replacement field from its "{" to the
// matching "}", stepping over nested brackets and strings.
func (lx *lexer) scanField() bool {
	depth := 0
	for lx.pos < len(lx.src) {
		c := lx.src[lx.pos]
		switch c {
		case '{', '[', '(':
			depth++
			lx.pos++
		case '}', ']', ')':
			depth--
			lx.pos++
			if depth == 0 {
				return true
			}
		case '"', '\'':
			triple := strings.HasPrefix(lx.src[lx.pos:], strings.Repeat(string(c), 3))
			if !lx.scanString(c, triple, false) {
				return false
			}
		case '\n', '\r':
			lx.skipNewline()
		default:
			lx.pos++
		}
	}
	return false
}

// decodeString returns the value of a string literal's source text: the
// prefix and quotes removed and, unless raw, the common escapes resolved.
// Implicitly concatenated literals are not joined here.
func decodeString(lit string) string {
	i := strings.IndexAny(lit, `"'`)
	if i < 0 {
		return lit
	}
	prefix, body := strings.ToLower(lit[:i]), lit[i:]
	n := 1
	if len(body) >= 6 && strings.HasPrefix(body, strings.Repeat(body[:1], 3)) {
		n = 3
	}
	if len(body) < 2*n {
		return strings.TrimLeft(body, body[:1])
	}
	body = strings.TrimSuffix(body[n:], strings.Repeat(body[:1], n))
	if strings.Contains(prefix, "r") {
		return body
	}
	return unescape(body)
}

// escapes are the escape sequences unescape resolves.
var escapes = map[byte]string{
	'\\': `\`, '\'': `'`, '"': `"`, 'n': "\n", 't': "\t", 'r': "\r", '\n': "", '0': "\x00",
}

// unescape resolves common backslash escapes; others are kept as written.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			if r, ok := escapes[s[i+1]]; ok {
				b.WriteString(r)
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
