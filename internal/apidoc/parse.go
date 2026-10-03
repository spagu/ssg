package apidoc

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// Parse parses the raw text of a documentation comment: everything between
// "/**" and "*/" exclusive. Leading " * " gutters are stripped; indentation
// inside fenced code blocks is preserved. Never returns nil; Comment.Doc is
// never nil. Inline links are kept verbatim; see RewriteLinks.
func Parse(raw string) *Comment {
	desc, tags := splitSections(commentLines(raw))
	p := &parser{c: &Comment{Doc: describe(desc)}}
	for _, t := range tags {
		p.apply(t)
	}
	return p.c
}

// describe builds a Doc holding the summary and body of a description.
func describe(desc string) *apimodel.Doc {
	summary, body := splitDescription(desc)
	return &apimodel.Doc{Summary: summary, Body: body}
}

// scope says which declaration the @param, @returns and @property tags
// currently belong to.
type scope int

// The scopes a tag can belong to.
const (
	scopeComment  scope = iota // the documented symbol itself
	scopeTypedef               // the most recent @typedef
	scopeCallback              // the most recent @callback
)

// parser applies block tags to a Comment in the order written.
type parser struct {
	c     *Comment
	scope scope
}

// apply dispatches one block tag to its handler; tags the model has no
// field for are kept verbatim in Doc.Tags.
func (p *parser) apply(t rawTag) {
	if h, ok := handlers[t.name]; ok {
		h(p, t)
		return
	}
	if m, ok := modifiers[t.name]; ok {
		m(&p.c.Mods)
		return
	}
	p.keep(t)
}

// keep stores a tag verbatim in Doc.Tags.
func (p *parser) keep(t rawTag) {
	p.c.Doc.Tags = append(p.c.Doc.Tags, apimodel.Tag{Name: t.name, Text: trimBlankLines(t.text)})
}

// callback returns the most recent @callback block.
func (p *parser) callback() *Callback { return &p.c.Callbacks[len(p.c.Callbacks)-1] }

// typedef returns the most recent @typedef block.
func (p *parser) typedef() *Typedef { return &p.c.Typedefs[len(p.c.Typedefs)-1] }

// param handles @param, @arg and @argument.
func (p *parser) param(t rawTag) {
	pt := parseParam(t.text)
	if p.scope == scopeCallback {
		cb := p.callback()
		cb.Params = append(cb.Params, pt)
		return
	}
	p.c.Params = append(p.c.Params, pt)
}

// returns handles @returns and @return.
func (p *parser) returns(t rawTag) {
	tt := parseTypeTag(t.text)
	if p.scope == scopeCallback {
		cb := p.callback()
		cb.Returns, cb.Doc.Returns = &tt, tt.Text
		return
	}
	p.c.Returns, p.c.Doc.Returns = &tt, tt.Text
}

// property handles @property and @prop: a field of the current typedef.
// Outside a typedef the tag is kept verbatim.
func (p *parser) property(t rawTag) {
	if p.scope != scopeTypedef {
		p.keep(t)
		return
	}
	td := p.typedef()
	td.Properties = append(td.Properties, parseParam(t.text))
}

// typedefTag handles "@typedef {Type} Name description".
func (p *parser) typedefTag(t rawTag) {
	typ, rest := splitType(t.text)
	name, desc := firstWord(rest)
	p.c.Typedefs = append(p.c.Typedefs, Typedef{Name: name, Type: typ, Doc: describe(dedentTail(stripHyphen(desc)))})
	p.scope = scopeTypedef
}

// callbackTag handles "@callback Name description".
func (p *parser) callbackTag(t rawTag) {
	name, desc := firstWord(t.text)
	p.c.Callbacks = append(p.c.Callbacks, Callback{Name: name, Doc: describe(dedentTail(stripHyphen(desc)))})
	p.scope = scopeCallback
}

// throws handles @throws and @exception. Doc.Throws gets the text, or the
// type when there is no text.
func (p *parser) throws(t rawTag) {
	tt := parseTypeTag(t.text)
	p.c.Throws = append(p.c.Throws, tt)
	if s := firstNonEmpty(tt.Text, tt.Type); s != "" {
		p.c.Doc.Throws = append(p.c.Doc.Throws, s)
	}
}

// typeTag handles "@type {Expr}"; without braces the whole text is the type.
func (p *parser) typeTag(t rawTag) {
	typ, rest := splitType(t.text)
	p.c.Type = firstNonEmpty(typ, strings.TrimSpace(rest))
}

// firstNonEmpty returns the first of a and b that is not "".
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
