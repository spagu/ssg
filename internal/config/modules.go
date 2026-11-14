package config

// ModuleConfig is one `modules:` entry (GO-144): a module deployed outside
// this site's build — a standalone Worker such as newsletter-ui — that the
// core worker should know about. Workers in `workers:` need no entry; the
// build reads their module.json itself.
type ModuleConfig struct {
	// ID is the module's id, as in its module.json.
	ID string `yaml:"id" toml:"id" json:"id"`
	// URL is the module's base path on the site's own host
	// (/newsletter-admin). A scheme, host, "//", "..", backslash or query is
	// a configuration error: a module is always on the same host.
	URL string `yaml:"url" toml:"url" json:"url"`
	// Dir optionally points at the module's source, so its module.json is
	// read and validated at build time; without it the core worker asks the
	// module for its manifest at runtime.
	Dir string `yaml:"dir" toml:"dir" json:"dir"`
}
