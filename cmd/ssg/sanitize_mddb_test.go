package main

import (
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// TestCreateGeneratorConfigSanitizesMddb covers SEC-003: an MDDB build hands
// the generator a sanitizer unless the server is marked trusted.
func TestCreateGeneratorConfigSanitizesMddb(t *testing.T) {
	cfg := &config.Config{Mddb: config.MddbConfig{Enabled: true}}
	if !createGeneratorConfig(cfg).SanitizeHTML {
		t.Error("an untrusted MDDB build must sanitize")
	}
	cfg.Mddb.Trusted = true
	if createGeneratorConfig(cfg).SanitizeHTML {
		t.Error("mddb.trusted must skip the default sanitizer")
	}
}
