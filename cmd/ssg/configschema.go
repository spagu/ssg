package main

// `ssg config schema` (GO-177): every configuration key with its type,
// default, description, form group and class, as JSON — what designer-ui
// builds its options tab from. See internal/config/schema.go.

import (
	"fmt"
	"os"
	"strings"

	"github.com/spagu/ssg/internal/config"
)

// runConfigSchema prints the configuration schema.
func runConfigSchema(args []string) int {
	if len(args) > 0 {
		errf("❌ usage: ssg config schema (takes no arguments)\n")
		return 2
	}
	_, _ = os.Stdout.Write(config.SchemaJSON())
	return 0
}

// noteOverlayKeys tells, on stderr, which values of the effective config came
// from the designer overlay — stderr, so `ssg config view --effective <key>`
// still prints only the value.
func noteOverlayKeys(configPath string) {
	keys, err := config.OverlayKeys(configPath)
	if err != nil || len(keys) == 0 {
		return // Load already reported an unreadable overlay
	}
	errf("ℹ️  from %s: %s\n", config.OverlayFileName, strings.Join(keys, ", "))
}

// configSignature hashes the config file and its overlay together, so the
// watcher rebuilds when either changes.
func configSignature(configPath string) string {
	return fmt.Sprintf("%s|%s", fileSignature(configPath), fileSignature(config.OverlayPath(configPath)))
}
