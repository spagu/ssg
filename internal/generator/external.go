package generator

import (
	"fmt"
	"html/template"

	"github.com/spagu/ssg/internal/externalsource"
	"github.com/spagu/ssg/internal/models"
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
	g.cmsImports = registry.CMSImports()
	if !g.config.Quiet {
		for _, name := range registry.Order {
			meta := registry.Results[name].Metadata
			fmt.Printf("   🔌 %s (%s, %s, %d records)\n", name, meta.SourceType, meta.ContentType, meta.RecordCount)
		}
	}
	return nil
}

// mergeCMSContent appends content-mode CMS imports to the site before
// finalizeLoadedContent runs, so imported pages get the same URL, translation,
// taxonomy and collision treatment as native content. Authors merge without
// overwriting IDs the local metadata already defines.
func (g *Generator) mergeCMSContent() {
	for _, imp := range g.cmsImports {
		g.siteData.Pages = append(g.siteData.Pages, imp.Pages...)
		g.siteData.Posts = append(g.siteData.Posts, imp.Posts...)
		if g.siteData.Authors == nil {
			g.siteData.Authors = map[int]models.Author{}
		}
		for id, author := range imp.Authors {
			if _, exists := g.siteData.Authors[id]; !exists {
				g.siteData.Authors[id] = author
			}
		}
		if !g.config.Quiet {
			fmt.Printf("   🔌 Merged CMS content: %d pages, %d posts, %d authors\n",
				len(imp.Pages), len(imp.Posts), len(imp.Authors))
		}
	}
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
