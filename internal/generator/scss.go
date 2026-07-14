// Optional SCSS/Sass compilation (ASSET-003). When `scss: true`, every
// non-partial *.scss file in the output tree is compiled to a sibling *.css via
// the dart-sass CLI, then all *.scss sources (partials included) are removed so
// they never ship. The compiled CSS flows into the regular bundling → minify →
// fingerprint pipeline. dart-sass is an optional system dependency, mirroring
// the cwebp philosophy: when the binary is absent the step is skipped with a
// clear warning instead of failing the build.
package generator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// sassBinary resolves the dart-sass executable: the configured sass_binary
// path wins, otherwise "sass" is looked up in PATH. Returns "" when absent.
func (g *Generator) sassBinary() string {
	if g.config.SassBinary != "" {
		if _, err := os.Stat(g.config.SassBinary); err == nil {
			return g.config.SassBinary
		}
		return ""
	}
	// NOSONAR S4036: sass is an optional system tool intentionally resolved from PATH (portable), like cwebp
	path, err := exec.LookPath("sass")
	if err != nil {
		return ""
	}
	return path
}

// compileSCSSIfRequested compiles *.scss in the output directory when enabled.
// Partials (_name.scss) are never compiled on their own — dart-sass resolves
// them through @use/@import — and every .scss file is removed from the output
// afterwards so sources do not ship.
func (g *Generator) compileSCSSIfRequested() error {
	if !g.config.SCSS {
		return nil
	}
	bin := g.sassBinary()
	if bin == "" {
		fmt.Println("   ⚠️  dart-sass not installed — skipping SCSS compilation (install `sass` or set sass_binary)")
		return nil
	}
	g.log("🎨 Compiling SCSS...")

	var scssFiles []string
	err := filepath.Walk(g.config.OutputDir, func(path string, info os.FileInfo, walkErr error) error { // #nosec G703 -- CLI walks its own output
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		if strings.EqualFold(filepath.Ext(path), ".scss") {
			scssFiles = append(scssFiles, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("scanning for SCSS: %w", err)
	}

	for _, src := range scssFiles {
		if strings.HasPrefix(filepath.Base(src), "_") {
			continue // partial: only ever pulled in via @use/@import
		}
		if err := compileSCSSFile(bin, src); err != nil {
			return err
		}
	}
	// Remove every .scss source (partials included) from the shipped output.
	for _, src := range scssFiles {
		if err := os.Remove(src); err != nil {
			fmt.Printf("   ⚠️  Warning: couldn't remove SCSS source %s: %v\n", src, err)
		}
	}
	return nil
}

// compileSCSSFile shells out to dart-sass for one entrypoint, writing the
// sibling .css. Paths are passed through webp's SafeArgPath hardening pattern
// so a filename can never be parsed as a CLI flag (SEC-011).
func compileSCSSFile(bin, src string) error {
	dst := strings.TrimSuffix(src, filepath.Ext(src)) + ".css"
	// #nosec G204 -- fixed optional tool (dart-sass); only sanitized paths vary (SEC-011)
	cmd := exec.Command(bin, "--no-source-map", safeSassArg(src), safeSassArg(dst)) // NOSONAR S4036: resolved via sassBinary
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sass %s: %v: %s", filepath.Base(src), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// safeSassArg prefixes relative paths with "./" so they are unambiguously
// paths, never options — the SEC-011 hardening shared with cwebp.
func safeSassArg(p string) string {
	if p == "" || filepath.IsAbs(p) || strings.HasPrefix(p, ".") {
		return p
	}
	return "./" + p
}
