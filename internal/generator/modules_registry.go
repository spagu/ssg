package generator

// The module registry (GO-144): when one of the site's workers is the `core`
// worker, the build collects the module.json of every worker it bundles and
// of every module declared under `modules:`, and writes the result where only
// the core Pages Function can read it — functions/_ssg-modules.json, which
// wrangler bundles into the function and never uploads as a static asset.
// Without a core worker nothing is written, so such a build is byte for byte
// what it was before.

import (
	"fmt"
	"os"
	"path/filepath"

	ssgroot "github.com/spagu/ssg"
	"github.com/spagu/ssg/internal/modules"
)

// writeModuleRegistry checks the `modules:` entries, and writes the registry
// when a core worker is among workers. seen maps the functions files the
// workers provide to their labels, so a worker shipping a file of the
// registry's name is refused instead of being overwritten.
func (g *Generator) writeModuleRegistry(workers []modules.Worker, seen map[string]string) error {
	file := g.configFileName()
	for i, r := range g.config.Modules {
		if err := modules.CheckRef(file, fmt.Sprintf("modules[%d]", i), r); err != nil {
			return err
		}
	}
	core := false
	for _, w := range workers {
		core = core || modules.IsCore(w.Dir)
	}
	mode := g.moduleCheckMode()
	theme, css, err := g.adminTheme(core)
	if err != nil {
		return err
	}
	if !core {
		if len(g.config.Modules) == 0 {
			return nil
		}
		return g.reportModules(mode, []modules.Finding{{File: file, Detail: fmt.Sprintf(
			"modules: lists %d module(s), but no worker in workers: is the core worker (a module.json with id %q) — the entries have no effect",
			len(g.config.Modules), modules.CoreID)}})
	}
	key := "functions/" + modules.RegistryFile
	if other, ok := seen[key]; ok {
		return fmt.Errorf("worker %s provides %s, a name the build writes the module registry to — rename that file", other, key)
	}
	data, findings, err := renderModuleRegistry(modules.Input{
		ConfigFile: file, Workers: workers, Refs: g.config.Modules, Templates: ssgroot.EmbeddedWorkers, Theme: theme,
	})
	if err != nil {
		return fmt.Errorf("module registry: %w", err)
	}
	if err := g.reportModules(mode, findings); err != nil {
		return err
	}
	// #nosec G306 -- read by wrangler when it bundles the core function
	if err := os.WriteFile(filepath.Join(g.config.OutputDir, filepath.FromSlash(key)), data, 0644); err != nil {
		return fmt.Errorf("module registry: %w", err)
	}
	g.log("   🧩 module registry → " + key)
	return g.writeAdminThemeCSS(css)
}

// renderModuleRegistry is modules.Render; a test replaces it to reach the
// error a broken embedded schema would cause.
var renderModuleRegistry = modules.Render

// moduleCheckMode resolves check_modules. Unlike most checks it warns by
// default: it only ever has findings on a site that uses modules, and there a
// module silently missing from the admin navigation is the failure to avoid.
func (g *Generator) moduleCheckMode() string {
	switch g.config.CheckModules {
	case "":
		return g.resolveMode("warn")
	case "off":
		return g.resolveMode("")
	}
	return g.resolveMode(g.config.CheckModules)
}

// reportModules prints findings (or fails a strict build) unless the check
// is off.
func (g *Generator) reportModules(mode string, found []modules.Finding) error {
	if mode == "" {
		return nil
	}
	findings := make([]finding, 0, len(found))
	for _, f := range found {
		findings = append(findings, finding{file: f.File, detail: f.Detail})
	}
	sortFindings(findings)
	return g.report(findings, mode, "modules", "module manifests valid", "%d module problem(s)")
}

// configFileName names the site's configuration in messages.
func (g *Generator) configFileName() string {
	if g.config.ConfigPath == "" {
		return ".ssg.yaml"
	}
	return g.config.ConfigPath
}

// workerKey names worker i the way the configuration does.
func (g *Generator) workerKey(i int) string {
	if len(g.config.Workers) > 0 {
		return fmt.Sprintf("workers[%d]", i)
	}
	return "worker"
}
