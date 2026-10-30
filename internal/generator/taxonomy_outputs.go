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
	"reflect"
	"strings"
)

// writeViewOutputs writes the custom formats a taxonomy view publishes beside
// the HTML written to htmlPath. view names the page in warnings.
func (g *Generator) writeViewOutputs(formats []string, htmlPath, view string, data interface{}) {
	if g.outputs == nil || len(formats) == 0 {
		return
	}
	ctx := viewContext(data)
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
		g.writeOutputFile(outputFilePath(htmlPath, format), out, what)
	}
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
