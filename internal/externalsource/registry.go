package externalsource

import (
	"fmt"
)

// Registry holds every loaded source for one build. Loading happens exactly
// once per build (the in-memory cache of phase 1); the disk cache arrives with
// the HTTP connector in phase 2.
type Registry struct {
	Order   []string
	Results map[string]*Result
}

// Load resolves the configuration and loads every source in deterministic
// order. A required source's failure aborts the build; an optional source's
// failure is returned as a warning and the source is skipped.
func Load(cfg Config) (*Registry, []string, error) {
	if !cfg.Enabled {
		return &Registry{Results: map[string]*Result{}}, nil, nil
	}
	sources, err := Resolve(cfg)
	if err != nil {
		return nil, nil, err
	}
	reg := &Registry{Results: make(map[string]*Result, len(sources))}
	var warnings []string
	connector := FileConnector{}
	for _, src := range sources {
		result, err := connector.Load(src)
		if err != nil {
			if src.Required {
				return nil, warnings, err
			}
			warnings = append(warnings, fmt.Sprintf("optional %v", err))
			continue
		}
		reg.Order = append(reg.Order, src.Name)
		reg.Results[src.Name] = result
	}
	return reg, warnings, nil
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
