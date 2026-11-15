package mcp

import "github.com/spagu/ssg/internal/config"

// The designer's allow-list: which settings an assistant may change through
// designer_config_set, and how each is written.

// Kinds of writable setting. A switch is a feature configured by a mapping
// (`story:`, `listen:`…): the assistant turns it on or off, and the mapping's
// other settings stay with the file's author.
const (
	kindBool   = "bool"
	kindInt    = "int"
	kindString = "string"
	kindList   = "list"   // comma-separated in, a YAML list out
	kindSwitch = "switch" // true/false, written to <key>.enabled
)

// configKey is one presentation setting the designer may change.
type configKey struct {
	kind string
	desc string
}

// designerConfigKeys is the complete set of writable keys. Anything absent here
// is refused — notably every secret (`ai.*.key`, `mcp.git.token`, `jwt_secret`,
// auth), deployment, server, endpoint, hook and URL-structure setting, plus
// `sass_binary` (an executable path), and since 1.9.10 `templates_dir` and
// `static_dir`: they name directories the build reads and copies into the
// public output, so `static_dir: .` would publish the whole repository.
//
// The test for membership is not "is it about rendering" but: does it carry a
// secret, change where the site deploys, what the server does, or what a URL
// is? The site's own name and description fail all four, and they are what a
// designer or content manager most obviously expects to edit (#212). So do the
// presentation features added since — story mode, the Listen, Print and Cook
// Mode buttons, heading anchors, share cards — and the files published for
// agents, `markdown_publish` and `site_graph` among them (#410).
//
// `language` and `author` are absent because no such top-level key exists.
// `default_language` does, but it drives i18n, URL prefixes and hreflang — a
// URL-structure setting, refused by the rule above rather than by oversight.
// The site's identity for sharing lives under `marketing.` and is writable key
// by key; `marketing.verification` holds search-console tokens and is not.
//
// The list is no longer kept here: since GO-177 it is every configuration
// schema entry marked .assistant() (internal/config/schema_docs_*.go), so the
// key, its kind and its description have one source. config_keys_pin_test.go
// holds the set as it was, so a change to it cannot slip in unnoticed.
var designerConfigKeys = designerKeysFromSchema()

// designerKeysFromSchema reads the allow-list from the configuration schema.
func designerKeysFromSchema() map[string]configKey {
	keys := config.MCPWritableKeys()
	out := make(map[string]configKey, len(keys))
	for _, k := range keys {
		out[k.Path] = configKey{kind: k.Kind, desc: k.Description}
	}
	return out
}
