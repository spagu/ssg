package generator

// {{ formatNumber 111381 }} → "111,381" (#358). A theme printing a count had
// add, mul and div but nothing that writes a number for a reader, so a site
// stored "111,381" as a string in its data for every count it showed.
//
// Digits are grouped the way the page's language groups them — the same
// language localizeDate uses — and an explicit separator overrides that:
//
//	{{ formatNumber 111381 }}      → 111,381    (en)
//	{{ formatNumber 111381 }}      → 111 381    (pl; a no-break space)
//	{{ formatNumber 111381 }}      → 1,11,381   (hi; the Indian grouping)
//	{{ formatNumber 111381 " " }}  → 111 381
//	{{ formatNumber 1234.5 }}      → 1,234.5

import (
	"strconv"
	"strings"
)

// numberStyle is how a language writes a number: the group separator, the
// decimal mark, and whether it groups the Indian way (3, then 2s).
type numberStyle struct {
	group, decimal string
	indian         bool
}

// numberStyles are the conventions of the languages ssg sites are written in
// (CLDR). A language not listed writes as English does. The space-grouping
// languages use a no-break space so a number never wraps across lines.
var numberStyles = map[string]numberStyle{
	"en": {",", ".", false},
	"pl": {" ", ",", false}, "cs": {" ", ",", false}, "sk": {" ", ",", false},
	"ru": {" ", ",", false}, "uk": {" ", ",", false}, "sv": {" ", ",", false},
	"fi": {" ", ",", false}, "nb": {" ", ",", false},
	"fr": {" ", ",", false},
	"de": {".", ",", false}, "es": {".", ",", false}, "it": {".", ",", false},
	"nl": {".", ",", false}, "pt": {".", ",", false}, "da": {".", ",", false},
	"hi": {",", ".", true}, "mr": {",", ".", true}, "bn": {",", ".", true},
}

// tmplFormatNumber writes a number for a reader in the current page's
// language, or with the separator given. Integers, floats (a JSON number
// arrives as float64) and numeric strings are accepted; a whole float prints
// without a decimal part. Anything else comes back as text, unchanged.
//
// The language is the page's; on a site without i18n, default_language; and
// English when neither says — so a Polish site writes 111 381 with nothing
// set beyond the language it already declares.
func (g *Generator) tmplFormatNumber(n interface{}, sep ...string) string {
	return formatNumber(n, firstNonEmpty(g.currentLang, g.config.DefaultLanguage, "en"), sep...)
}

// formatNumber is tmplFormatNumber for a given language.
func formatNumber(n interface{}, lang string, sep ...string) string {
	text, ok := numberText(n)
	if !ok {
		return text
	}
	code, _, _ := strings.Cut(strings.ToLower(lang), "-")
	style, known := numberStyles[code]
	if !known {
		style = numberStyles["en"]
	}
	if len(sep) > 0 {
		style.group, style.indian = sep[0], false
	}
	sign := ""
	if strings.HasPrefix(text, "-") {
		sign, text = "-", text[1:]
	}
	whole, frac, hasFrac := strings.Cut(text, ".")
	out := sign + groupDigits(whole, style.group, style.indian)
	if hasFrac {
		out += style.decimal + frac
	}
	return out
}

// numberText is a number as plain digits ("-1234.5"), and whether it was a
// number at all.
func numberText(n interface{}) (string, bool) {
	switch v := n.(type) {
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case int32:
		return strconv.FormatInt(int64(v), 10), true
	case uint:
		return strconv.FormatUint(uint64(v), 10), true
	case uint64:
		return strconv.FormatUint(v, 10), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), true
	case string:
		s := strings.TrimSpace(v)
		if f, err := strconv.ParseFloat(s, 64); err == nil && !strings.ContainsAny(s, "eEinfINFnaNA") {
			return strconv.FormatFloat(f, 'f', -1, 64), true
		}
		return v, false
	}
	return str(n), false
}

// groupDigits inserts the separator between groups of a digit string:
// threes, or the Indian three-then-twos.
func groupDigits(digits, sep string, indian bool) string {
	if len(digits) <= 3 {
		return digits
	}
	head, tail := digits[:len(digits)-3], digits[len(digits)-3:]
	size := 3
	if indian {
		size = 2
	}
	var groups []string
	for len(head) > size {
		groups = append([]string{head[len(head)-size:]}, groups...)
		head = head[:len(head)-size]
	}
	groups = append([]string{head}, groups...)
	return strings.Join(append(groups, tail), sep)
}
