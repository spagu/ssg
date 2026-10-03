package apidoc

import (
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// handlers maps block tag names to the code that applies them.
var handlers = map[string]func(*parser, rawTag){
	"param":        (*parser).param,
	"arg":          (*parser).param,
	"argument":     (*parser).param,
	"returns":      (*parser).returns,
	"return":       (*parser).returns,
	"property":     (*parser).property,
	"prop":         (*parser).property,
	"typedef":      (*parser).typedefTag,
	"callback":     (*parser).callbackTag,
	"throws":       (*parser).throws,
	"exception":    (*parser).throws,
	"type":         (*parser).typeTag,
	"template":     (*parser).template,
	"typeParam":    (*parser).template,
	"typeparam":    (*parser).template,
	"example":      (*parser).example,
	"deprecated":   (*parser).deprecated,
	"since":        func(p *parser, t rawTag) { p.c.Doc.Since = strings.TrimSpace(t.text) },
	"see":          func(p *parser, t rawTag) { p.c.Doc.See = append(p.c.Doc.See, strings.TrimSpace(t.text)) },
	"default":      (*parser).defaultValue,
	"defaultValue": (*parser).defaultValue,
	"summary":      func(p *parser, t rawTag) { p.c.Doc.Summary = strings.TrimSpace(t.text) },
	"remarks":      (*parser).remarks,
	"public":       func(*parser, rawTag) {},
}

// modifiers maps modifier tags to the flag they set; their text is ignored.
var modifiers = map[string]func(*Modifiers){
	"internal":     func(m *Modifiers) { m.Internal = true },
	"private":      func(m *Modifiers) { m.Internal, m.Private = true, true },
	"hidden":       func(m *Modifiers) { m.Hidden = true },
	"ignore":       func(m *Modifiers) { m.Hidden = true },
	"beta":         func(m *Modifiers) { m.Stability = apimodel.Beta },
	"alpha":        func(m *Modifiers) { m.Stability = apimodel.Alpha },
	"experimental": func(m *Modifiers) { m.Stability = apimodel.Experimental },
	"readonly":     func(m *Modifiers) { m.Readonly = true },
	"abstract":     func(m *Modifiers) { m.Abstract = true },
	"override":     func(m *Modifiers) { m.Override = true },
}

// template handles @template and @typeParam.
func (p *parser) template(t rawTag) {
	p.c.Templates = append(p.c.Templates, parseTemplates(t.text)...)
}

// example handles @example; an empty example is dropped.
func (p *parser) example(t rawTag) {
	if ex := formatExample(t.text); ex != "" {
		p.c.Doc.Examples = append(p.c.Doc.Examples, ex)
	}
}

// deprecated handles @deprecated; an empty text still marks the symbol.
func (p *parser) deprecated(t rawTag) {
	text := strings.TrimSpace(t.text)
	p.c.Doc.Deprecated = &text
}

// defaultValue handles @default and @defaultValue.
func (p *parser) defaultValue(t rawTag) {
	p.c.Doc.Default = trimBlankLines(t.text)
}

// remarks handles @remarks: its text is appended to Doc.Body.
func (p *parser) remarks(t rawTag) {
	text := trimBlankLines(t.text)
	switch {
	case text == "":
		return
	case p.c.Doc.Body == "":
		p.c.Doc.Body = text
	default:
		p.c.Doc.Body += "\n\n" + text
	}
}
