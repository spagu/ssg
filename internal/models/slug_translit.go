package models

// Transliterating slugs (#426): `slug_transliterate: true` folds Latin letters
// with diacritics to their base letters before Slugify's ASCII filter, so
// "Zażółć gęślą jaźń" becomes "zazolc-gesla-jazn" instead of "za-g-l-ja".
//
// It is opt-in because a slug is a URL, and a URL is a promise: Slugify stays
// exactly as it is, and this is the only other way ssg makes a slug from a
// label. There is still one ASCII filter — SlugifyTranslit is Slugify over a
// folded string, never a second copy of the rule.
//
// Only Latin letters are folded. Cyrillic, Greek, Devanagari, CJK and every
// other script are left to the filter, which drops them as it always did: each
// of those has several romanisation standards that disagree (Russian г is "g"
// in one, Ukrainian г is "h"), and choosing one for the reader is guessing. A
// title with no Latin letter therefore still slugs to "", and each caller keeps
// its own fallback for that.

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// latinFold is every Latin letter Unicode does not decompose into a base
// letter plus marks (so NFD leaves it whole), mapped to the ASCII a reader of
// the language writes when the letter is not available. Lowercase only: the
// input is lowercased first. ä, é, ż, ç, å, ő and the rest need no entry — NFD
// splits them into a base letter and a combining mark, and the mark is dropped.
// That also means German ä/ö/ü become a/o/u, not ae/oe/ue: the table does not
// guess a language from a word.
var latinFold = map[rune]string{
	'\u00df': "ss", // ß German; ẞ lowercases to it
	'\u00e6': "ae", // æ Danish, Norwegian, Icelandic
	'\u0153': "oe", // œ French
	'\u00f8': "o",  // ø Danish, Norwegian
	'\u0142': "l",  // ł Polish
	'\u0111': "d",  // đ Croatian, Bosnian, Serbian Latin, Vietnamese
	'\u00f0': "d",  // ð Icelandic, Faroese
	'\u00fe': "th", // þ Icelandic
	'\u0131': "i",  // ı Turkish, Azerbaijani dotless i
	'\u0127': "h",  // ħ Maltese
	'\u0140': "l",  // ŀ Catalan l with middle dot
	'\u0133': "ij", // ĳ Dutch
	'\u0167': "t",  // ŧ Northern Sami
}

// TransliterateLatin lowercases s, splits each letter into a base letter and
// its combining marks (Unicode NFD), drops the marks and folds the letters of
// latinFold. Letters of other scripts pass through unchanged.
func TransliterateLatin(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if fold, ok := latinFold[r]; ok {
			b.WriteString(fold)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// SlugifyTranslit is Slugify after TransliterateLatin.
func SlugifyTranslit(s string) string { return Slugify(TransliterateLatin(s)) }

// SlugFunc returns the slug function a build uses: SlugifyTranslit when
// slug_transliterate is on, Slugify otherwise.
func SlugFunc(transliterate bool) func(string) string {
	if transliterate {
		return SlugifyTranslit
	}
	return Slugify
}
