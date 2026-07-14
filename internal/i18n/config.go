// Package i18n contains the opt-in multilingual configuration and runtime
// primitives shared by the config loader and generator.
package i18n

import (
	"fmt"
	"strings"
	"time"
)

type LanguageConfig struct {
	Code     string `yaml:"code" toml:"code" json:"code"`
	Locale   string `yaml:"locale" toml:"locale" json:"locale"`
	Name     string `yaml:"name" toml:"name" json:"name"`
	Timezone string `yaml:"timezone" toml:"timezone" json:"timezone"`
}

type Config struct {
	Enabled               bool                `yaml:"enabled" toml:"enabled" json:"enabled"`
	PrefixDefaultLanguage bool                `yaml:"prefix_default_language" toml:"prefix_default_language" json:"prefix_default_language"`
	TranslationsDir       string              `yaml:"translations_dir" toml:"translations_dir" json:"translations_dir"`
	DictionaryFallback    bool                `yaml:"dictionary_fallback" toml:"dictionary_fallback" json:"dictionary_fallback"`
	ContentFallback       bool                `yaml:"content_fallback" toml:"content_fallback" json:"content_fallback"`
	MissingTranslation    string              `yaml:"missing_translation" toml:"missing_translation" json:"missing_translation"`
	InvalidLanguage       string              `yaml:"invalid_language" toml:"invalid_language" json:"invalid_language"`
	DuplicateTranslation  string              `yaml:"duplicate_translation" toml:"duplicate_translation" json:"duplicate_translation"`
	FallbackLanguages     map[string][]string `yaml:"fallback_languages" toml:"fallback_languages" json:"fallback_languages"`
}

func (c Config) WithDefaults() Config {
	if c.TranslationsDir == "" {
		c.TranslationsDir = "i18n"
	}
	if c.MissingTranslation == "" {
		c.MissingTranslation = "warn"
	}
	if c.InvalidLanguage == "" {
		c.InvalidLanguage = "fail"
	}
	if c.DuplicateTranslation == "" {
		c.DuplicateTranslation = "fail"
	}
	return c
}

// Normalize preserves the compact format while enriching it with locale/name/timezone data.
func Normalize(codes []string, expanded []LanguageConfig, legacyTZ map[string]string) []LanguageConfig {
	if len(expanded) > 0 {
		out := append([]LanguageConfig(nil), expanded...)
		for i := range out {
			if out[i].Locale == "" {
				out[i].Locale = out[i].Code
			}
			if out[i].Name == "" {
				out[i].Name = out[i].Code
			}
			if out[i].Timezone == "" {
				out[i].Timezone = legacyTZ[out[i].Code]
			}
		}
		return out
	}
	out := make([]LanguageConfig, 0, len(codes))
	for _, code := range codes {
		out = append(out, LanguageConfig{Code: code, Locale: code, Name: code, Timezone: legacyTZ[code]})
	}
	return out
}

func Validate(langs []LanguageConfig, defaultLang string, cfg Config) error {
	if !cfg.Enabled {
		return nil
	}
	if len(langs) == 0 {
		return fmt.Errorf("i18n enabled but no languages are configured")
	}
	known := make(map[string]bool, len(langs))
	for _, l := range langs {
		if strings.TrimSpace(l.Code) == "" {
			return fmt.Errorf("i18n language code cannot be empty")
		}
		if known[l.Code] {
			return fmt.Errorf("duplicate i18n language code %q", l.Code)
		}
		known[l.Code] = true
		if l.Timezone != "" {
			if _, err := time.LoadLocation(l.Timezone); err != nil {
				return fmt.Errorf("invalid timezone %q for language %q: %w", l.Timezone, l.Code, err)
			}
		}
	}
	if defaultLang == "" || !known[defaultLang] {
		return fmt.Errorf("default_language %q is not a configured language", defaultLang)
	}
	if !oneOf(cfg.MissingTranslation, "error", "warn", "fallback", "empty") {
		return fmt.Errorf("unsupported i18n missing_translation policy %q", cfg.MissingTranslation)
	}
	if !oneOf(cfg.InvalidLanguage, "fail", "warn") {
		return fmt.Errorf("unsupported i18n invalid_language policy %q", cfg.InvalidLanguage)
	}
	if !oneOf(cfg.DuplicateTranslation, "fail", "warn") {
		return fmt.Errorf("unsupported i18n duplicate_translation policy %q", cfg.DuplicateTranslation)
	}
	for from, tos := range cfg.FallbackLanguages {
		if !known[from] {
			return fmt.Errorf("fallback source language %q is not configured", from)
		}
		for _, to := range tos {
			if !known[to] {
				return fmt.Errorf("fallback language %q referenced by %q is not configured", to, from)
			}
		}
	}
	state := map[string]uint8{}
	var visit func(string) error
	visit = func(lang string) error {
		if state[lang] == 1 {
			return fmt.Errorf("i18n fallback cycle includes %q", lang)
		}
		if state[lang] == 2 {
			return nil
		}
		state[lang] = 1
		for _, next := range cfg.FallbackLanguages[lang] {
			if err := visit(next); err != nil {
				return err
			}
		}
		state[lang] = 2
		return nil
	}
	for code := range known {
		if err := visit(code); err != nil {
			return err
		}
	}
	return nil
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func Language(langs []LanguageConfig, code string) (LanguageConfig, bool) {
	for _, l := range langs {
		if l.Code == code {
			return l, true
		}
	}
	return LanguageConfig{}, false
}

func Prefix(code, defaultLang string, cfg Config) string {
	if !cfg.Enabled || code == "" || (code == defaultLang && !cfg.PrefixDefaultLanguage) {
		return ""
	}
	return code
}
