package models

// How the host serves URLs (#103).
//
// This started as a boolean feeding link checking only, and modelled one
// behaviour: strip ".html" and append a trailing slash. Cloudflare Pages strips
// the extension but adds no slash, so on Pages the "corrected" target that
// check_redirects suggested was itself a URL the host would redirect. One
// boolean cannot describe both hosts, so it became a mode — while still
// accepting `true` and `false`, which keep meaning exactly what they meant.

import (
	"fmt"
	"path"
	"strings"
)

// PrettyURLMode describes what the host does to a URL before serving it.
type PrettyURLMode string

const (
	// PrettyOff — the host serves files literally. "/docs/intro" is a genuine
	// 404 rather than a redirect, which is how a plain object store behaves.
	PrettyOff PrettyURLMode = "off"

	// PrettyStrip — ".html" is dropped and no trailing slash is added:
	// /docs/intro.html is served at /docs/intro. Cloudflare Pages.
	PrettyStrip PrettyURLMode = "strip"

	// PrettyStripSlash — ".html" is dropped and a trailing slash is added:
	// /docs/intro.html is served at /docs/intro/. This is what `true` has always
	// meant, so it stays the meaning of `true`.
	PrettyStripSlash PrettyURLMode = "strip-slash"
)

// Enabled reports whether the host rewrites URLs at all.
func (m PrettyURLMode) Enabled() bool {
	return m == PrettyStrip || m == PrettyStripSlash
}

// ParsePrettyURLMode accepts the boolean spellings alongside the named modes,
// so a config written before modes existed keeps its meaning.
func ParsePrettyURLMode(v string) (PrettyURLMode, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "false", "off", "no":
		return PrettyOff, nil
	case "true", "yes", "strip-slash":
		return PrettyStripSlash, nil
	case "strip":
		return PrettyStrip, nil
	}
	return PrettyOff, fmt.Errorf("pretty_urls: unknown value %q; expected off, strip or strip-slash (true means strip-slash); see --help", v)
}

// UnmarshalYAML accepts `pretty_urls: true` and `pretty_urls: strip` alike.
func (m *PrettyURLMode) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var b bool
	if err := unmarshal(&b); err == nil {
		*m = boolPrettyMode(b)
		return nil
	}
	var s string
	if err := unmarshal(&s); err != nil {
		return fmt.Errorf("pretty_urls: expected a boolean or one of off/strip/strip-slash; see --help")
	}
	parsed, err := ParsePrettyURLMode(s)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// UnmarshalJSON mirrors the YAML behaviour for JSON configs.
func (m *PrettyURLMode) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "true" || s == "false" {
		*m = boolPrettyMode(s == "true")
		return nil
	}
	parsed, err := ParsePrettyURLMode(strings.Trim(s, `"`))
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// UnmarshalText covers TOML, which decodes into an encoding.TextUnmarshaler.
func (m *PrettyURLMode) UnmarshalText(text []byte) error {
	parsed, err := ParsePrettyURLMode(string(text))
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// boolPrettyMode maps the historical boolean onto a mode.
func boolPrettyMode(b bool) PrettyURLMode {
	if b {
		return PrettyStripSlash
	}
	return PrettyOff
}

// ServedURL returns the URL the host actually answers for a path the build
// wrote — the form a page should name when it declares its own identity (#103).
//
// This is why the mode has to exist rather than being link-checking trivia: a
// canonical tag, an og:url or a sitemap <loc> naming a URL that 308s is the one
// thing those must not do, and it is invisible locally because resolution
// against the output directory is not how the host answers.
//
// Query strings and fragments are carried across untouched.
func (m PrettyURLMode) ServedURL(u string) string {
	if !m.Enabled() {
		return u
	}
	clean, suffix := splitURLSuffix(u)
	if clean == "" {
		return u
	}
	base, tail := splitURLBase(clean)
	if tail == "" || tail == "/" {
		return u
	}

	switch {
	case strings.EqualFold(tail, indexHTMLFile):
		// A directory index is already the directory's own URL.
		tail = ""
	case strings.HasSuffix(strings.ToLower(tail), ".html"):
		tail = strings.TrimSuffix(tail, path.Ext(tail))
	default:
		return u
	}

	out := base + tail
	if m == PrettyStripSlash {
		if !strings.HasSuffix(out, "/") {
			out += "/"
		}
	} else {
		// PrettyStrip: the host serves the bare form, so a trailing slash would
		// itself be a redirect.
		if out != "/" {
			out = strings.TrimSuffix(out, "/")
		}
		if out == "" {
			out = "/"
		}
	}
	return out + suffix
}

// indexHTMLFile is the directory index the host serves for a directory URL.
const indexHTMLFile = "index.html"

// splitURLBase separates the directory part from the final segment.
func splitURLBase(u string) (base, tail string) {
	i := strings.LastIndex(u, "/")
	if i < 0 {
		return "", u
	}
	return u[:i+1], u[i+1:]
}

// splitURLSuffix separates a query/fragment so it can be carried across.
func splitURLSuffix(u string) (string, string) {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		return u[:i], u[i:]
	}
	return u, ""
}
