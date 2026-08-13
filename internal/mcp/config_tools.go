package mcp

// Designer-owned configuration editing (#1.8.16). Presentation does not live in
// templates alone — the theme, the syntax-highlight style, whether diagrams
// render — so the designer can set those keys too. It is deliberately narrow:
// only an allow-list of presentation settings is writable, every other key
// (secrets, deployment, server, content and URL structure) is refused, and a
// write that leaves the file invalid is rolled back.

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/spagu/ssg/internal/config"
)

// configKey is one presentation setting the designer may change.
type configKey struct {
	kind string // "bool", "int" or "string"
	desc string
}

// designerConfigKeys is the complete set of writable keys. Anything absent here
// is refused — notably every secret (`ai.*.key`, `mcp.git.token`, `jwt_secret`,
// auth), deployment, server, endpoint, hook and URL-structure setting, plus
// `sass_binary` (an executable path). Presentation only, by construction.
var designerConfigKeys = map[string]configKey{
	"template":               {"string", "theme name used to render the site"},
	"templates_dir":          {"string", "directory themes are loaded from"},
	"static_dir":             {"string", "directory copied verbatim into the output"},
	"mermaid":                {"bool", "render mermaid diagrams"},
	"mermaid_theme":          {"string", "mermaid built-in theme (default, neutral, dark, forest, base)"},
	"mermaid_background":     {"string", "mermaid background colour, e.g. \"#ffffff\""},
	"highlight":              {"bool", "syntax-highlight fenced code blocks"},
	"highlight_style":        {"string", "syntax-highlight colour scheme"},
	"highlight_line_numbers": {"bool", "show line numbers in highlighted code"},
	"math":                   {"bool", "render KaTeX math"},
	"toc":                    {"bool", "generate a table of contents"},
	"toc_depth":              {"int", "heading depth the table of contents covers"},
	"minify_html":            {"bool", "minify generated HTML"},
	"minify_css":             {"bool", "minify generated CSS"},
	"minify_js":              {"bool", "minify generated JS"},
	"minify_all":             {"bool", "minify HTML, CSS and JS together"},
	"pretty_html":            {"bool", "indent generated HTML for readability"},
	"sourcemap":              {"bool", "emit source maps for compiled assets"},
	"fingerprint":            {"bool", "content-hash asset filenames for cache busting"},
	"paginate":               {"int", "posts per page in listings"},
	"webp":                   {"bool", "generate WebP variants of images"},
	"webp_quality":           {"int", "WebP encoder quality (1-100)"},
	"image_sizes_attr":       {"string", "default sizes attribute for responsive images"},
}

// configTools returns the designer's configuration tools. They exist only when a
// config file is known; without one there is nothing to edit.
func (s *Server) configTools() []tool {
	return []tool{
		{
			name: "designer_config_read",
			description: "DESIGNER · Read the presentation settings you are allowed to change, with " +
				"their current values and what each does. Call this before designer_config_set so you " +
				"know the key names and what they are set to now. Secrets and deployment settings are " +
				"never shown or writable.",
			schema:  objectSchema(nil),
			handler: s.configRead,
		},
		{
			name: "designer_config_set",
			description: "DESIGNER · Change one presentation setting in the site configuration (e.g. " +
				"the theme, the mermaid theme, the highlight style, minification). CAN: set the keys " +
				"listed by designer_config_read. CANNOT: touch secrets (API keys, tokens, passwords), " +
				"deployment, server, endpoints, hooks, or content/URL structure — those are refused. " +
				"The file is validated after the edit and the change is rolled back if it would leave " +
				"the configuration invalid. Comments and key order in the file are preserved.",
			schema: objectSchema(map[string]any{
				"key":   stringProp("Setting to change, e.g. \"mermaid_theme\" (must be one from designer_config_read)"),
				"value": stringProp("New value; \"true\"/\"false\" for switches, a number for counts, otherwise text"),
			}, "key", "value"),
			handler: s.configSet,
		},
	}
}

func (s *Server) configRead(map[string]any) toolResult {
	current, err := readConfigValues(s.opts.ConfigPath)
	if err != nil {
		return errResult("could not read " + s.opts.ConfigPath + ": " + err.Error())
	}
	var b strings.Builder
	b.WriteString("Presentation settings you may change in " + s.opts.ConfigPath + ":\n")
	for _, k := range sortedConfigKeys() {
		def := designerConfigKeys[k]
		val, set := current[k]
		if !set {
			val = "(not set — the built-in default applies)"
		}
		fmt.Fprintf(&b, "  %-22s = %-24s %s (%s)\n", k, val, def.desc, def.kind)
	}
	b.WriteString("\nEverything else in the file — secrets, deployment, server, endpoints, hooks,\n" +
		"content and URL structure — is read-only for you and will be refused.")
	return textResult(b.String())
}

func (s *Server) configSet(args map[string]any) toolResult {
	key, _ := strArg(args, "key")
	value, ok := strArg(args, "value")
	if !ok {
		return errResult("`value` is required")
	}
	def, allowed := designerConfigKeys[key]
	if !allowed {
		return errResult(fmt.Sprintf("%q is not a setting the designer may change — call designer_config_read "+
			"for the list. Secrets, deployment, server and content/URL settings are refused by design.", key))
	}
	typed, err := typedConfigValue(def.kind, value)
	if err != nil {
		return errResult(fmt.Sprintf("%q expects a %s value: %v", key, def.kind, err))
	}

	path := s.opts.ConfigPath
	before, err := os.ReadFile(path) // #nosec G304 -- the config file this run was given
	if err != nil {
		return errResult("could not read " + path + ": " + err.Error())
	}
	updated, err := config.SetYAMLKey(before, key, typed)
	if err != nil {
		return errResult("could not update " + path + ": " + err.Error())
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil { // #nosec G306 -- project config file
		return errResult("could not write " + path + ": " + err.Error())
	}
	// A setting that leaves the file unloadable is rolled back, so the designer
	// can never strand the project on a broken config.
	if s.opts.ValidateConfig != nil {
		if verr := s.opts.ValidateConfig(path); verr != nil {
			_ = os.WriteFile(path, before, 0o644) // #nosec G306 -- restoring the previous file
			return errResult(fmt.Sprintf("%s = %s was rolled back: it makes the configuration invalid: %v", key, value, verr))
		}
	}
	return s.afterMutate(fmt.Sprintf("config %s = %s", key, value))
}

// sortedConfigKeys lists the writable keys in a stable order.
func sortedConfigKeys() []string {
	keys := make([]string, 0, len(designerConfigKeys))
	for k := range designerConfigKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// typedConfigValue converts the string argument to the key's declared type, so
// the written YAML holds a real bool/int rather than a quoted string.
func typedConfigValue(kind, raw string) (interface{}, error) {
	switch kind {
	case "bool":
		return strconv.ParseBool(raw)
	case "int":
		return strconv.Atoi(raw)
	default:
		return raw, nil
	}
}

// readConfigValues returns the current values of the writable keys, formatted for
// display. Only those keys are read — the rest of the file is never surfaced.
func readConfigValues(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- the config file this run was given
	if err != nil {
		return nil, err
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for k := range designerConfigKeys {
		if v, ok := doc[k]; ok {
			out[k] = fmt.Sprintf("%v", v)
		}
	}
	return out, nil
}
