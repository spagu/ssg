package openapipage

import (
	"strings"

	"github.com/spagu/ssg/internal/openapi"
)

// operation writes one endpoint: heading, method and path, description,
// authorization, parameters, request body, responses and the console.
func (w *writer) operation(op *openapi.Operation) {
	id := Slug(op.ID)
	title := op.Summary
	if strings.TrimSpace(title) == "" {
		title = op.Method + " " + op.Path
	}
	w.heading(2, id, escape(title))
	w.line(`<p class="ssg-rest-endpoint" data-ssg-rest>%s <code>%s</code></p>`+"\n", methodLabel(op.Method), escape(op.Path))
	if op.Deprecated {
		w.line("> **Deprecated.** This operation may be removed; avoid it in new code.\n")
	}
	w.text(op.Description)
	w.line("**Authorization:** %s\n", w.requirements(op.Security))
	if len(op.Parameters) > 0 {
		w.heading(3, id+"-parameters", "Parameters")
		w.line("| Name | In | Type | Description |\n|---|---|---|---|")
		for _, p := range op.Parameters {
			name := "`" + p.Name + "`"
			if p.Required {
				name += " (required)"
			}
			w.line("| %s | %s | %s | %s |", name, p.In, w.typeOf(p.Schema), cell(describe(p.Description, p.Schema, p.Deprecated)))
		}
		w.line("")
	}
	if b := op.RequestBody; b != nil {
		w.heading(3, id+"-body", "Request body")
		if b.Required {
			w.line("Required.\n")
		}
		w.text(b.Description)
		for _, mt := range b.Content {
			w.media(mt)
		}
	}
	if len(op.Responses) > 0 {
		w.heading(3, id+"-responses", "Responses")
		w.line("| Status | Description | Body |\n|---|---|---|")
		for _, r := range op.Responses {
			body := ""
			if len(r.Content) > 0 {
				body = "`" + r.Content[0].Type + "`"
				if t := w.typeOf(r.Content[0].Schema); t != "" {
					body = t + " (" + r.Content[0].Type + ")"
				}
			}
			w.line("| `%s` | %s | %s |", r.Status, cell(r.Description), body)
		}
		w.line("")
		for _, r := range op.Responses {
			for _, mt := range r.Content {
				if ex := exampleText(mt, w.site.schemas); ex != "" && mt.Example != nil {
					w.line("**%s example** (`%s`)\n\n%s\n", r.Status, mt.Type, fence(mt.Type, ex))
				}
			}
		}
	}
	if w.site.opts.TryIt {
		w.tryIt(op)
	}
}

// methodLabel is an HTTP method as a coloured label; the class names the
// method, the text says it, so colour is never the only signal.
func methodLabel(method string) string {
	return `<span class="ssg-method ssg-method-` + strings.ToLower(method) + `" data-ssg-rest>` + method + `</span>`
}

// requirements says which credentials an operation takes: alternatives
// joined by "or", schemes needed together by "and".
func (w *writer) requirements(reqs []openapi.Requirement) string {
	if len(reqs) == 0 {
		return "none"
	}
	var alts []string
	for _, r := range reqs {
		if len(r) == 0 {
			alts = append(alts, "none")
			continue
		}
		var all []string
		for _, it := range r {
			s := "[" + it.Scheme + "](" + w.site.opts.Base + "#authentication)"
			if len(it.Scopes) > 0 {
				s += " (" + strings.Join(it.Scopes, ", ") + ")"
			}
			all = append(all, s)
		}
		alts = append(alts, strings.Join(all, " and "))
	}
	return strings.Join(alts, " or ")
}

// media writes one content type of a body: its schema and an example.
func (w *writer) media(mt openapi.MediaType) {
	w.line("Content type `%s`: %s\n", mt.Type, w.typeOf(mt.Schema))
	if mt.Schema != nil && mt.Schema.Ref == "" && len(mt.Schema.Properties) > 0 {
		w.properties(mt.Schema)
	}
	if ex := exampleText(mt, w.site.schemas); ex != "" {
		w.line("%s\n", fence(mt.Type, ex))
	}
}

// fence is a code block, highlighted as JSON when the type is JSON.
func fence(mediaType, body string) string {
	lang := ""
	if strings.Contains(mediaType, "json") {
		lang = "json"
	}
	return "```" + lang + "\n" + body + "\n```"
}
