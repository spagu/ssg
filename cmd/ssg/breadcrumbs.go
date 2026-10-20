package main

// Wiring for `breadcrumbs:` (1.8.73, #335).

import (
	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// buildBreadcrumbOptions turns `breadcrumbs:` into generator options.
func buildBreadcrumbOptions(b config.BreadcrumbsConfig) generator.BreadcrumbOptions {
	opts := generator.BreadcrumbOptions{Off: b.Enabled != nil && !*b.Enabled, Home: b.Home}
	if len(b.Sections) > 0 {
		opts.Sections = make(map[string]generator.BreadcrumbSection, len(b.Sections))
		for seg, s := range b.Sections {
			opts.Sections[seg] = generator.BreadcrumbSection{Name: s.Name, URL: s.URL}
		}
	}
	return opts
}
