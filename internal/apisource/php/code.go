package php

import "strings"

// code renders tokens [from, to) as the declaration was written, with
// attributes and comments left out and whitespace collapsed to one space:
// "public static function parse(string $src, array $opts = []): Doc".
// A trailing comma before ")" is dropped.
func (p *parser) code(from, to int) string {
	var b strings.Builder
	var prev *token
	for i := from; i < to && i < len(p.toks); i++ {
		t := &p.toks[i]
		if t.kind == tokAttr || t.kind == tokDoc {
			continue
		}
		if t.is(",") && i+1 < to && p.toks[i+1].is(")") {
			continue
		}
		if prev != nil && p.gap(prev, t) {
			b.WriteByte(' ')
		}
		b.WriteString(t.text)
		prev = t
	}
	return b.String()
}

// gap reports whether a space separates two tokens in rendered code:
// anything but adjacency in the source, except just inside brackets and
// before a comma.
func (p *parser) gap(prev, t *token) bool {
	if prev.is("(") || prev.is("[") || t.is(")") || t.is("]") || t.is(",") {
		return false
	}
	return prev.end < t.start
}

// typeText joins the tokens of a type, which PHP writes without spaces
// that matter: "?int", "A|B", "(A&B)|null".
func (p *parser) typeText(from, to int) string {
	var b strings.Builder
	for i := from; i < to; i++ {
		if p.toks[i].kind != tokAttr && p.toks[i].kind != tokDoc {
			b.WriteString(p.toks[i].text)
		}
	}
	return b.String()
}
