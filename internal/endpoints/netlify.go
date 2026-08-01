package endpoints

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/config"
)

func init() { Register(netlifyAdapter{}) }

// netlifyAdapter emits Netlify Functions (v2 ES modules) under netlify/functions/.
// Each function declares its own route with `export const config = { path }`, so
// no _redirects wiring is needed and nothing SSG already generates is clobbered
// (#63).
type netlifyAdapter struct{}

func (netlifyAdapter) Platform() string { return "netlify" }

func (netlifyAdapter) Emit(eps []config.Endpoint, outDir string) ([]string, error) {
	var written []string
	for _, ep := range eps {
		src, err := netlifySource(ep)
		if err != nil {
			return nil, fmt.Errorf("endpoint %q: %w", ep.Path, err)
		}
		rel := filepath.Join("netlify", "functions", pathSlug(ep.Path)+".mjs")
		full := filepath.Join(outDir, rel)
		// #nosec G301 -- function dirs must be traversable by the build
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, err
		}
		// #nosec G306 -- function source is a public build artifact
		if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
			return nil, err
		}
		written = append(written, rel)
	}
	return written, nil
}

// netlifySource renders one Netlify Function v2 module (Web-API handler).
func netlifySource(ep config.Endpoint) (string, error) {
	var b strings.Builder
	b.WriteString(cfGeneratedHeader)
	switch ep.Type {
	case "redirect":
		if ep.To == "" {
			return "", fmt.Errorf("redirect needs a 'to'")
		}
		status := ep.Status
		if status == 0 {
			status = 302
		}
		if status < 300 || status > 399 {
			return "", fmt.Errorf("redirect status %d is not a 3xx", status)
		}
		b.WriteString("export const config = { path: " + jsStringLit(ep.Path) + " };\n")
		b.WriteString("export default async (request) => {\n")
		b.WriteString("  const to = new URL(" + jsStringLit(ep.To) + ", request.url).toString();\n")
		fmt.Fprintf(&b, "  return Response.redirect(to, %d);\n", status)
		b.WriteString("};\n")
	case "proxy":
		if ep.Target == "" {
			return "", fmt.Errorf("proxy needs a 'target'")
		}
		b.WriteString("export const config = { path: " + jsStringLit(ep.Path) + " };\n")
		b.WriteString("export default async (request) => {\n")
		if m := methodsLit(ep.Methods); m != "" {
			b.WriteString("  const allowed = " + m + ";\n")
			b.WriteString("  if (!allowed.includes(request.method)) {\n")
			b.WriteString("    return new Response(\"method not allowed\", { status: 405, headers: { Allow: allowed.join(\", \") } });\n")
			b.WriteString("  }\n")
		}
		b.WriteString("  const target = new URL(" + jsStringLit(ep.Target) + ");\n")
		b.WriteString("  target.search = new URL(request.url).search;\n")
		b.WriteString("  return fetch(new Request(target, request));\n")
		b.WriteString("};\n")
	case "form":
		body, err := formBodyJS(ep)
		if err != nil {
			return "", err
		}
		b.WriteString("export const config = { path: " + jsStringLit(ep.Path) + " };\n")
		b.WriteString("export default async (request) => {\n")
		b.WriteString(body)
		b.WriteString("};\n")
	default:
		return "", fmt.Errorf("unknown type %q (want redirect, proxy or form)", ep.Type)
	}
	return b.String(), nil
}
