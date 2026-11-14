package admintheme

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spagu/ssg/internal/modules"
)

// Config is `admin_theme:` as the build reads it. Every field is optional.
type Config struct {
	Name      string
	Logo      string
	LogoDark  string
	Main      string
	Accent    string
	Font      string
	FontBody  string
	FontFiles []FontFileConfig
	// Tokens overrides single themed tokens per scheme ("light", "dark"),
	// after derivation; each overridden pair is checked again.
	Tokens map[string]map[string]string
}

// FontFileConfig is one `font_files:` entry.
type FontFileConfig struct {
	Family string
	Src    string
	Weight string
	Style  string
}

// maxName is the longest name the shared navigation shows (nav.js cuts there
// too); maxFontFiles keeps theme.css small.
const (
	maxName      = 80
	maxFontFiles = 8
)

var logoExt = map[string]bool{".svg": true, ".png": true, ".webp": true}

// configError names the configuration file and the key.
func configError(file, key, format string, args ...any) error {
	return fmt.Errorf("%s: admin_theme.%s %s", file, key, fmt.Sprintf(format, args...))
}

// checkName: plain text — no control or formatting character (except the
// zero-width joiners Devanagari needs), at most maxName characters.
func checkName(file, name string) error {
	if !utf8.ValidString(name) || strings.TrimSpace(name) == "" {
		return configError(file, "name", "must be plain text")
	}
	if n := utf8.RuneCountInString(name); n > maxName {
		return configError(file, "name", "is %d characters long; the navigation shows %d at most", n, maxName)
	}
	for _, r := range name {
		if (unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r)) && r != '‌' && r != '‍' {
			return configError(file, "name", "contains the control character %U; it must be plain text", r)
		}
	}
	return nil
}

// checkFile: a file on this host with one of the extensions allowed for it.
func checkFile(file, key, p string, allowed map[string]bool, what string) error {
	if !modules.SiteFilePath(p) {
		return configError(file, key, "%q must be a file on this site's own host, like /img/logo.svg — no scheme, host, \"//\", \"..\", "+
			"backslash, query, fragment or space: the panels load images and fonts from their own host only", p)
	}
	if !allowed[strings.ToLower(path.Ext(p))] {
		return configError(file, key, "%q is not %s", p, what)
	}
	return nil
}

// colour parses an optional colour; nil when empty.
func colour(file, key, raw string) (*RGB, error) {
	if raw == "" {
		return nil, nil
	}
	c, err := ParseHex(raw)
	if err != nil {
		return nil, configError(file, key, "%v", err)
	}
	return &c, nil
}

// overrides validates `tokens:`: known schemes, themed tokens, opaque hex.
func overrides(file string, in map[string]map[string]string) (map[string]map[string]RGB, error) {
	out := map[string]map[string]RGB{}
	for _, scheme := range slices.Sorted(maps.Keys(in)) {
		tokens := in[scheme]
		if scheme != "light" && scheme != "dark" {
			return nil, configError(file, "tokens."+scheme, "is not a scheme: use light: and dark:")
		}
		out[scheme] = map[string]RGB{}
		for _, name := range slices.Sorted(maps.Keys(tokens)) {
			raw := tokens[name]
			key := "tokens." + scheme + "." + name
			if !IsThemed(name) {
				return nil, configError(file, key, "is not a token a theme can set (%s); neutral and state colours stay fixed so that "+
					"every panel stays readable and red always means an error", strings.Join(Themed, ", "))
			}
			c, err := ParseHex(raw)
			if err != nil {
				return nil, configError(file, key, "%v", err)
			}
			out[scheme][name] = c
		}
	}
	return out, nil
}
