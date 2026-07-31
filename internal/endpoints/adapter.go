// Package endpoints compiles the vendor-neutral endpoints: declaration (#63) into
// platform-specific serverless functions. Each platform is a self-contained
// adapter in its own file, registered via Register in init() — adding a platform
// is one new file, and nothing else in the build changes. The same endpoint
// declarations run natively in the built-in server (self-hosted), so the format
// is the single contract regardless of where an endpoint executes.
package endpoints

import (
	"fmt"
	"sort"

	"github.com/spagu/ssg/internal/config"
)

// Adapter emits one platform's functions from the shared endpoint declarations.
type Adapter interface {
	// Platform is the deploy-target name this adapter serves (e.g. "cloudflare").
	Platform() string
	// Emit writes the platform's function files under outDir and returns the
	// paths written (relative to outDir), or an error.
	Emit(eps []config.Endpoint, outDir string) ([]string, error)
}

// registry holds the adapters registered at init time (the plugin table).
var registry = map[string]Adapter{}

// Register adds an adapter; called from each adapter file's init().
func Register(a Adapter) { registry[a.Platform()] = a }

// For returns the adapter for a platform name, if one is registered.
func For(platform string) (Adapter, bool) {
	a, ok := registry[platform]
	return a, ok
}

// Platforms lists the registered platform names, sorted, for diagnostics.
func Platforms() []string {
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Emit compiles the endpoints for the named platform into outDir. It is a no-op
// when no endpoints are declared; an unknown platform is an error that lists the
// platforms this binary knows.
func Emit(platform string, eps []config.Endpoint, outDir string) ([]string, error) {
	if len(eps) == 0 {
		return nil, nil
	}
	a, ok := For(platform)
	if !ok {
		return nil, fmt.Errorf("unknown endpoints platform %q (known: %v)", platform, Platforms())
	}
	return a.Emit(eps, outDir)
}
