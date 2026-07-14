package i18n

import (
	"strings"
	"testing"
)

func TestValidateAndPrefix(t *testing.T) {
	langs := []LanguageConfig{{Code: "pl", Locale: "pl-PL"}, {Code: "en", Locale: "en-GB"}}
	cfg := Config{Enabled: true, FallbackLanguages: map[string][]string{"pl": {"en"}}}.WithDefaults()
	if err := Validate(langs, "pl", cfg); err != nil {
		t.Fatal(err)
	}
	if got := Prefix("pl", "pl", cfg); got != "" {
		t.Fatalf("default prefix = %q", got)
	}
	if got := Prefix("en", "pl", cfg); got != "en" {
		t.Fatalf("secondary prefix = %q", got)
	}
	cfg.PrefixDefaultLanguage = true
	if got := Prefix("pl", "pl", cfg); got != "pl" {
		t.Fatalf("prefixed default = %q", got)
	}
}

func TestValidateRejectsFallbackCycle(t *testing.T) {
	langs := []LanguageConfig{{Code: "pl"}, {Code: "en"}}
	cfg := Config{Enabled: true, FallbackLanguages: map[string][]string{"pl": {"en"}, "en": {"pl"}}}.WithDefaults()
	if err := Validate(langs, "pl", cfg); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestNormalizeExpandedAndDefaults(t *testing.T) {
	out := Normalize(nil, []LanguageConfig{{Code: "pl"}, {Code: "en", Locale: "en-GB", Name: "English", Timezone: "Europe/London"}},
		map[string]string{"pl": "Europe/Warsaw"})
	if out[0].Locale != "pl" || out[0].Name != "pl" || out[0].Timezone != "Europe/Warsaw" {
		t.Errorf("expanded defaults = %+v", out[0])
	}
	if out[1].Locale != "en-GB" || out[1].Timezone != "Europe/London" {
		t.Errorf("explicit values overridden = %+v", out[1])
	}
	compact := Normalize([]string{"de"}, nil, nil)
	if len(compact) != 1 || compact[0].Code != "de" || compact[0].Locale != "de" {
		t.Errorf("compact = %+v", compact)
	}
}

func TestValidateMatrix(t *testing.T) {
	langs := []LanguageConfig{{Code: "pl"}, {Code: "en"}}
	base := Config{Enabled: true}.WithDefaults()
	cases := []struct {
		name    string
		langs   []LanguageConfig
		def     string
		cfg     Config
		wantErr string
	}{
		{"disabled is nil", nil, "", Config{}, ""},
		{"no languages", nil, "pl", base, "no languages"},
		{"empty code", []LanguageConfig{{Code: " "}}, "pl", base, "cannot be empty"},
		{"duplicate code", []LanguageConfig{{Code: "pl"}, {Code: "pl"}}, "pl", base, "duplicate"},
		{"bad timezone", []LanguageConfig{{Code: "pl", Timezone: "Mars/Base"}}, "pl", base, "invalid timezone"},
		{"default not configured", langs, "fr", base, "not a configured language"},
		{"bad missing policy", langs, "pl", Config{Enabled: true, MissingTranslation: "explode", InvalidLanguage: "fail", DuplicateTranslation: "fail"}, "missing_translation"},
		{"bad invalid policy", langs, "pl", Config{Enabled: true, MissingTranslation: "warn", InvalidLanguage: "explode", DuplicateTranslation: "fail"}, "invalid_language"},
		{"bad duplicate policy", langs, "pl", Config{Enabled: true, MissingTranslation: "warn", InvalidLanguage: "fail", DuplicateTranslation: "explode"}, "duplicate_translation"},
		{"fallback source unknown", langs, "pl", withFallback(base, map[string][]string{"fr": {"en"}}), "not configured"},
		{"fallback target unknown", langs, "pl", withFallback(base, map[string][]string{"pl": {"fr"}}), "not configured"},
		{"fallback cycle", langs, "pl", withFallback(base, map[string][]string{"pl": {"en"}, "en": {"pl"}}), "cycle"},
		{"valid", langs, "pl", withFallback(base, map[string][]string{"pl": {"en"}}), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.langs, c.def, c.cfg)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want contains %q", err, c.wantErr)
			}
		})
	}
}

func withFallback(c Config, fb map[string][]string) Config {
	c.FallbackLanguages = fb
	return c
}

func TestLanguageAndPrefix(t *testing.T) {
	langs := []LanguageConfig{{Code: "pl", Name: "Polski"}}
	if l, ok := Language(langs, "pl"); !ok || l.Name != "Polski" {
		t.Errorf("Language(pl) = %+v, %v", l, ok)
	}
	if _, ok := Language(langs, "xx"); ok {
		t.Error("unknown language must not resolve")
	}
	on := Config{Enabled: true}
	if Prefix("pl", "pl", on) != "" || Prefix("en", "pl", on) != "en" {
		t.Error("prefix defaults broken")
	}
	on.PrefixDefaultLanguage = true
	if Prefix("pl", "pl", on) != "pl" {
		t.Error("prefix_default_language ignored")
	}
	if Prefix("en", "pl", Config{}) != "" {
		t.Error("disabled i18n must not prefix")
	}
	if Prefix("", "pl", on) != "" {
		t.Error("empty code must not prefix")
	}
}

func TestWithDefaults(t *testing.T) {
	d := Config{}.WithDefaults()
	if d.TranslationsDir != "i18n" || d.MissingTranslation != "warn" ||
		d.InvalidLanguage != "fail" || d.DuplicateTranslation != "fail" {
		t.Errorf("defaults = %+v", d)
	}
	keep := Config{TranslationsDir: "x", MissingTranslation: "error", InvalidLanguage: "warn", DuplicateTranslation: "warn"}.WithDefaults()
	if keep.TranslationsDir != "x" || keep.MissingTranslation != "error" {
		t.Errorf("explicit values overridden = %+v", keep)
	}
}
