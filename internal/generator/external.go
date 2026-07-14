package generator

import (
	"fmt"
	"html/template"

	"github.com/spagu/ssg/internal/externalsource"
)

// This file wires the unified external source system (phase 1 of
// audit/ssg-external-sources-implementation-plan.md) into the build:
// loading, the .ExternalData/.ExternalDataMeta template namespaces and the
// getExternal/getExternalMeta helpers. The legacy .Data namespace is untouched.

// loadExternalSources loads every configured source once per build. Required
// sources abort the build on failure; optional ones warn and are skipped.
// --clear-external-cache wipes the shared disk cache first.
func (g *Generator) loadExternalSources() error {
	if g.config.ExternalSources.ClearCache {
		if err := externalsource.ClearCache(g.config.ExternalSources.CacheDir); err != nil {
			return fmt.Errorf("clearing external-source cache: %w", err)
		}
	}
	registry, warnings, err := externalsource.Load(g.config.ExternalSources)
	for _, w := range warnings {
		fmt.Printf("   ⚠️  Warning: %s\n", w)
	}
	if err != nil {
		return err
	}
	g.externalData = registry.Data()
	g.externalMeta = registry.Meta()
	if !g.config.Quiet {
		for _, name := range registry.Order {
			meta := registry.Results[name].Metadata
			fmt.Printf("   🔌 %s (%s, %s, %d records)\n", name, meta.SourceType, meta.ContentType, meta.RecordCount)
		}
	}
	return nil
}

// externalFuncs exposes the external-source template helpers.
func (g *Generator) externalFuncs() template.FuncMap {
	return template.FuncMap{
		// getExternal "products" → the source's parsed data (nil when absent).
		"getExternal": func(name string) interface{} {
			return g.externalData[name]
		},
		// getExternalMeta "products" → Metadata (zero value when absent).
		"getExternalMeta": func(name string) externalsource.Metadata {
			return g.externalMeta[name]
		},
	}
}
