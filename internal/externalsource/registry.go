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

// Load resolves the configuration and loads every source, up to
// max_concurrent_sources at a time. Results and warnings keep the
// deterministic name-sorted order regardless of completion order. A required
// source's failure aborts the build; an optional source's failure becomes a
// warning and the source is skipped.
func Load(cfg Config) (*Registry, []string, error) {
	if !cfg.Enabled {
		return &Registry{Results: map[string]*Result{}}, nil, nil
	}
	sources, err := Resolve(cfg)
	if err != nil {
		return nil, nil, err
	}

	fileConn := FileConnector{}
	httpConn := newHTTPConnector(cfg)
	results := make([]*Result, len(sources))
	errs := make([]error, len(sources))

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
			var conn connector = fileConn
			if src.Type == "http" {
				conn = httpConn
			}
			results[i], errs[i] = conn.Load(src)
		}(i, src)
	}
	wg.Wait()

	reg := &Registry{Results: make(map[string]*Result, len(sources))}
	var warnings []string
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
