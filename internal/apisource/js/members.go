package js

import "strings"

// memberPrefix are the modifiers that may precede a class member's name.
var memberPrefix = map[string]bool{"static": true, "async": true, "get": true, "set": true, "*": true}

// memberBoundary are the tokens after which a member (or an object literal
// property) can start.
var memberBoundary = map[string]bool{"{": true, "}": true, ";": true, ",": true,
	"static": true, "async": true, "get": true, "set": true, "*": true}

// memberCursor finds the members of a class body or object literal in
// source order, which is the order the syntax tree lists them in.
type memberCursor struct {
	s     *scan
	next  int // first token not yet searched
	end   int // index of the closing "}"
	depth int // depth of the members' tokens
}

// membersAfter returns a cursor over the body of the first "{" at or after
// token from, at the depth of token from. The keyword names what must come
// first ("class"), or is "" for an object literal right at from.
func (s *scan) membersAfter(from int, keyword string) *memberCursor {
	if from < 0 || from >= len(s.toks) {
		return &memberCursor{s: s}
	}
	depth := s.toks[from].depth
	i := from
	for keyword != "" && i < len(s.toks) && s.toks[i].text != keyword {
		i++
	}
	for i < len(s.toks) && (s.toks[i].text != "{" || s.toks[i].depth != depth) {
		i++
	}
	if i >= len(s.toks) {
		return &memberCursor{s: s}
	}
	return &memberCursor{s: s, next: i + 1, end: s.closeOf(i), depth: depth + 1}
}

// closeOf returns the index of the "}" closing the "{" at open.
func (s *scan) closeOf(open int) int {
	for i := open + 1; i < len(s.toks); i++ {
		if s.toks[i].depth == s.toks[open].depth && s.toks[i].text == "}" {
			return i
		}
	}
	return len(s.toks)
}

// find returns the start token of the next member called name, or -1.
func (c *memberCursor) find(name string) int {
	for i := c.next; i < c.end; i++ {
		t := c.s.toks[i]
		if t.depth != c.depth || unquote(t.text) != name || !c.boundary(i) {
			continue
		}
		c.next = i + 1
		start := i
		for start > 0 && memberPrefix[c.s.toks[start-1].text] && c.s.toks[start-1].depth == c.depth {
			start--
		}
		return start
	}
	return -1
}

// boundary reports whether a member can start at token i: after a
// separator or modifier, or on a new line after a complete operand.
func (c *memberCursor) boundary(i int) bool {
	prev := c.s.toks[i-1]
	if memberBoundary[prev.text] {
		return true
	}
	return c.s.toks[i].nl && (isIdent(prev.text) || strings.ContainsAny(prev.text[:1], `)]"'0123456789`))
}

// unquote strips the quotes of a string-literal property name.
func unquote(text string) string {
	if len(text) >= 2 && (text[0] == '"' || text[0] == '\'') && text[len(text)-1] == text[0] {
		return text[1 : len(text)-1]
	}
	return text
}
