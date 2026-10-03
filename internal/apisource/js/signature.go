package js

import (
	"strings"

	"github.com/spagu/ssg/internal/apidoc"
	"github.com/spagu/ssg/internal/apimodel"
	"github.com/tdewolff/parse/v2/js"
)

// signature builds a call signature from the parameters as written and the
// @param, @returns and @template tags of the comment. A parameter without a
// tag has no type; a default value in the code makes it optional.
func (b *builder) signature(params js.Params, c *apidoc.Comment, at where) *apimodel.Signature {
	b = b.withTemplates(c.Templates)
	sig := &apimodel.Signature{TypeParams: b.typeParams(c.Templates, at)}
	tags := topLevelTags(c.Params)
	for i, el := range params.List {
		sig.Params = append(sig.Params, b.param(el.Binding, el.Default, tags, i, at))
	}
	if params.Rest != nil {
		p := b.param(params.Rest, nil, tags, len(params.List), at)
		p.Rest = true
		sig.Params = append(sig.Params, p)
	}
	if c.Returns != nil {
		sig.Returns = b.typeOf(c.Returns.Type, at)
	}
	return sig
}

// param builds one parameter. A plain name finds its tag by name; a
// destructuring pattern takes the tag at its position, and that tag's name.
func (b *builder) param(bind js.IBinding, def js.IExpr, tags []apidoc.ParamTag, i int, at where) *apimodel.Param {
	name := nodeText(bind)
	tag, ok := tagByName(tags, name)
	if _, plain := bind.(*js.Var); !ok && !plain && i < len(tags) {
		tag, ok, name = tags[i], true, tags[i].Name
	}
	p := &apimodel.Param{Name: name}
	if def != nil {
		p.Default, p.Optional = nodeText(def), true
	}
	if !ok {
		return p
	}
	p.Type = b.typeOf(tag.Type, at)
	p.Optional = p.Optional || tag.Optional
	p.Rest = tag.Rest
	if p.Default == "" {
		p.Default = tag.Default
	}
	p.Doc = tag.Text
	return p
}

// tagParam builds a parameter or field from a tag alone, for callbacks and
// typedef properties.
func (b *builder) tagParam(t apidoc.ParamTag, at where) *apimodel.Param {
	return &apimodel.Param{
		Name: t.Name, Type: b.typeOf(t.Type, at), Optional: t.Optional,
		Rest: t.Rest, Default: t.Default, Doc: t.Text,
	}
}

// tagByName returns the tag documenting a parameter name.
func tagByName(tags []apidoc.ParamTag, name string) (apidoc.ParamTag, bool) {
	for _, t := range tags {
		if t.Name == name {
			return t, true
		}
	}
	return apidoc.ParamTag{}, false
}

// topLevelTags drops the tags of nested object fields ("opts.strict").
func topLevelTags(tags []apidoc.ParamTag) []apidoc.ParamTag {
	out := make([]apidoc.ParamTag, 0, len(tags))
	for _, t := range tags {
		if !strings.Contains(t.Name, ".") {
			out = append(out, t)
		}
	}
	return out
}
