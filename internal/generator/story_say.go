package generator

// Pronunciation rules for the recorded Web Story narration (1.9.4) — the
// same rules, read the same way, as assets/story/story-say.js does for the
// browser's voice; see assets/story/pronounce/README.md for the syntax.
// testdata/story/pronounce.json and every built-in set's examples are the
// cases both must agree on.
//
// RE2 has no lookaround, so "whole words only" is checked by hand: each rule
// is tried, longest first, anchored at every position, and kept only when
// the characters on either side are not letters or digits.

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const sayNum = `(\d+(?:[.,]\d+)?)`

// sayPlaceholder is {num} or {num:one|two} in what is said.
var sayPlaceholder = regexp.MustCompile(`\{num(?::([^|}]*)\|([^}]*))?\}`)

var (
	plUnits    = strings.Fields("zero jeden dwa trzy cztery pięć sześć siedem osiem dziewięć")
	plTeens    = strings.Fields("dziesięć jedenaście dwanaście trzynaście czternaście piętnaście szesnaście siedemnaście osiemnaście dziewiętnaście")
	plTens     = append([]string{"", ""}, strings.Fields("dwadzieścia trzydzieści czterdzieści pięćdziesiąt sześćdziesiąt siedemdziesiąt osiemdziesiąt dziewięćdziesiąt")...)
	plHundreds = append([]string{""}, strings.Fields("sto dwieście trzysta czterysta pięćset sześćset siedemset osiemset dziewięćset")...)
)

// sayRule is one compiled pattern.
type sayRule struct {
	re         *regexp.Regexp
	kinds      []string // what each group captures: "num" or "word"
	head, tail bool     // must start / end on a word boundary
	said       string
}

// sayer applies a set of rules to text.
type sayer struct {
	rules []sayRule
	lang  string
}

// newSayer compiles rules for a language (pl, en, …).
func newSayer(rules map[string]string, lang string) *sayer {
	lang = strings.ToLower(strings.FieldsFunc(lang+"_", func(r rune) bool { return r == '-' || r == '_' })[0])
	keys := make([]string, 0, len(rules))
	for k := range rules {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	s := &sayer{lang: lang}
	for _, k := range keys {
		if r, ok := compileSay(k); ok {
			r.said = rules[k]
			s.rules = append(s.rules, r)
		}
	}
	return s
}

// compileSay turns one pattern into an anchored regexp.
func compileSay(key string) (sayRule, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return sayRule{}, false
	}
	var r sayRule
	parts := strings.Split(key, "{num}")
	src := make([]string, len(parts))
	for i, part := range parts {
		if i > 0 {
			r.kinds = append(r.kinds, "num")
		}
		words := regexp.MustCompile(`\s+`).Split(part, -1)
		for j, w := range words {
			if strings.Contains(w, "|") {
				r.kinds = append(r.kinds, "word")
				alts := strings.Split(w, "|")
				// Longest first: RE2 does not backtrack into "mins" after
				// "min" fails the word boundary, as the browser's regex does.
				sort.SliceStable(alts, func(a, b int) bool { return len(alts[a]) > len(alts[b]) })
				for a := range alts {
					alts[a] = regexp.QuoteMeta(alts[a])
				}
				words[j] = "(" + strings.Join(alts, "|") + ")"
			} else {
				words[j] = regexp.QuoteMeta(w)
			}
		}
		src[i] = strings.Join(words, `\s*`)
	}
	first, _ := utf8.DecodeRuneInString(key)
	last, _ := utf8.DecodeLastRuneInString(key)
	r.head = strings.HasPrefix(key, "{num}") || isWordRune(first)
	r.tail = strings.HasSuffix(key, "{num}") || isWordRune(last)
	r.re = regexp.MustCompile(`^(?:` + strings.Join(src, sayNum) + `)`) // every literal is quoted
	return r, true
}

// isWordRune is a letter or a digit, any script.
func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// Say rewrites text for the voice.
func (s *sayer) Say(text string) string {
	if len(s.rules) == 0 {
		return text
	}
	var out strings.Builder
	for i := 0; i < len(text); {
		prev, _ := utf8.DecodeLastRuneInString(text[:i])
		done := false
		for _, r := range s.rules {
			if r.head && i > 0 && isWordRune(prev) {
				continue
			}
			m := r.re.FindStringSubmatchIndex(text[i:])
			if m == nil {
				continue
			}
			end := i + m[1]
			if next, _ := utf8.DecodeRuneInString(text[end:]); r.tail && end < len(text) && isWordRune(next) {
				continue
			}
			out.WriteString(s.speak(r, text[i:], m))
			i, done = end, true
			break
		}
		if !done {
			_, n := utf8.DecodeRuneInString(text[i:])
			out.WriteString(text[i : i+n])
			i += n
		}
	}
	return out.String()
}

// speak is what a matched rule says.
func (s *sayer) speak(r sayRule, text string, m []int) string {
	var nums, words []string
	for j, kind := range r.kinds {
		v := ""
		if m[2+2*j] >= 0 {
			v = text[m[2+2*j]:m[3+2*j]]
		}
		if kind == "num" {
			nums = append(nums, v)
		} else {
			words = append(words, v)
		}
	}
	word := ""
	if len(words) > 0 {
		word = words[0]
	}
	k, last, hasLast := 0, "", false
	toks := regexp.MustCompile(`\s+|\S+`).FindAllString(r.said, -1)
	for i, tok := range toks {
		tok = strings.ReplaceAll(tok, "{word}", word)
		tok = sayPlaceholder.ReplaceAllStringFunc(tok, func(p string) string {
			sub := sayPlaceholder.FindStringSubmatch(p)
			last, hasLast = "", true
			if k < len(nums) {
				last = nums[k]
			}
			k++
			if strings.Contains(p, ":") {
				if w, ok := plWords(last, s.lang, sub[1], sub[2]); ok {
					return w
				}
			}
			return last
		})
		if strings.Contains(tok, "|") {
			tok = s.form(tok, last, hasLast)
		}
		toks[i] = tok
	}
	return strings.Join(toks, "")
}

// form picks the plural form agreeing with num.
func (s *sayer) form(word, num string, has bool) string {
	forms := strings.Split(word, "|")
	if !has {
		return forms[0]
	}
	n, _ := strconv.ParseFloat(strings.Replace(num, ",", ".", 1), 64)
	i := pluralIndex(s.lang, n)
	if i >= len(forms) {
		i = len(forms) - 1
	}
	return forms[i]
}

// pluralIndex is the CLDR plural category of n as an index into the
// language's categories in order: Polish one, few, many, other; anything
// else one, other.
func pluralIndex(lang string, n float64) int {
	whole := n == float64(int64(n))
	i := int64(n)
	if lang != "pl" {
		if whole && i == 1 {
			return 0
		}
		return 1
	}
	switch {
	case !whole:
		return 3
	case i == 1:
		return 0
	case i%10 >= 2 && i%10 <= 4 && (i%100 < 12 || i%100 > 14):
		return 1
	}
	return 2
}

// plWords is a whole number up to 999 999 in Polish words, one replacing
// a lone 1 and two a final 2; false for other languages and fractions.
func plWords(num, lang, one, two string) (string, bool) {
	if lang != "pl" || num == "" || len(num) > 6 || strings.Trim(num, "0123456789") != "" {
		return "", false
	}
	n, _ := strconv.Atoi(num)
	if n == 1 {
		return one, true
	}
	th, rest := n/1000, n%1000
	var out []string
	if th > 0 {
		f, t := th%10, th%100
		switch {
		case th == 1:
			out = append(out, "tysiąc")
		case f >= 2 && f <= 4 && (t < 12 || t > 14):
			out = append(out, plHundreds3(th, "")+" tysiące")
		default:
			out = append(out, plHundreds3(th, "")+" tysięcy")
		}
	}
	if rest > 0 || th == 0 {
		out = append(out, plHundreds3(rest, two))
	}
	return strings.Join(out, " "), true
}

// plHundreds3 is 0–999 in Polish words; two replaces a final "dwa".
func plHundreds3(n int, two string) string {
	out := []string{plHundreds[n/100]}
	r := n % 100
	switch {
	case r >= 10 && r < 20:
		out = append(out, plTeens[r-10])
	default:
		out = append(out, plTens[r/10])
		if r%10 != 0 || n == 0 {
			u := plUnits[r%10]
			if r%10 == 2 && two != "" {
				u = two
			}
			out = append(out, u)
		}
	}
	parts := out[:0]
	for _, w := range out {
		if w != "" {
			parts = append(parts, w)
		}
	}
	return strings.Join(parts, " ")
}
