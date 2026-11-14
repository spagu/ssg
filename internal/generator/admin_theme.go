package generator

// The admin panels' look (GO-152): `admin_theme:` is validated and turned
// into a full palette by internal/admintheme, travels to the core worker in
// the module registry's `theme` field, and is written as /core/theme.css —
// over the empty file the core template ships, after the workers' public
// files are copied, so it is core's own file and never an output clash
// (#420). Without `admin_theme:` nothing here writes anything, and a site
// without the core worker builds byte for byte as before.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/admintheme"
	"github.com/spagu/ssg/internal/modules"
)

// adminThemeCSS is where the build writes the theme, in the output: next to
// the core worker's public files (its panel is /core/).
const adminThemeCSS = "core/theme.css"

// adminTheme validates `admin_theme:` and computes it when the site has a
// core worker. A value that cannot be used is a configuration error with or
// without core; without core the section has no effect, which is a finding.
func (g *Generator) adminTheme(core bool) (*modules.Theme, []byte, error) {
	if g.config.AdminTheme == nil {
		return nil, nil, nil
	}
	file := g.configFileName()
	res, err := admintheme.Build(file, *g.config.AdminTheme, g.inOutput)
	if err != nil {
		return nil, nil, err
	}
	var found []finding
	for _, f := range res.Findings {
		found = append(found, finding{file: file, detail: "admin_theme." + f.Key + ": " + f.Detail})
	}
	if !core {
		found = []finding{{file: file, detail: fmt.Sprintf(
			"admin_theme: is set, but no worker in workers: is the core worker (a module.json with id %q) — the panels' look has no effect", modules.CoreID)}}
	}
	if mode := g.adminThemeCheckMode(); mode != "" {
		sortFindings(found)
		if err := g.report(found, mode, "admin_theme", "admin theme valid", "%d admin theme problem(s)"); err != nil {
			return nil, nil, err
		}
	}
	return res.Theme, res.CSS, nil
}

// adminThemeCheckMode resolves check_admin_theme like check_modules: warn by
// default, since it only has findings on a site that set admin_theme.
func (g *Generator) adminThemeCheckMode() string {
	switch g.config.CheckAdminTheme {
	case "":
		return g.resolveMode("warn")
	case "off":
		return g.resolveMode("")
	}
	return g.resolveMode(g.config.CheckAdminTheme)
}

// inOutput reports whether a site path (/img/logo.svg) is a file this build
// has written.
func (g *Generator) inOutput(p string) bool {
	fi, err := os.Stat(filepath.Join(g.config.OutputDir, filepath.FromSlash(strings.TrimPrefix(p, "/"))))
	return err == nil && fi.Mode().IsRegular()
}

// writeAdminThemeCSS writes the computed theme.css; nil keeps the shipped
// empty file.
func (g *Generator) writeAdminThemeCSS(css []byte) error {
	if css == nil {
		return nil
	}
	path := filepath.Join(g.config.OutputDir, filepath.FromSlash(adminThemeCSS))
	if err := g.ensureParent(path); err != nil {
		return fmt.Errorf("admin theme: %w", err)
	}
	// #nosec G306 -- a stylesheet the panels load
	if err := os.WriteFile(path, css, 0644); err != nil {
		return fmt.Errorf("admin theme: %w", err)
	}
	g.log("   🎨 admin theme → " + adminThemeCSS)
	return nil
}
