package generator

// Render hooks: a template decides the markup for one kind of Markdown node
// (GO-099).
//
// Everything the build does to rendered content today, it does with regular
// expressions over finished HTML: rewrite an image path, swap a `.jpg` for a
// `.webp`, relativise a link. That works until it does not, and it can only
// ever change what is already there.
//
// The gap it cannot close is the one that matters most. An image written in
// Markdown — `![](photo.jpg)` — comes out as a bare `<img>`: no `srcset`, no
// width, no height, no `loading="lazy"`. The whole responsive-image pipeline
// exists and is reachable only from templates, so a site migrated from
// WordPress has hundreds of images that skip it entirely. And there is no
// policy for external links at all: no `rel="noopener"`, which is a security
// property rather than a nicety.
//
// A hook is a template that renders one node kind, registered with goldmark as
// a NodeRenderer — at the AST, where the node still knows what it is, rather
// than over the string it became. Without hooks nothing is registered and the
// output is goldmark's own, byte for byte.

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	gmparser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Hook names, as `render_hooks:` spells them.
const (
	HookImage      = "image"
	HookLink       = "link"
	HookHeading    = "heading"
	HookCode       = "code"
	HookTable      = "table"
	HookBlockquote = "blockquote"
)

// hookNames is every hook this build knows, in a stable order for messages.
var hookNames = []string{HookImage, HookLink, HookHeading, HookCode, HookTable, HookBlockquote}

// hookPriority registers the hooks ahead of goldmark's own renderer.
//
// goldmark sorts its node renderers by priority and registers them from the
// end, so the LOWEST number registers last and wins. Its HTML renderer sits at
// 1000; anything below that overrides it for the kinds it claims.
const hookPriority = 100

// ImageContext is what an image hook's template receives.
type ImageContext struct {
	// Src is the destination as the author wrote it, after the build's own
	// path fixing has had its say.
	Src string
	// Alt is the image's alternative text, as plain text: it is an attribute
	// value, and markup inside it would be nonsense.
	Alt string
	// Title is the optional Markdown title, `![alt](src "title")`.
	Title string
	// IsExternal reports a destination on another origin.
	IsExternal bool
}

// LinkContext is what a link hook's template receives.
type LinkContext struct {
	Href  string
	Title string
	// Text is the link's own content, already rendered — so `[**bold**](/x)`
	// arrives as markup rather than as flattened text.
	Text       template.HTML
	IsExternal bool
}

// HeadingContext is what a heading hook's template receives. ID is the anchor
// the build already computed, so a hook does not have to derive it again and
// cannot disagree with the table of contents about it.
type HeadingContext struct {
	Level int
	ID    string
	Text  template.HTML
}

// CodeContext is what a code hook's template receives.
//
// Rendered is the block as this build would otherwise have written it, syntax
// highlighting included. A hook WRAPS that — adds a copy button, a filename, a
// language label — rather than replacing it, because re-implementing Chroma in
// a template is not what anyone wants from this.
type CodeContext struct {
	Lang     string
	Code     string
	Rendered template.HTML
	// Info is the whole info string, so `go title="main.go"` reaches a hook
	// that wants the part after the language.
	Info string
}

// InnerContext is what the container hooks receive: their content, rendered.
type InnerContext struct {
	Inner template.HTML
}

// hookSet is the loaded hooks for one build.
type hookSet struct {
	tmpls map[string]*template.Template
	// inner renders a node's children with the ordinary renderer, so a hook
	// gets real markup and not a flattened approximation. Nested hooks do not
	// apply inside it, which is the price of not recursing forever.
	inner  renderer.Renderer
	domain string
}

// loadRenderHooks parses the configured hook templates.
//
// A hook that names a file the site does not have is an error at load time:
// the author asked for it by name, and silently rendering without it would
// look like the hook simply did nothing.
func (g *Generator) loadRenderHooks(funcs template.FuncMap) error {
	if len(g.config.RenderHooks) == 0 {
		g.hooks = nil
		return nil
	}
	set := &hookSet{tmpls: map[string]*template.Template{}, domain: g.config.Domain}
	for _, name := range sortedKeys(g.config.RenderHooks) {
		path := g.config.RenderHooks[name]
		if path == "" {
			continue
		}
		if !knownHook(name) {
			return fmt.Errorf("render_hooks: %q is not a hook (have: %s)", name, strings.Join(hookNames, ", "))
		}
		raw, err := os.ReadFile(path) // #nosec G304 -- a template path from the site's own config
		if err != nil {
			return fmt.Errorf("render_hooks.%s: %w", name, err)
		}
		tmpl, err := template.New(filepath.Base(path)).Funcs(funcs).Parse(string(raw))
		if err != nil {
			return fmt.Errorf("render_hooks.%s: %s: %w", name, path, err)
		}
		set.tmpls[name] = tmpl
	}
	if len(set.tmpls) == 0 {
		g.hooks = nil
		return nil
	}
	set.inner = plainRenderer(g.config)
	g.hooks = set
	if !g.config.Quiet {
		fmt.Printf("   🪝 Render hooks: %s\n", strings.Join(sortedKeys(set.tmpls), ", "))
	}
	// The markdown renderer is rebuilt with the hooks registered; content is
	// converted after this, so nothing was rendered without them.
	g.md = buildMarkdownWith(g.config, set)
	return nil
}

// knownHook reports whether a name is one of the hooks.
func knownHook(name string) bool {
	for _, n := range hookNames {
		if n == name {
			return true
		}
	}
	return false
}

// RegisterFuncs claims the node kinds this set has templates for, and leaves
// every other kind to goldmark.
func (h *hookSet) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	if h == nil {
		return
	}
	if _, ok := h.tmpls[HookImage]; ok {
		reg.Register(ast.KindImage, h.renderImage)
	}
	if _, ok := h.tmpls[HookLink]; ok {
		reg.Register(ast.KindLink, h.renderLink)
	}
	if _, ok := h.tmpls[HookHeading]; ok {
		reg.Register(ast.KindHeading, h.renderHeading)
	}
	if _, ok := h.tmpls[HookCode]; ok {
		reg.Register(ast.KindFencedCodeBlock, h.renderCode)
	}
	if _, ok := h.tmpls[HookTable]; ok {
		reg.Register(extast.KindTable, h.renderTable)
	}
	if _, ok := h.tmpls[HookBlockquote]; ok {
		reg.Register(ast.KindBlockquote, h.renderBlockquote)
	}
}

// execute renders one hook template into the output.
//
// A hook that fails writes nothing and says so. The alternative — falling back
// to goldmark's markup — would hide a broken template behind output that looks
// almost right, which is the failure mode this whole feature exists to replace.
func (h *hookSet) execute(w util.BufWriter, name string, ctx interface{}) (ast.WalkStatus, error) {
	tmpl := h.tmpls[name]
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return ast.WalkStop, fmt.Errorf("render_hooks.%s: %w", name, err)
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkSkipChildren, nil
}

// renderImage renders an image through its hook.
func (h *hookSet) renderImage(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	img := n.(*ast.Image)
	dest := string(img.Destination)
	return h.execute(w, HookImage, ImageContext{
		Src:        dest,
		Alt:        nodeText(n, source),
		Title:      string(img.Title),
		IsExternal: h.isExternal(dest),
	})
}

// renderLink renders a link through its hook.
func (h *hookSet) renderLink(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	link := n.(*ast.Link)
	dest := string(link.Destination)
	return h.execute(w, HookLink, LinkContext{
		Href:       dest,
		Title:      string(link.Title),
		Text:       h.renderChildren(source, n),
		IsExternal: h.isExternal(dest),
	})
}

// renderHeading renders a heading through its hook.
func (h *hookSet) renderHeading(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	heading := n.(*ast.Heading)
	id := ""
	if v, ok := heading.AttributeString("id"); ok {
		if b, ok := v.([]byte); ok {
			id = string(b)
		}
	}
	return h.execute(w, HookHeading, HeadingContext{
		Level: heading.Level,
		ID:    id,
		Text:  h.renderChildren(source, n),
	})
}

// renderCode renders a fenced code block through its hook, handing it the
// block as this build would otherwise have written it.
func (h *hookSet) renderCode(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	block := n.(*ast.FencedCodeBlock)
	var code bytes.Buffer
	lines := block.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		code.Write(line.Value(source))
	}
	info := ""
	if block.Info != nil {
		info = string(block.Info.Segment.Value(source))
	}
	return h.execute(w, HookCode, CodeContext{
		Lang:     string(block.Language(source)),
		Code:     code.String(),
		Info:     info,
		Rendered: h.renderNode(source, n),
	})
}

// renderTable and renderBlockquote hand a container its rendered content.
func (h *hookSet) renderTable(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	return h.execute(w, HookTable, InnerContext{Inner: h.renderNode(source, n)})
}

func (h *hookSet) renderBlockquote(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	return h.execute(w, HookBlockquote, InnerContext{Inner: h.renderChildren(source, n)})
}

// renderChildren renders a node's children with the ordinary renderer.
func (h *hookSet) renderChildren(source []byte, n ast.Node) template.HTML {
	var buf bytes.Buffer
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if err := h.inner.Render(&buf, source, c); err != nil {
			return ""
		}
	}
	// #nosec G203 -- goldmark's own rendering of the author's content
	return template.HTML(buf.String())
}

// renderNode renders a whole node with the ordinary renderer, which is how a
// hook receives what it is wrapping.
func (h *hookSet) renderNode(source []byte, n ast.Node) template.HTML {
	var buf bytes.Buffer
	if err := h.inner.Render(&buf, source, n); err != nil {
		return ""
	}
	// #nosec G203 -- goldmark's own rendering of the author's content
	return template.HTML(buf.String())
}

// isExternal reports whether a destination leaves this site.
//
// A scheme-bearing URL on another host is external; a root-relative or
// same-document reference never is. A site with no domain configured treats
// every absolute URL as external, which is the safe reading.
func (h *hookSet) isExternal(dest string) bool {
	lower := strings.ToLower(strings.TrimSpace(dest))
	switch {
	// A protocol-relative URL starts with a slash and is not a local path, so
	// it has to be recognised before the root-relative case eats it.
	case strings.HasPrefix(lower, "//"):
	case lower == "", strings.HasPrefix(lower, "#"), strings.HasPrefix(lower, "/"):
		return false
	case strings.HasPrefix(lower, "mailto:"), strings.HasPrefix(lower, "tel:"):
		return false
	case !namesAHost(lower):
		return false // a relative path
	}
	if h.domain == "" {
		return true
	}
	// Compare hosts rather than whole prefixes: a site's own links are written
	// with either scheme and sometimes with neither, and three concatenated
	// prefixes only made that harder to read.
	domain := strings.ToLower(strings.TrimSuffix(h.domain, "/"))
	host := stripScheme(lower)
	return host != domain && !strings.HasPrefix(host, domain+"/")
}

// namesAHost reports whether a reference carries an origin at all — a web
// scheme, or the scheme-relative form — rather than being a path on this site.
func namesAHost(lower string) bool {
	for _, prefix := range ownOriginPrefixes("") {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// stripScheme removes a URL's scheme and its leading slashes, so two addresses
// can be compared by host whatever each was written with.
func stripScheme(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		return u[i+3:]
	}
	return strings.TrimPrefix(u, "//")
}

// plainRenderer is a renderer with this site's extensions and no hooks, used
// to produce the content a hook wraps.
func plainRenderer(cfg Config) renderer.Renderer {
	return buildMarkdown(cfg).Renderer()
}

// hookRendererOption registers a hook set with goldmark, or nothing when there
// are no hooks — which is what keeps an unhooked build byte-identical.
func hookRendererOption(h *hookSet) []renderer.Option {
	if h == nil || len(h.tmpls) == 0 {
		return nil
	}
	return []renderer.Option{
		renderer.WithNodeRenderers(util.Prioritized(h, hookPriority)),
		gmhtml.WithUnsafe(),
	}
}

// soleImageParagraph unwraps a paragraph whose only content is one image.
//
// Markdown treats an image as inline, so `![](photo.jpg)` on its own line
// becomes `<p><img></p>`. That is fine for an `<img>` and invalid for what an
// image hook usually emits: a `<figure>` inside a `<p>` is markup no browser
// agrees about, and a caption is the first thing anyone writes such a hook for.
//
// It runs only when an image hook is configured, so a site without one keeps
// the paragraphs it has always had.
type soleImageParagraph struct{}

// Transform implements gmparser.ASTTransformer.
func (soleImageParagraph) Transform(doc *ast.Document, _ text.Reader, _ gmparser.Context) {
	var unwrap []*ast.Paragraph
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if p, ok := n.(*ast.Paragraph); ok && onlyChildIsImage(p) {
			unwrap = append(unwrap, p)
		}
		return ast.WalkContinue, nil
	})
	for _, p := range unwrap {
		parent := p.Parent()
		if parent == nil {
			continue
		}
		img := p.FirstChild()
		p.RemoveChild(p, img)
		parent.ReplaceChild(parent, p, img)
	}
}

// onlyChildIsImage reports a paragraph holding exactly one image and nothing
// else that renders.
func onlyChildIsImage(p *ast.Paragraph) bool {
	child := p.FirstChild()
	if child == nil || child.Kind() != ast.KindImage {
		return false
	}
	for c := child.NextSibling(); c != nil; c = c.NextSibling() {
		return false
	}
	return true
}

// hasImageHook reports whether an image hook is configured.
func (h *hookSet) hasImageHook() bool {
	if h == nil {
		return false
	}
	_, ok := h.tmpls[HookImage]
	return ok
}

// hookTransformers is the AST transformer list for a build: the heading-id fix
// always, and the image unwrap only when an image hook will render one.
func hookTransformers(h *hookSet) []util.PrioritizedValue {
	out := []util.PrioritizedValue{util.Prioritized(headingIDTransformer{}, 900)}
	if h.hasImageHook() {
		out = append(out, util.Prioritized(soleImageParagraph{}, 950))
	}
	return out
}
