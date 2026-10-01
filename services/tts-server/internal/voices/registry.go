package voices

import "sync"

// Registry holds the current Catalog and swaps it atomically on Reload, so
// requests in flight keep the snapshot they started with.
type Registry struct {
	mu   sync.RWMutex
	cat  Catalog
	load func() (Catalog, error)
}

// NewRegistry loads the first catalog; it fails if that load fails.
func NewRegistry(load func() (Catalog, error)) (*Registry, error) {
	r := &Registry{load: load}
	return r, r.Reload()
}

// Reload rebuilds the catalog. On error the previous catalog stays active.
func (r *Registry) Reload() error {
	cat, err := r.load()
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.cat = cat
	r.mu.Unlock()
	return nil
}

// Catalog returns the current snapshot.
func (r *Registry) Catalog() Catalog {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cat
}
