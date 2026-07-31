package endpoints

import "strings"

// functionFile maps an endpoint path to a function file path under a platform's
// functions root: /api/quote -> api/quote<ext>; "/" or "" -> index<ext>.
func functionFile(epPath, ext string) string {
	clean := strings.Trim(epPath, "/")
	if clean == "" {
		clean = "index"
	}
	return clean + ext
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
