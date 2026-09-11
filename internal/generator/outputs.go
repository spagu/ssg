package generator

// One page, several representations (GO-092).
//
// Two mechanisms did this already and did not know about each other.
// `outputs: [html, json]` wrote an `index.json` beside every `index.html`, from
// one global list nobody could vary per content type. `markdown_publish: true`
// wrote `index.md`, the flat sibling, `llms.txt` and the `<head>` alternate —
// a complete Markdown output, reached by a completely separate switch.
//
// So a site could publish JSON for everything or nothing, Markdown for
// everything or nothing, and could not say "my reference pages also publish
// plain text" at all.
//
// This is one registry: a format is a name, a file suffix, a MIME type for the
// alternate link, and a renderer. The two existing formats keep their exact
// behaviour — the golden corpora prove their bytes did not move — and the list
// gains a per-type form, a `txt` format, and formats a site defines with a
// template of its own.

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	texttemplate "text/template"

	"github.com/spagu/ssg/internal/models"
)

// Built-in format names.
const (
	FormatHTML     = "html"
	FormatJSON     = "json"
	FormatMarkdown = "markdown"
	FormatText     = "txt"
)

// OutputFormat describes one representation of a page.
type OutputFormat struct {
	// Name is what `outputs:` calls it.
	Name string
	// Suffix replaces "index.html" for this format's file; "index.json",
	// "index.md", "index.txt".
	Suffix string
	// MIME is announced in the page's <head> as a `rel="alternate"` type. An
	// empty MIME means the format is not advertised.
	MIME string
	// Render produces the file's bytes for a page. A format that returns an
	// empty body writes nothing.
	Render func(g *Generator, page models.Page) ([]byte, error)
	// Custom marks a format the site defined with a template.
	Custom bool
}

// CustomOutput is a format a site defines for itself.
//
// This is where the ticket's "XML" lives. A generic XML output would have to
// invent a schema, and a schema nobody agreed on is noise; a site that wants
// XML knows which one it wants and writes the template.
type CustomOutput struct {
	Name     string `yaml:"name" toml:"name" json:"name"`
	Suffix   string `yaml:"suffix" toml:"suffix" json:"suffix"`
	MIME     string `yaml:"mime" toml:"mime" json:"mime"`
	Template string `yaml:"template" toml:"template" json:"template"`
}

// builtinFormats are the representations this build knows how to write.
func builtinFormats() map[string]OutputFormat {
	return map[string]OutputFormat{
		FormatJSON: {
			Name: FormatJSON, Suffix: "index.json", MIME: "application/json",
			Render: func(g *Generator, page models.Page) ([]byte, error) {
				data, err := json.MarshalIndent(g.pageRecord(page), "", "  ")
				if err != nil {
					return nil, err
				}
				return append(data, '\n'), nil
			},
		},
		FormatMarkdown: {
			Name: FormatMarkdown, Suffix: "index.md", MIME: "text/markdown",
			Render: func(g *Generator, page models.Page) ([]byte, error) {
				return encodeText(g.pageMarkdown(page), g.encodingFor(&page)), nil
			},
		},
		FormatText: {
			Name: FormatText, Suffix: "index.txt", MIME: "text/plain",
			Render: func(g *Generator, page models.Page) ([]byte, error) {
				return encodeText(g.pageText(page), g.encodingFor(&page)), nil
			},
		},
	}
}

// pageText is the page as plain prose: its title, then its body with the
// markup taken out. For a reader with a screen reader, a terminal, or a model
// with a small context, that is the whole document and none of the chrome.
func (g *Generator) pageText(page models.Page) string {
	body := tmplStripHTML(g.convertMarkdownToHTML(g.cleanSpecialChars(page.Content)))
	body = collapseBlankLines(strings.TrimSpace(body))
	title := strings.TrimSpace(page.Title)
	if title == "" {
		return body + "\n"
	}
	if body == "" {
		return title + "\n"
	}
	return title + "\n\n" + body + "\n"
}

// collapseBlankLines squeezes the runs of empty lines that stripping tags
// leaves behind.
func collapseBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}

// outputRegistry is the formats available to this build.
type outputRegistry struct {
	formats map[string]OutputFormat
	// perType is the resolved list per content type, with "" holding the
	// default for a type the config does not name.
	perType map[string][]string
}

// loadOutputs builds the registry from the site's configuration.
//
// The custom templates are parsed here so a broken one fails at load, where the
// author is looking, rather than on the page that happened to use it.
func (g *Generator) loadOutputs(funcs template.FuncMap) error {
	reg := &outputRegistry{formats: builtinFormats(), perType: map[string][]string{}}

	for _, c := range g.config.OutputsCustom {
		name := strings.ToLower(strings.TrimSpace(c.Name))
		if name == "" || name == FormatHTML {
			return fmt.Errorf("outputs_custom: a format needs a name of its own (%q)", c.Name)
		}
		if _, builtin := reg.formats[name]; builtin {
			return fmt.Errorf("outputs_custom: %q is a built-in format", name)
		}
		if c.Template == "" {
			return fmt.Errorf("outputs_custom.%s: template is required", name)
		}
		tmpl, err := parseOutputTemplate(c.Template, funcs)
		if err != nil {
			return fmt.Errorf("outputs_custom.%s: %w", name, err)
		}
		suffix := c.Suffix
		if suffix == "" {
			suffix = "index." + name
		}
		reg.formats[name] = OutputFormat{
			Name: name, Suffix: suffix, MIME: c.MIME, Custom: true,
			Render: func(g *Generator, page models.Page) ([]byte, error) {
				var buf bytes.Buffer
				if err := tmpl.Execute(&buf, g.pageOutputContext(page)); err != nil {
					return nil, err
				}
				return buf.Bytes(), nil
			},
		}
	}

	perType, err := g.resolveOutputLists(reg)
	if err != nil {
		return err
	}
	reg.perType = perType
	g.outputs = reg
	return nil
}

// parseOutputTemplate reads a custom format's template from disk.
//
// text/template, not html/template, and the difference matters: a custom format
// is by definition not HTML — the HTML one is built in — and html/template's
// contextual escaping is actively wrong everywhere else. It turned the XML
// declaration `<?xml version="1.0"?>` into `&lt;?xml version="1.0"?>`, which is
// not a document any XML parser will read.
//
// The template therefore owns its own escaping, and gets `xmlEscape` for the
// common case. The theme's helpers come along, so a custom format can call what
// a partial can.
func parseOutputTemplate(path string, funcs template.FuncMap) (*texttemplate.Template, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- a template path from the site's own config
	if err != nil {
		return nil, err
	}
	tmpl := texttemplate.New(filepath.Base(path)).Funcs(outputFuncs(funcs))
	return tmpl.Parse(string(raw))
}

// outputFuncs are the helpers a custom format's template can call: the theme's,
// plus escaping for the format it is most likely to be.
func outputFuncs(funcs template.FuncMap) texttemplate.FuncMap {
	out := texttemplate.FuncMap{
		// xmlEscape makes a value safe as XML text or an attribute value.
		"xmlEscape": func(v interface{}) string {
			var buf bytes.Buffer
			// Writing to a bytes.Buffer cannot fail; the error exists for
			// writers that can.
			_ = xml.EscapeText(&buf, []byte(fmt.Sprintf("%v", v)))
			return buf.String()
		},
	}
	for name, fn := range funcs {
		if _, taken := out[name]; !taken {
			out[name] = fn
		}
	}
	return out
}

// pageOutputContext is what a custom format's template receives: the page and
// the site, which is the same shape a page template gets.
func (g *Generator) pageOutputContext(page models.Page) map[string]interface{} {
	return map[string]interface{}{
		"Page":    page,
		"Site":    g.siteData,
		"Domain":  g.config.Domain,
		"Content": g.contentContextValue(page.Content),
	}
}

// resolveOutputLists works out which formats each content type publishes.
//
// Back-compat is the whole job here. `outputs: [html, json]` is a flat list
// that has always meant "every type", and it keeps meaning exactly that;
// `outputs: {page: [...], post: [...]}` is the new per-type form. And
// `markdown_publish: true` becomes markdown in every list, because it has been
// the way to ask for that since GO-085 and there is no reason to make anyone
// rewrite it.
func (g *Generator) resolveOutputLists(reg *outputRegistry) (map[string][]string, error) {
	lists := map[string][]string{}
	if len(g.config.OutputsPerType) > 0 {
		for kind, formats := range g.config.OutputsPerType {
			lists[strings.ToLower(kind)] = formats
		}
	}
	if len(g.config.Outputs) > 0 {
		lists[""] = g.config.Outputs
	}
	if g.config.MarkdownPublish {
		if len(lists) == 0 {
			lists[""] = nil
		}
		for kind := range lists {
			lists[kind] = appendUnique(lists[kind], FormatMarkdown)
		}
	}
	for kind, formats := range lists {
		if err := reg.checkNames(kind, formats); err != nil {
			return nil, err
		}
	}
	return lists, nil
}

// checkNames refuses a format nobody defined, naming the ones that exist. A
// typo here otherwise produces a page that is simply never written.
func (r *outputRegistry) checkNames(kind string, formats []string) error {
	for _, name := range formats {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == FormatHTML {
			continue // always written; listing it is harmless
		}
		if _, ok := r.formats[name]; !ok {
			return fmt.Errorf("outputs%s: %q is not a format (have: %s)",
				typeSuffix(kind), name, strings.Join(r.names(), ", "))
		}
	}
	return nil
}

// typeSuffix names the config key an error came from.
func typeSuffix(kind string) string {
	if kind == "" {
		return ""
	}
	return "." + kind
}

// names lists every format, built-in and custom, in a stable order.
func (r *outputRegistry) names() []string {
	out := make([]string, 0, len(r.formats)+1)
	out = append(out, FormatHTML)
	for name := range r.formats {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// appendUnique adds a format to a list once.
func appendUnique(list []string, name string) []string {
	for _, existing := range list {
		if strings.EqualFold(existing, name) {
			return list
		}
	}
	return append(list, name)
}

// formatsFor is the list of non-HTML formats one page publishes.
//
// Precedence, narrowest first: the page's own `outputs:` (GO-096), then the
// list for its content type, then the site-wide list.
func (g *Generator) formatsFor(page models.Page) []string {
	if g.outputs == nil {
		return nil
	}
	var list []string
	switch {
	case len(page.Outputs) > 0:
		list = page.Outputs
	default:
		kind := strings.ToLower(page.Type)
		if perType, ok := g.outputs.perType[kind]; ok {
			list = perType
		} else {
			list = g.outputs.perType[""]
		}
	}
	out := make([]string, 0, len(list))
	for _, name := range list {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == FormatHTML || name == "" {
			continue
		}
		if _, ok := g.outputs.formats[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

// publishesFormat reports whether a page publishes one named format.
func (g *Generator) publishesFormat(page models.Page, name string) bool {
	for _, f := range g.formatsFor(page) {
		if strings.EqualFold(f, name) {
			return true
		}
	}
	return false
}

// alternateLinks is the `<head>` block announcing a page's other
// representations, skipping anything the theme already declared.
func (g *Generator) alternateLinks(page models.Page, existing string) string {
	if g.outputs == nil {
		return ""
	}
	var b strings.Builder
	for _, name := range g.formatsFor(page) {
		format := g.outputs.formats[name]
		if format.MIME == "" {
			continue
		}
		href := outputURLFor(page, format)
		// GO-085 announces the Markdown copy with its own absolute link, and a
		// page must not carry two alternates for one representation.
		if href == "" || strings.Contains(existing, href) || strings.Contains(existing, `type="`+format.MIME+`"`) {
			continue
		}
		fmt.Fprintf(&b, `<link rel="alternate" type="%s" href="%s">`+"\n", format.MIME, href)
	}
	return b.String()
}

// outputURLFor is the path a page's representation is served at.
//
// A directory page gets the suffix beside its index; a flat page swaps its
// extension. That is the convention GO-085 established for Markdown, kept here
// so every format is addressed the same way.
func outputURLFor(page models.Page, format OutputFormat) string {
	url := page.GetURL()
	if url == "" {
		return ""
	}
	if strings.HasSuffix(url, "/") {
		// A page with no slug resolves to "//" rather than "/", and a doubled
		// slash in a published href is a different URL from the one meant.
		return strings.TrimRight(url, "/") + "/" + format.Suffix
	}
	ext := format.Suffix
	if i := strings.LastIndex(ext, "."); i >= 0 {
		ext = ext[i:]
	}
	return strings.TrimSuffix(url, ".html") + ext
}

// outputFilePath is where a page's representation is written, given the path
// its HTML went to.
func outputFilePath(htmlPath string, format OutputFormat) string {
	if strings.HasSuffix(htmlPath, indexHTMLName) {
		return strings.TrimSuffix(htmlPath, indexHTMLName) + format.Suffix
	}
	ext := format.Suffix
	if i := strings.LastIndex(ext, "."); i >= 0 {
		ext = ext[i:]
	}
	return strings.TrimSuffix(htmlPath, ".html") + ext
}

// writePageOutputs writes every representation a page publishes besides its
// HTML.
//
// Markdown keeps its flat sibling: GO-085 wrote `/section.md` beside
// `/section/index.md` so a URL without a trailing slash resolves too, and sites
// depend on those files existing. A format that returns nothing writes nothing.
func (g *Generator) writePageOutputs(page models.Page, htmlPath string) {
	if !strings.HasSuffix(strings.ToLower(htmlPath), ".html") {
		return
	}
	for _, name := range g.formatsFor(page) {
		format := g.outputs.formats[name]
		data, err := format.Render(g, page)
		if err != nil {
			g.warnOutput(fmt.Sprintf("output %s for %q: %v", name, page.Slug, err))
			continue
		}
		if len(data) == 0 {
			continue
		}
		path := outputFilePath(htmlPath, format)
		if err := g.ensureWithinOutput(path); err != nil {
			g.warnOutput(fmt.Sprintf("output %s for %q: %v", name, page.Slug, err))
			continue
		}
		// #nosec G306,G703 -- a published representation of the page beside it
		if err := os.WriteFile(path, data, 0o644); err != nil {
			g.warnOutput(fmt.Sprintf("output %s for %q: %v", name, page.Slug, err))
			continue
		}
		if name == FormatMarkdown {
			g.writeFlatMarkdownSibling(htmlPath, data)
		}
	}
}

// writeFlatMarkdownSibling keeps GO-085's `/section.md` beside
// `/section/index.md`, so a URL written without its trailing slash still
// resolves to the Markdown copy.
func (g *Generator) writeFlatMarkdownSibling(htmlPath string, data []byte) {
	if !strings.HasSuffix(htmlPath, indexHTMLName) {
		return
	}
	dir := filepath.Dir(htmlPath)
	if filepath.Clean(dir) == filepath.Clean(g.config.OutputDir) {
		return // the site root has no slug to hang a flat file on
	}
	if err := g.ensureWithinOutput(dir + ".md"); err != nil {
		return
	}
	// #nosec G306,G703 -- the flat Markdown sibling, beside the page it copies
	_ = os.WriteFile(dir+".md", data, 0o644)
}

// warnOutput reports a representation that could not be written, once per
// distinct message.
func (g *Generator) warnOutput(msg string) {
	g.componentMu.Lock()
	defer g.componentMu.Unlock()
	if g.componentWarned == nil {
		g.componentWarned = map[string]bool{}
	}
	if g.componentWarned[msg] {
		return
	}
	g.componentWarned[msg] = true
	fmt.Printf("   ⚠️  %s\n", msg)
}

// injectAlternates places the discovery links in the document head.
func injectAlternates(s, links string) string {
	if links == "" {
		return s
	}
	if i := strings.LastIndex(s, "</head>"); i >= 0 {
		return s[:i] + links + s[i:]
	}
	return links + s
}
