package main

// Wiring for the reader's page tools (1.8.72): `print:` and `cook_mode:`
// become the generator's options.

import (
	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// buildPrintOptions turns `print:` into generator options.
func buildPrintOptions(p config.PrintConfig) generator.PrintOptions {
	return generator.PrintOptions{Enabled: p.Enabled, Label: p.Label, Sections: p.Sections,
		NoAuto: p.Auto != nil && !*p.Auto, Labels: p.Labels}
}

// buildCookModeOptions turns `cook_mode:` into generator options.
func buildCookModeOptions(c config.CookModeConfig) generator.CookModeOptions {
	return generator.CookModeOptions{Enabled: c.Enabled, Label: c.Label,
		NoAuto: c.Auto != nil && !*c.Auto, Labels: c.Labels}
}
