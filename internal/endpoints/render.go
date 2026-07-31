package endpoints

import (
	"fmt"
	"strings"

	"github.com/spagu/ssg/internal/config"
)

// formBodyJS renders the shared Web-API handler body for a form endpoint,
// operating on a `request` binding so every adapter can wrap it (CF destructures
// context.request first; Netlify/Vercel pass request directly). It drops bots via
// the honeypot, collects the declared (or all) fields, and POSTs them as JSON to
// the delivery webhook — the same behaviour as the self-hosted handler (#63).
func formBodyJS(ep config.Endpoint) (string, error) {
	if ep.To == "" {
		return "", fmt.Errorf("form needs a 'to' (delivery webhook URL)")
	}
	var b strings.Builder
	b.WriteString("  if (request.method !== \"POST\") {\n")
	b.WriteString("    return new Response(\"method not allowed\", { status: 405, headers: { Allow: \"POST\" } });\n")
	b.WriteString("  }\n")
	b.WriteString("  const form = await request.formData();\n")
	b.WriteString("  const done = () => ")
	if ep.Redirect != "" {
		b.WriteString("Response.redirect(new URL(" + jsStringLit(ep.Redirect) + ", request.url).toString(), 303);\n")
	} else {
		b.WriteString("new Response(JSON.stringify({ ok: true }), { headers: { \"content-type\": \"application/json\" } });\n")
	}
	if ep.Honeypot != "" {
		b.WriteString("  if (((form.get(" + jsStringLit(ep.Honeypot) + ") || \"\") + \"\").trim()) { return done(); }\n")
	}
	b.WriteString("  const payload = {};\n")
	if len(ep.Fields) > 0 {
		for _, f := range ep.Fields {
			if f == ep.Honeypot {
				continue
			}
			b.WriteString("  payload[" + jsStringLit(f) + "] = (form.get(" + jsStringLit(f) + ") || \"\").toString();\n")
		}
	} else {
		b.WriteString("  for (const [k, v] of form.entries()) {\n")
		if ep.Honeypot != "" {
			b.WriteString("    if (k === " + jsStringLit(ep.Honeypot) + ") continue;\n")
		}
		b.WriteString("    payload[k] = v.toString();\n")
		b.WriteString("  }\n")
	}
	b.WriteString("  const resp = await fetch(" + jsStringLit(ep.To) + ", { method: \"POST\", headers: { \"content-type\": \"application/json\" }, body: JSON.stringify(payload) });\n")
	b.WriteString("  if (!resp.ok) { return new Response(\"delivery failed\", { status: 502 }); }\n")
	b.WriteString("  return done();\n")
	return b.String(), nil
}

// functionFile maps an endpoint path to a function file path under a platform's
// functions root: /api/quote -> api/quote<ext>; "/" or "" -> index<ext>.
func functionFile(epPath, ext string) string {
	clean := strings.Trim(epPath, "/")
	if clean == "" {
		clean = "index"
	}
	return clean + ext
}

// pathSlug flattens an endpoint path into a single function name segment:
// /api/quote -> api-quote; "/" or "" -> index.
func pathSlug(epPath string) string {
	clean := strings.Trim(epPath, "/")
	if clean == "" {
		return "index"
	}
	return strings.ReplaceAll(clean, "/", "-")
}

// jsStringLit renders a safe double-quoted JS string literal, escaping the
// backslash, quote and newline characters that would otherwise break the
// generated module. Values come from the author's own config (trusted) and land
// in standalone .js files (never inlined in HTML), so no markup escaping is
// needed — just valid-JS escaping.
func jsStringLit(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(s) + `"`
}

// methodsLit renders an upper-cased JS array literal of HTTP methods, or "" when
// no method restriction is configured.
func methodsLit(methods []string) string {
	if len(methods) == 0 {
		return ""
	}
	parts := make([]string, 0, len(methods))
	for _, m := range methods {
		if m = strings.ToUpper(strings.TrimSpace(m)); m != "" {
			parts = append(parts, jsStringLit(m))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
