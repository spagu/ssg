package dts

import (
	"strings"

	"github.com/spagu/ssg/internal/apidoc"
	"github.com/spagu/ssg/internal/apimodel"
)

// commentInfo is a parsed documentation comment; a declaration without one
// has an empty comment, so callers need no nil checks.
type commentInfo struct {
	*apidoc.Comment
}

// parseComment parses the raw text of a /** */ comment.
func parseComment(raw string) *commentInfo {
	if strings.TrimSpace(raw) == "" {
		return &commentInfo{&apidoc.Comment{Doc: &apimodel.Doc{}}}
	}
	return &commentInfo{apidoc.Parse(raw)}
}

// comment returns the parsed comment of a declaration, parsing it once.
func (b *builder) comment(n *node) *commentInfo {
	c, ok := b.comments[n]
	if !ok {
		c = parseComment(n.doc)
		b.comments[n] = c
	}
	return c
}

// hidden reports whether a declaration stays out of the model: private in
// the code (a private member or #name) or in its comment (@private,
// @hidden, @ignore).
func (b *builder) hidden(n *node) bool {
	if n.mods.private || strings.HasPrefix(n.name, "#") {
		return true
	}
	c := b.comment(n)
	return c.Mods.Hidden || c.Mods.Private
}

// paramDoc returns the @param text for a parameter name.
func (c *commentInfo) paramDoc(name string) string {
	for _, p := range c.Params {
		if p.Name == name {
			return p.Text
		}
	}
	return ""
}

// templateDoc returns the @template text for a type parameter name.
func (c *commentInfo) templateDoc(name string) string {
	for _, t := range c.Templates {
		if t.Name == name {
			return t.Text
		}
	}
	return ""
}

// docOf returns the comment's Doc, or nil when it says nothing.
func docOf(c *commentInfo) *apimodel.Doc {
	if emptyDoc(c.Doc) {
		return nil
	}
	return c.Doc
}

// emptyDoc reports whether a Doc carries no text and no tag.
func emptyDoc(d *apimodel.Doc) bool {
	return d.Summary == "" && d.Body == "" && d.Returns == "" && len(d.Throws) == 0 &&
		len(d.Examples) == 0 && d.Deprecated == nil && d.Since == "" && len(d.See) == 0 &&
		d.Default == "" && len(d.Tags) == 0
}

// withTag returns a copy of d with a tag added, so the parsed comment
// shared by every listing of a declaration is left as it was.
func withTag(d *apimodel.Doc, name string) *apimodel.Doc {
	var cp apimodel.Doc
	if d != nil {
		cp = *d
	}
	cp.Tags = append(append([]apimodel.Tag{}, cp.Tags...), apimodel.Tag{Name: name})
	return &cp
}

// fileTags mark a comment as the overview of a whole file.
var fileTags = map[string]bool{"packageDocumentation": true, "module": true, "file": true, "fileoverview": true}

// moduleDoc parses a file's overview comment, without the tag that marked it.
func moduleDoc(raw string) *apimodel.Doc {
	d := parseComment(raw).Doc
	var tags []apimodel.Tag
	for _, t := range d.Tags {
		if !fileTags[t.Name] {
			tags = append(tags, t)
		}
	}
	d.Tags = tags
	if emptyDoc(d) {
		return nil
	}
	return d
}
