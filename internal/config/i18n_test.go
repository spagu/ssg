package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExpandedLanguages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssg.yaml")
	data := []byte("languages:\n  - code: pl\n    locale: pl-PL\n    name: Polski\n  - code: en\n    locale: en-GB\ndefault_language: pl\ni18n:\n  enabled: true\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Languages) != 2 || cfg.Languages[1] != "en" {
		t.Fatalf("compact codes = %#v", cfg.Languages)
	}
	if len(cfg.LanguageConfigs) != 2 || cfg.LanguageConfigs[0].Locale != "pl-PL" {
		t.Fatalf("expanded = %#v", cfg.LanguageConfigs)
	}
	if !cfg.I18n.Enabled || cfg.I18n.TranslationsDir != "i18n" {
		t.Fatalf("i18n defaults = %#v", cfg.I18n)
	}
}
