package dts

import "strings"

// Stop sets for the type positions of a declaration.
var (
	stopMember = []string{";", ",", "="}          // property, index signature, return of a member
	stopParam  = []string{",", "="}               // parameter type; ")" closes
	stopAlias  = []string{";"}                    // type alias right-hand side
	stopTParam = []string{",", "="}               // constraint; ">" closes
	stopHerit  = []string{",", "{", "implements"} // extends and implements clauses
)

// typeSpan consumes one type and returns its text on one line without
// comments. It stops before a token of stop outside brackets, before a
// closing bracket it did not open, and where endsBefore says the type is
// complete (a member list may leave out its semicolons; a method may have a
// body). An empty or unclosed type is a syntax error.
func (p *parser) typeSpan(stop []string) string {
	from := p.pos
	var open []string // closing brackets expected, innermost last
	for t := p.peek(); t.kind != tokEOF; t = p.peek() {
		if len(open) == 0 && (stops(t, stop) || p.pos > from && endsBefore(p.toks[p.pos-1], t)) {
			break
		}
		if t.is(";") && len(open) > 0 && open[len(open)-1] != "}" {
			p.fail("unclosed bracket in a type before \";\"")
		}
		if i := strings.Index("([{<", t.text); t.kind == tokPunct && i >= 0 && len(t.text) == 1 {
			open = append(open, ")]}>"[i:i+1])
		}
		if t.kind == tokPunct && len(t.text) == 1 && strings.Contains(")]}>", t.text) {
			if len(open) == 0 {
				break
			}
			if open[len(open)-1] != t.text {
				p.fail("unbalanced %q in a type", t.text)
			}
			open = open[:len(open)-1]
		}
		p.advance()
	}
	if p.pos == from || len(open) > 0 {
		p.fail("expected a type, found %s", describe(p.peek()))
	}
	return p.tokenText(from, p.pos)
}

// tokenText joins the tokens from..to (end exclusive) as written, with one
// space wherever white space or a comment separated two of them; strings
// keep their spacing and comments inside a type are dropped.
func (p *parser) tokenText(from, to int) string {
	var b strings.Builder
	for i := from; i < to; i++ {
		if i > from && p.toks[i].start > p.toks[i-1].end {
			b.WriteByte(' ')
		}
		b.WriteString(p.toks[i].text)
	}
	return b.String()
}

// stops reports whether t is one of the stop tokens.
func stops(t token, stop []string) bool {
	if t.kind != tokPunct && t.kind != tokIdent {
		return false
	}
	for _, s := range stop {
		if t.text == s {
			return true
		}
	}
	return false
}

// endsBefore reports whether a complete type ends between prev and next:
// at a line break that ends the type, or before a "{" that opens a body
// rather than an object type (an object type never follows a type).
func endsBefore(prev, next token) bool {
	return lineEndsType(prev, next) || next.is("{") && closesType(prev)
}

// lineEndsType reports whether a line break between prev and next ends the
// type: prev can close a type and next cannot continue one.
func lineEndsType(prev, next token) bool {
	return next.nl && closesType(prev) && !continuesType(next)
}

// closesType reports whether a type may end with t.
func closesType(t token) bool {
	switch t.kind {
	case tokString, tokNumber, tokTemplate:
		return true
	case tokIdent:
		return !typeOperators[t.text]
	}
	return t.is(")") || t.is("]") || t.is("}") || t.is(">")
}

// continuesType reports whether a line starting with t carries on the type
// before it: a union or intersection bar, a member access, a conditional.
func continuesType(t token) bool {
	for _, s := range []string{"|", "&", ".", "?", ":", "=>", "extends", "is"} {
		if t.is(s) {
			return true
		}
	}
	return false
}

// typeOperators are words after which a type cannot end.
var typeOperators = map[string]bool{
	"keyof": true, "typeof": true, "infer": true, "extends": true, "is": true,
	"asserts": true, "new": true, "readonly": true, "unique": true, "abstract": true,
}
