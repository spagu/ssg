package endpoints

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/config"
)

func init() { Register(vercelAdapter{}) }

// vercelAdapter emits Vercel Edge Functions under api/ plus a vercel.json that
// rewrites each endpoint path to its function, so a clean path like /api/quote
// resolves regardless of the file name (#63).
type vercelAdapter struct{}

func (vercelAdapter) Platform() string { return "vercel" }

func (vercelAdapter) Emit(eps []config.Endpoint, outDir string) ([]string, error) {
	var written []string
	rewrites := make([]vercelRewrite, 0, len(eps))
	for _, ep := range eps {
		src, err := vercelSource(ep)
		if err != nil {
			return nil, fmt.Errorf("endpoint %q: %w", ep.Path, err)
		}
		slug := pathSlug(ep.Path)
		rel := filepath.Join("api", slug+".js")
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
		rewrites = append(rewrites, vercelRewrite{Source: ep.Path, Destination: "/api/" + slug})
	}
	doc, err := json.MarshalIndent(map[string]interface{}{"rewrites": rewrites}, "", "  ")
	if err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(outDir, "vercel.json")
	// #nosec G306 -- config is a public build artifact
	if err := os.WriteFile(cfgPath, append(doc, '\n'), 0o644); err != nil {
		return nil, err
	}
	written = append(written, "vercel.json")
	return written, nil
}

type vercelRewrite struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// vercelSource renders one Vercel Edge Function (Web-API handler).
func vercelSource(ep config.Endpoint) (string, error) {
	var b strings.Builder
	b.WriteString(cfGeneratedHeader)
	b.WriteString("export const config = { runtime: 'edge' };\n")
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
		b.WriteString("export default function handler(request) {\n")
		b.WriteString("  const to = new URL(" + jsStringLit(ep.To) + ", request.url).toString();\n")
		fmt.Fprintf(&b, "  return Response.redirect(to, %d);\n", status)
		b.WriteString("}\n")
	case "proxy":
		if ep.Target == "" {
			return "", fmt.Errorf("proxy needs a 'target'")
		}
		b.WriteString("export default function handler(request) {\n")
		if m := methodsLit(ep.Methods); m != "" {
			b.WriteString("  const allowed = " + m + ";\n")
			b.WriteString("  if (!allowed.includes(request.method)) {\n")
			b.WriteString("    return new Response(\"method not allowed\", { status: 405, headers: { Allow: allowed.join(\", \") } });\n")
			b.WriteString("  }\n")
		}
		b.WriteString("  const target = new URL(" + jsStringLit(ep.Target) + ");\n")
		b.WriteString("  target.search = new URL(request.url).search;\n")
		b.WriteString("  return fetch(new Request(target, request));\n")
		b.WriteString("}\n")
	case "form":
		body, err := formBodyJS(ep)
		if err != nil {
			return "", err
		}
		b.WriteString("export default async function handler(request) {\n")
		b.WriteString(body)
		b.WriteString("}\n")
	default:
		return "", fmt.Errorf("unknown type %q (want redirect, proxy or form)", ep.Type)
	}
	return b.String(), nil
}
