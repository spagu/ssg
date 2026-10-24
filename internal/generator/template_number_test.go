package generator

import "testing"

func TestFormatNumber(t *testing.T) {
	nbsp, narrow := " ", " "
	for _, tt := range []struct {
		n    interface{}
		lang string
		sep  []string
		want string
	}{
		{111381, "en", nil, "111,381"},
		{111381, "pl", nil, "111" + nbsp + "381"},
		{111381, "pl-PL", nil, "111" + nbsp + "381"},
		{111381, "de", nil, "111.381"},
		{111381, "fr", nil, "111" + narrow + "381"},
		{111381, "hi", nil, "1,11,381"},
		{int64(123456789), "hi", nil, "12,34,56,789"},
		{111381, "xx", nil, "111,381"},
		{111381, "en", []string{" "}, "111 381"},
		{111381, "hi", []string{"."}, "111.381"},
		{1234.5, "en", nil, "1,234.5"},
		{1234.5, "de", nil, "1.234,5"},
		{float64(111381), "en", nil, "111,381"},
		{-1234567, "en", nil, "-1,234,567"},
		{999, "en", nil, "999"},
		{0, "en", nil, "0"},
		{"12345", "en", nil, "12,345"},
		{"1e9", "en", nil, "1e9"},
		{"n/a", "en", nil, "n/a"},
		{nil, "en", nil, ""},
		{uint(1000), "en", nil, "1,000"},
		{uint64(1000), "en", nil, "1,000"},
		{int32(1000), "en", nil, "1,000"},
		{float32(1500), "en", nil, "1,500"},
	} {
		if got := formatNumber(tt.n, tt.lang, tt.sep...); got != tt.want {
			t.Errorf("formatNumber(%v, %q, %v) = %q, want %q", tt.n, tt.lang, tt.sep, got, tt.want)
		}
	}
}

// TestFormatNumberDefaultLanguage: the page's language, else the site's
// default_language, else English.
func TestFormatNumberDefaultLanguage(t *testing.T) {
	g := &Generator{config: Config{DefaultLanguage: "pl"}}
	if got := g.tmplFormatNumber(111381); got != "111 381" {
		t.Errorf("site default language: %q", got)
	}
	g.currentLang = "de"
	if got := g.tmplFormatNumber(111381); got != "111.381" {
		t.Errorf("page language wins: %q", got)
	}
	if got := (&Generator{}).tmplFormatNumber(111381); got != "111,381" {
		t.Errorf("nothing set is English: %q", got)
	}
}
