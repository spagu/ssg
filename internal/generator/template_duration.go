package generator

// {{ duration "PT1H25M" }} → "1 hr 25 min" (1.8.72). Structured data keeps
// times as ISO 8601 durations, which is what search engines read and no
// reader wants to see; a theme printing a recipe's time used to keep a second,
// hand-written copy of it.

import (
	"regexp"
	"strconv"
	"strings"
)

// isoDurationRe matches PnDTnHnMnS (weeks, years and months are not times a
// page prints, and are rejected rather than guessed).
var isoDurationRe = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

// durationUnits are the unit words per language: day, hour, minute, second.
var durationUnits = map[string][4]string{
	"en": {"d", "hr", "min", "s"},
	"pl": {"d.", "godz.", "min", "s"},
	"de": {"T.", "Std.", "Min.", "Sek."},
	"fr": {"j", "h", "min", "s"},
	"es": {"d", "h", "min", "s"},
	"it": {"g", "h", "min", "s"},
}

// tmplDuration renders an ISO 8601 duration for people, in the language
// given (or English): "PT1H25M" → "1 hr 25 min". Input that is not a
// duration comes back unchanged, so a theme never prints less than it had.
func tmplDuration(iso interface{}, lang ...string) string {
	s := strings.ToUpper(strings.TrimSpace(str(iso)))
	m := isoDurationRe.FindStringSubmatch(s)
	if m == nil || s == "P" || s == "PT" {
		return str(iso)
	}
	code := "en"
	if len(lang) > 0 {
		code, _, _ = strings.Cut(strings.ToLower(lang[0]), "-")
	}
	units, ok := durationUnits[code]
	if !ok {
		units = durationUnits["en"]
	}
	var parts []string
	for i, v := range m[1:] {
		if v == "" {
			continue
		}
		if n, err := strconv.ParseFloat(v, 64); err == nil && n == 0 {
			continue
		}
		parts = append(parts, v+" "+units[i])
	}
	if len(parts) == 0 {
		return "0 " + units[2]
	}
	return strings.Join(parts, " ")
}
