package apipage

import (
	"fmt"
	"html"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// writer accumulates one page's Markdown.
type writer struct {
	b      strings.Builder
	opts   *Options
	urls   *URLs
	module string // the page's module ID, for link resolution
}

// typeText is a type as the package's language writes it. Extractors for Go
// and PHP keep the source text in the name; Python's type arguments go in
// brackets (list[Token]), where TypeScript uses angle brackets.
func (w *writer) typeText(t *apimodel.TypeRef) string {
	if w.opts.language != "python" || t == nil {
		return t.String()
	}
	switch t.Kind {
	case apimodel.TypeName:
		if len(t.Args) == 0 {
			return t.Name
		}
		return t.Name + "[" + w.typeList(t.Args, ", ") + "]"
	case apimodel.TypeUnion:
		return w.typeList(t.Args, " | ")
	case apimodel.TypeTuple:
		return "tuple[" + w.typeList(t.Args, ", ") + "]"
	case apimodel.TypeArray:
		return "list[" + w.typeList(t.Args, ", ") + "]"
	}
	return t.String()
}

// typeList renders types with a separator, in the package's language.
func (w *writer) typeList(ts []*apimodel.TypeRef, sep string) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = w.typeText(t)
	}
	return strings.Join(parts, sep)
}

// fence is the code-block language of declarations: the package's own, or
// TypeScript for JavaScript and TypeScript packages.
func (w *writer) fence() string {
	switch w.opts.language {
	case "go", "php", "python":
		return w.opts.language
	}
	return "ts"
}

// links rewrites inline links as written in the page's module.
func (w *writer) links(md string) string { return w.opts.links(md, w.module) }

func (w *writer) line(format string, args ...any) {
	fmt.Fprintf(&w.b, format, args...)
	w.b.WriteByte('\n')
}

// text writes documentation Markdown, with inline links rewritten.
func (w *writer) text(md string) {
	if md = strings.TrimSpace(md); md != "" {
		w.line("%s\n", w.links(md))
	}
}

// doc writes a symbol's documentation in full.
func (w *writer) doc(d *apimodel.Doc) {
	w.docIntro(d)
	w.docOutro(d)
}

// docIntro is what a reader needs before the signature: deprecation,
// summary and body.
func (w *writer) docIntro(d *apimodel.Doc) {
	if d == nil {
		return
	}
	if d.Deprecated != nil {
		w.line("> **Deprecated.** %s\n", w.links(*d.Deprecated))
	}
	w.text(d.Summary)
	w.text(d.Body)
}

// docOutro is what comes after it: since, examples, see also.
func (w *writer) docOutro(d *apimodel.Doc) {
	if d == nil {
		return
	}
	if d.Since != "" {
		w.line("*Since %s.*\n", d.Since)
	}
	for _, ex := range d.Examples {
		w.line("**Example**\n")
		w.example(ex)
	}
	for _, see := range d.See {
		w.line("See also: %s\n", w.links(see))
	}
}

// example writes one @example. With a playground configured, a JavaScript
// code block is wrapped in the element the browser script turns into an
// editor; the blank lines around it keep the fence Markdown, not raw HTML.
func (w *writer) example(ex string) {
	if w.opts.Playground == "" || !runnable(ex) {
		w.text(ex)
		return
	}
	w.line(`<div class="ssg-playground" data-ssg-playground data-package="%s" data-module="%s">`+"\n",
		html.EscapeString(w.opts.pkgName), html.EscapeString(w.opts.Playground))
	w.text(ex)
	w.line("</div>\n")
}

// runnable reports whether an example is a single JavaScript code block:
// the only kind the playground can run.
func runnable(ex string) bool {
	ex = strings.TrimSpace(ex)
	first, _, _ := strings.Cut(ex, "\n")
	switch strings.TrimSpace(strings.TrimPrefix(first, "```")) {
	case "js", "javascript", "mjs":
	default:
		return false
	}
	return strings.HasPrefix(first, "```") && strings.HasSuffix(ex, "```") && strings.Count(ex, "```") == 2
}

// signature writes one callable shape: the declaration, parameters, returns
// and throws. A constructor returns nothing worth saying.
func (w *writer) signature(name string, sig *apimodel.Signature) {
	decl := sig.Code
	if decl == "" {
		decl = sig.Declaration(name)
		if name == "constructor" && sig.Returns == nil {
			decl = strings.TrimSuffix(decl, ": unknown")
		}
	}
	w.line("```%s\n%s\n```\n", w.fence(), decl)
	w.doc(sig.Doc)
	if len(sig.Params) > 0 {
		w.line("| Parameter | Type | Description |\n|---|---|---|")
		for _, p := range sig.Params {
			w.line("| `%s` | `%s` | %s |", paramLabel(p), cell(w.typeText(p.Type)), cell(w.links(p.Doc)))
		}
		w.line("")
	}
	if sig.Returns != nil {
		w.line("**Returns** `%s`\n", w.typeText(sig.Returns))
	}
}

// paramLabel is "name", "name?", "...name" or "name = default".
func paramLabel(p *apimodel.Param) string {
	s := p.Name
	if p.Rest {
		s = "..." + s
	}
	if p.Optional && p.Default == "" {
		s += "?"
	}
	if p.Default != "" {
		s += " = " + p.Default
	}
	return s
}

// cell makes text safe inside a Markdown table cell.
func cell(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	return strings.ReplaceAll(s, "|", `\|`)
}

// heading writes a symbol's heading as HTML with its anchor as the id. A
// Markdown heading would get an id from the renderer's own rules — lower
// case, de-duplicated with -1 — and a link to #Token would land nowhere.
func (w *writer) heading(level int, id, title string) {
	w.line("<h%d id=\"%s\">%s</h%d>\n", level, w.urls.Anchor(id), title, level)
}

// section writes a group heading ("Functions"). Its id carries a prefix so it
// can never take the anchor of a symbol with the same name — a constructor,
// a function called "types".
func (w *writer) section(title string) {
	w.line("<h2 id=\"kind-%s\">%s</h2>\n", strings.ToLower(title), title)
}

// symbol writes one symbol in full, at heading level.
func (w *writer) symbol(level int, s *apimodel.Symbol) {
	w.heading(level, s.ID, symbolTitle(s))
	w.badges(s)
	w.docIntro(s.Doc)
	switch {
	case len(s.Signatures) > 0:
		for _, sig := range s.Signatures {
			w.signature(s.Name, sig)
		}
	case s.Code != "":
		w.line("```%s\n%s\n```\n", w.fence(), s.Code)
	case s.Type != nil:
		w.line("```%s\n%s\n```\n", w.fence(), declaration(s))
	}
	w.docOutro(s.Doc)
	if s.Kind == apimodel.KindEnum {
		for _, m := range s.Members {
			w.line("- `%s`%s", m.Name, summarySuffix(w, m))
		}
		w.line("")
	}
	w.source(s.Source)
}

// badges lists the flags a reader should notice before the description.
func (w *writer) badges(s *apimodel.Symbol) {
	var b []string
	for _, f := range []struct {
		on    bool
		label string
	}{{s.Flags.Static, "static"}, {s.Flags.Readonly, "readonly"}, {s.Flags.Abstract, "abstract"},
		{s.Flags.Async, "async"}, {s.Flags.Optional, "optional"}, {s.Flags.Stability != "", string(s.Flags.Stability)}} {
		if f.on {
			b = append(b, "`"+f.label+"`")
		}
	}
	if len(b) > 0 {
		w.line("%s\n", strings.Join(b, " "))
	}
}

// source links the definition, when the configuration says where code lives.
func (w *writer) source(src *apimodel.Source) {
	if src == nil || w.opts.SourceURL == nil {
		return
	}
	if href := w.opts.SourceURL(src); href != "" {
		w.line("[Source: %s:%d](%s)\n", src.File, src.Line, href)
	}
}

// symbolTitle is the heading HTML: <code>parse()</code>, <code>Lexer</code>.
func symbolTitle(s *apimodel.Symbol) string {
	name := html.EscapeString(s.Name)
	switch s.Kind {
	case apimodel.KindFunction, apimodel.KindMethod, apimodel.KindConstructor:
		name += "()"
	}
	return "<code>" + name + "</code>"
}

// declaration renders a non-callable symbol as one line of code.
func declaration(s *apimodel.Symbol) string {
	switch s.Kind {
	case apimodel.KindType:
		return "type " + s.Name + " = " + s.Type.String()
	case apimodel.KindVariable:
		kw := "let"
		if s.Flags.Readonly {
			kw = "const"
		}
		return kw + " " + s.Name + ": " + s.Type.String()
	}
	opt := ""
	if s.Flags.Optional {
		opt = "?"
	}
	return s.Name + opt + ": " + s.Type.String()
}

// summarySuffix is " — summary" for list entries, or "".
func summarySuffix(w *writer, s *apimodel.Symbol) string {
	if s.Doc == nil || s.Doc.Summary == "" {
		return ""
	}
	return " — " + cell(w.links(s.Doc.Summary))
}
