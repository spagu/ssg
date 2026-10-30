package generator

// Custom output formats beside the taxonomy pages (#367).
//
// `outputs_custom` formats were written for content pages only, so a site
// that publishes an `index.json` beside every page had none for `/license/` or
// `/license/mit/`. A taxonomy now takes `outputs:` (or `outputs_index:` and
// `outputs_term:`), and each listed format renders its own template with the
// data the HTML view gets.
//
// Only custom formats apply. The built-in json, markdown and txt describe a
// page — its front matter, its body — and a taxonomy view has neither.

import (
	"fmt"
	stdhtml "html"
	"reflect"
	"strings"
)

// writeViewOutputs writes the custom formats a taxonomy view publishes beside
// the HTML that goes to htmlPath, served at pageURL, and keeps a <head> link
// for each format written, for that HTML to carry (#370). view names the page
// in warnings.
func (g *Generator) writeViewOutputs(formats []string, htmlPath, pageURL, view string, data interface{}) {
	if g.outputs == nil || len(formats) == 0 {
		return
	}
	ctx := viewContext(data)
	var links []viewAlternate
	defer func() {
		if len(links) > 0 {
			g.viewAlternates.Store(htmlPath, links)
		}
	}()
	for _, raw := range formats {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" || name == FormatHTML {
			continue
		}
		what := fmt.Sprintf("output %s for %s", name, view)
		format, ok := g.outputs.formats[name]
		if !ok || format.renderView == nil {
			g.warnOutput(what + ": a taxonomy takes formats from outputs_custom only")
			continue
		}
		out, err := format.renderView(ctx)
		if err != nil {
			g.warnOutput(fmt.Sprintf("%s: %v", what, err))
			continue
		}
		if len(out) == 0 {
			continue
		}
		if g.writeOutputFile(outputFilePath(htmlPath, format), out, what) && format.MIME != "" {
			links = append(links, viewAlternate{mime: format.MIME, href: strings.TrimRight(pageURL, "/") + "/" + format.Suffix})
		}
	}
}

// viewAlternate is one format a taxonomy page announces.
type viewAlternate struct{ mime, href string }

// injectViewAlternates places the links writeViewOutputs kept for the page
// written to outputPath. A format the theme already announces, by its address
// or its type, is not announced twice — the rule content pages follow.
func (g *Generator) injectViewAlternates(s, outputPath string) string {
	v, ok := g.viewAlternates.LoadAndDelete(outputPath)
	if !ok {
		return s
	}
	var b strings.Builder
	for _, a := range v.([]viewAlternate) {
		if strings.Contains(s, `href="`+a.href+`"`) || strings.Contains(s, `rel="alternate" type="`+a.mime+`"`) {
			continue
		}
		fmt.Fprintf(&b, `<link rel="alternate" type="%s" href="%s">`+"\n", stdhtml.EscapeString(a.mime), stdhtml.EscapeString(a.href))
	}
	return injectAlternates(s, b.String())
}

// viewContext turns a view's data into a map, so one format template can test
// for a field only some views have — `{{if .Term}}` — without failing on the
// views that lack it, as it would on a struct.
func viewContext(data interface{}) map[string]interface{} {
	if m, ok := data.(map[string]interface{}); ok {
		return m
	}
	out := map[string]interface{}{}
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return out
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		if f := t.Field(i); f.IsExported() {
			out[f.Name] = v.Field(i).Interface()
		}
	}
	return out
}
