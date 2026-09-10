package externalsource

import (
	"errors"
	"fmt"
	"sync"
)

// Registry holds every loaded source for one build. Loading happens exactly
// once per build (the in-memory cache); HTTP sources additionally use the
// shared disk cache between builds.
type Registry struct {
	Order   []string
	Results map[string]*Result
}

// connector loads one resolved source.
type connector interface {
	Load(src Source) (*Result, error)
}

// connectorFor picks the connector one source needs. File is the default,
// because a path is what a source is unless it says otherwise.
func connectorFor(src Source, fileConn FileConnector, httpConn connector) connector {
	switch src.Type {
	case "http":
		return httpConn
	case "sql":
		return SQLConnector{}
	case "cms":
		return CMSConnector{}
	}
	return fileConn
}

// mapRecordsIfContent turns records into pages for every source type that is
// not a CMS — the CMS connector produces its own import (GO-098).
func mapRecordsIfContent(src Source, result *Result) ([]string, error) {
	if src.Mode != "content" || src.Type == "cms" {
		return nil, nil
	}
	imported, warnings, err := MapRecords(src, result.Data)
	if err != nil {
		return nil, fail(src, "content_map", err)
	}
	result.CMS = imported
	result.Metadata.RecordCount = len(imported.Pages) + len(imported.Posts)
	return warnings, nil
}

// Load resolves the configuration and loads every source, up to
// max_concurrent_sources at a time. Results and warnings keep the
// deterministic name-sorted order regardless of completion order. A required
// source's failure aborts the build; an optional source's failure becomes a
// warning and the source is skipped.
func Load(cfg Config) (*Registry, []string, error) {
	if !cfg.Enabled {
		return &Registry{Results: map[string]*Result{}}, nil, nil
	}
	sources, warnings, err := resolveAll(cfg)
	if err != nil {
		return nil, warnings, err
	}

	fileConn := FileConnector{}
	httpConn := newHTTPConnector(cfg)
	results := make([]*Result, len(sources))
	errs := make([]error, len(sources))
	mapWarnings := make([][]string, len(sources))

	limit := cfg.MaxConcurrent
	if limit <= 0 {
		limit = defaultConcurrency
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, src := range sources {
		wg.Add(1)
		go func(i int, src Source) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i], errs[i] = connectorFor(src, fileConn, httpConn).Load(src)
			if errs[i] == nil {
				mapWarnings[i], errs[i] = mapRecordsIfContent(src, results[i])
				if errs[i] != nil {
					results[i] = nil
				}
			}
		}(i, src)
	}
	wg.Wait()

	reg := &Registry{Results: make(map[string]*Result, len(sources))}
	for i, src := range sources {
		if errs[i] != nil {
			// fail_on_cache_miss: false downgrades offline misses to warnings
			// even for required sources (plan §Cache).
			if src.Required && !errors.Is(errs[i], errCacheMissSkip) {
				return nil, warnings, errs[i]
			}
			warnings = append(warnings, fmt.Sprintf("optional %v", errs[i]))
			continue
		}
		reg.Order = append(reg.Order, src.Name)
		reg.Results[src.Name] = results[i]
		warnings = append(warnings, mapWarnings[i]...)
	}
	return reg, warnings, nil
}

// newHTTPConnector wires the global cache/offline/refresh policy into the
// HTTP connector.
func newHTTPConnector(cfg Config) HTTPConnector {
	dir := cfg.CacheDir
	if dir == "" {
		dir = DefaultCacheDir
	}
	staleIfError := true
	if cfg.StaleIfError != nil {
		staleIfError = *cfg.StaleIfError
	}
	failOnMiss := true
	if cfg.FailOnCacheMiss != nil {
		failOnMiss = *cfg.FailOnCacheMiss
	}
	return HTTPConnector{
		cache:        diskCache{dir: dir},
		allowedHosts: cfg.AllowedHosts,
		offline:      cfg.Offline,
		refresh:      cfg.Refresh,
		refreshOnly:  cfg.Only,
		staleIfError: staleIfError,
		failOnMiss:   failOnMiss,
	}
}

// CMSImports returns the content-mode CMS imports in deterministic order for
// the generator to merge into the site.
func (r *Registry) CMSImports() []*CMSImportResult {
	var out []*CMSImportResult
	for _, name := range r.Order {
		if res := r.Results[name]; res.CMS != nil {
			out = append(out, res.CMS)
		}
	}
	return out
}

// Data returns the template-facing .ExternalData namespace.
func (r *Registry) Data() map[string]interface{} {
	out := make(map[string]interface{}, len(r.Results))
	for name, res := range r.Results {
		out[name] = res.Data
	}
	return out
}

// Meta returns the template-facing .ExternalDataMeta namespace.
func (r *Registry) Meta() map[string]Metadata {
	out := make(map[string]Metadata, len(r.Results))
	for name, res := range r.Results {
		out[name] = res.Metadata
	}
	return out
}
