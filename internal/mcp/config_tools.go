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
	"strings"

	"gopkg.in/yaml.v3"
)

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
				"the theme, the highlight style, story mode, the Listen button, share cards). CAN: set the keys " +
				"listed by designer_config_read. CANNOT: touch secrets (API keys, tokens, passwords), " +
				"deployment, server, endpoints, hooks, or content/URL structure — those are refused. " +
				"The file is validated after the edit and the change is rolled back if it would leave " +
				"the configuration invalid. Comments and key order in the file are preserved.",
			schema: objectSchema(map[string]any{
				"key": stringProp("Setting to change, e.g. \"mermaid_theme\" (must be one from designer_config_read)"),
				"value": stringProp("New value; \"true\"/\"false\" for switches, a number for counts, " +
					"comma-separated for lists (\"webp,avif\"), otherwise text"),
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
		fmt.Fprintf(&b, "  %-28s = %-24s %s (%s)\n", k, val, def.desc, def.kind)
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
	updated, err := writeConfigKey(before, key, def.kind, typed)
	if err != nil {
		return errResult("could not update " + path + ": " + err.Error())
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil { // #nosec G306 G703 -- the project config file this run was given
		return errResult("could not write " + path + ": " + err.Error())
	}
	// A setting that leaves the file unloadable is rolled back, so the designer
	// can never strand the project on a broken config.
	if s.opts.ValidateConfig != nil {
		if verr := s.opts.ValidateConfig(path); verr != nil {
			_ = os.WriteFile(path, before, 0o644) // #nosec G306 G703 -- restoring the config file this run was given
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
		if v, ok := lookupConfigValue(doc, k); ok {
			out[k] = displayConfigValue(v)
		}
	}
	return out, nil
}

// lookupConfigValue follows a dotted key ("marketing.logo") into the decoded file.
func lookupConfigValue(doc map[string]interface{}, key string) (interface{}, bool) {
	var cur interface{} = doc
	for _, seg := range strings.Split(key, ".") {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		if cur, ok = m[seg]; !ok {
			return nil, false
		}
	}
	return cur, true
}

// displayConfigValue shows a switch's mapping by its `enabled` alone: the rest
// of it is the author's, and may hold paths or text the model need not see.
func displayConfigValue(v interface{}) string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	if on, set := m["enabled"]; set {
		return fmt.Sprintf("enabled: %v", on)
	}
	return "(settings in the file)"
}
