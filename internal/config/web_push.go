package config

// `web_push:` — browser notifications about new posts (#368). The sending is
// done by the web-push worker; this switch makes the site ready for it.

import "gopkg.in/yaml.v3"

// WebPushConfig turns on the static half of Web Push: a web app manifest and
// the subscribe script in every page's <head>. `web_push: true` uses the
// defaults, and so does a mapping that does not say `enabled: false`.
type WebPushConfig struct {
	Enabled bool
	// Endpoint is where the web-push worker answers; default "/api/push".
	Endpoint string
}

// webPushFields is the mapping form.
type webPushFields struct {
	Enabled  *bool  `yaml:"enabled" toml:"enabled" json:"enabled"`
	Endpoint string `yaml:"endpoint" toml:"endpoint" json:"endpoint"`
}

func (w *WebPushConfig) set(f webPushFields) {
	*w = WebPushConfig{Enabled: toggleOn(f.Enabled), Endpoint: f.Endpoint}
}

// UnmarshalYAML accepts `web_push: true` as well as the mapping.
func (w *WebPushConfig) UnmarshalYAML(value *yaml.Node) error {
	return decodeToggleYAML(value, &w.Enabled, w.set)
}

// UnmarshalJSON accepts `"web_push": true` as well as the object.
func (w *WebPushConfig) UnmarshalJSON(data []byte) error {
	return decodeToggleJSON(data, &w.Enabled, w.set)
}

// UnmarshalTOML accepts `web_push = true` as well as the table.
func (w *WebPushConfig) UnmarshalTOML(data interface{}) error {
	return decodeToggleTOML("web_push", data, &w.Enabled, w.set)
}
