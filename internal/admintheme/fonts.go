package admintheme

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// A font stack goes into a CSS declaration, so it is parsed against a strict
// grammar and written back from the parts — never copied through. A family is
// a quoted name of letters, digits, spaces, dots, dashes and underscores, or
// the same unquoted as words starting with a letter, or a generic keyword.
// Nothing else exists in the output: no ";", "{", "}", "url(", "@", comment,
// backslash, quote inside a name or line break can come through.

var (
	quotedFamilyRe   = regexp.MustCompile(`^[\p{L}\p{N} ._-]{1,64}$`)
	unquotedFamilyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*(?: [A-Za-z][A-Za-z0-9_-]*)*$`)
	weightRe         = regexp.MustCompile(`^(?:normal|bold|(?:[1-9][0-9]{0,2}|1000)(?: (?:[1-9][0-9]{0,2}|1000))?)$`)
)

var generics = map[string]bool{
	"serif": true, "sans-serif": true, "monospace": true, "cursive": true, "fantasy": true, "system-ui": true,
	"ui-serif": true, "ui-sans-serif": true, "ui-monospace": true, "ui-rounded": true, "math": true, "emoji": true, "fangsong": true,
}

// reserved are CSS-wide keywords a family may not be named unquoted.
var reserved = map[string]bool{"inherit": true, "initial": true, "unset": true, "revert": true, "revert-layer": true, "default": true}

const maxStack = 300

// family parses one family; generic is true for a generic keyword.
func family(raw string) (name string, generic bool, err error) {
	for _, r := range raw {
		if r != ' ' && (unicode.IsSpace(r) || unicode.IsControl(r)) {
			return "", false, fmt.Errorf("contains the character %U; write it on one line, family names separated by commas", r)
		}
	}
	s := strings.TrimSpace(raw)
	if len(s) > 66 {
		return "", false, fmt.Errorf("%.20q… is longer than a font family name can be (64 characters)", s)
	}
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if !quotedFamilyRe.MatchString(inner) {
			return "", false, fmt.Errorf("%s is not a font family name (letters, digits, spaces, dots, dashes and underscores only)", s)
		}
		return inner, false, nil
	}
	lower := strings.ToLower(s)
	switch {
	case generics[lower]:
		return lower, true, nil
	case reserved[lower]:
		return "", false, fmt.Errorf("%s is a CSS keyword, not a font family", s)
	case !unquotedFamilyRe.MatchString(s):
		return "", false, fmt.Errorf("%q is not a font family: quote a name with other characters, and use nothing but family names separated by commas", s)
	}
	return s, false, nil
}

// fallbacks are added to every stack: whatever the brand font, Hindi (and the
// other scripts the panels speak) still has a font, and the stack ends in a
// generic family.
var fallbacks = map[string][]string{
	"serif":      {"Noto Serif", "Noto Serif Devanagari", "Nirmala UI"},
	"sans-serif": {"Noto Sans", "Noto Sans Devanagari", "Nirmala UI"},
}

// Stack validates a font-family list and returns it as written to CSS:
// names double-quoted, generics bare, the script fallbacks inserted before
// the first generic, and generic (serif or sans-serif) appended when the
// list has none.
func Stack(raw, generic string) (string, error) {
	if len(raw) > maxStack {
		return "", fmt.Errorf("is longer than %d characters", maxStack)
	}
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("is empty")
	}

	var names []string
	seen := map[string]bool{}
	firstGeneric := -1
	for _, part := range strings.Split(raw, ",") {
		n, isGeneric, err := family(part)
		if err != nil {
			return "", err
		}
		if seen[strings.ToLower(n)] {
			continue
		}
		seen[strings.ToLower(n)] = true
		if isGeneric && firstGeneric < 0 {
			firstGeneric = len(names)
		}
		names = append(names, quoteFamily(n, isGeneric))
	}
	var extra []string
	for _, f := range fallbacks[generic] {
		if !seen[strings.ToLower(f)] {
			extra = append(extra, quoteFamily(f, false))
		}
	}
	if firstGeneric < 0 {
		names = append(append(names, extra...), generic)
	} else {
		names = append(names[:firstGeneric], append(extra, names[firstGeneric:]...)...)
	}
	return strings.Join(names, ", "), nil
}

func quoteFamily(n string, generic bool) string {
	if generic {
		return n
	}
	return `"` + n + `"`
}

// fontFormats maps a font file's extension to its @font-face format().
var fontFormats = map[string]string{".woff2": "woff2", ".woff": "woff", ".ttf": "truetype", ".otf": "opentype"}

var fontExt = map[string]bool{".woff2": true, ".woff": true, ".ttf": true, ".otf": true}

// FamilyName validates the family an @font-face declares: one name, not a
// generic keyword.
func FamilyName(raw string) (string, error) {
	n, generic, err := family(raw)
	if err != nil {
		return "", err
	}
	if generic || strings.Contains(raw, ",") {
		return "", fmt.Errorf("%q must be one family name of your own, not a generic family or a list", raw)
	}
	return n, nil
}

// Weight validates an @font-face weight: normal, bold, one number 1–1000 or
// a range "400 700". Empty means normal.
func Weight(raw string) (string, error) {
	if raw == "" {
		return "normal", nil
	}
	if !weightRe.MatchString(raw) {
		return "", fmt.Errorf("%q is not a font weight: use normal, bold, a number from 1 to 1000 or a range like \"400 700\"", raw)
	}
	return raw, nil
}

// Style validates an @font-face style. Empty means normal.
func Style(raw string) (string, error) {
	switch raw {
	case "":
		return "normal", nil
	case "normal", "italic", "oblique":
		return raw, nil
	}
	return "", fmt.Errorf("%q is not a font style: use normal, italic or oblique", raw)
}
