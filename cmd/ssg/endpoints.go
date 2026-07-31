package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/externalsource"
)

// endpointHandler routes requests whose path matches a configured endpoint to a
// native handler, falling through to next (the static file server) for
// everything else (#63). This is the self-hosted execution of the vendor-neutral
// endpoints: declaration — the same endpoints the adapters compile to platform
// functions run here in the single Go binary, no external runtime. A no-op when
// no endpoints are declared, so a pure-static server is unchanged.
func endpointHandler(cfg *config.Config, next http.Handler) http.Handler {
	if len(cfg.Endpoints) == 0 {
		return next
	}
	routes := make(map[string]http.Handler, len(cfg.Endpoints))
	for _, ep := range cfg.Endpoints {
		h, err := buildEndpoint(ep)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  endpoint %q: %v (skipped)\n", ep.Path, err)
			continue
		}
		routes[ep.Path] = h
	}
	if len(routes) == 0 {
		return next
	}
	if !cfg.Quiet {
		fmt.Printf("   🔌 Serving %d endpoint(s)\n", len(routes))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, ok := routes[r.URL.Path]; ok {
			w.Header().Set("Cache-Control", "no-store")
			h.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// buildEndpoint compiles one endpoint declaration into an http.Handler.
func buildEndpoint(ep config.Endpoint) (http.Handler, error) {
	if !strings.HasPrefix(ep.Path, "/") {
		return nil, fmt.Errorf("path must start with '/'")
	}
	switch ep.Type {
	case "redirect":
		return redirectEndpoint(ep)
	case "proxy":
		return proxyEndpoint(ep)
	default:
		return nil, fmt.Errorf("unknown type %q (want redirect or proxy)", ep.Type)
	}
}

// redirectEndpoint issues a server-side redirect — the dynamic complement to the
// static _redirects file.
func redirectEndpoint(ep config.Endpoint) (http.Handler, error) {
	if ep.To == "" {
		return nil, fmt.Errorf("redirect needs a 'to'")
	}
	status := ep.Status
	if status == 0 {
		status = http.StatusFound // 302
	}
	if status < 300 || status > 399 {
		return nil, fmt.Errorf("redirect status %d is not a 3xx", status)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, ep.To, status)
	}), nil
}

// proxyEndpoint forwards the request to an upstream, keeping the upstream's
// credentials server-side. The transport resolves and vets the upstream IP
// itself, refusing private/loopback ranges unless allow_private is set, so a
// proxy endpoint can't be turned into an SSRF pivot (reuses the external-source
// guard). The client's path is dropped in favour of the target's path.
func proxyEndpoint(ep config.Endpoint) (http.Handler, error) {
	if ep.Target == "" {
		return nil, fmt.Errorf("proxy needs a 'target'")
	}
	target, err := url.Parse(ep.Target)
	if err != nil || target.Host == "" {
		return nil, fmt.Errorf("proxy target %q is not an absolute URL", ep.Target)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, fmt.Errorf("proxy target scheme %q must be http or https", target.Scheme)
	}
	allowed := methodSet(ep.Methods)
	rp := &httputil.ReverseProxy{
		Transport: externalsource.SecureTransport(ep.AllowPrivate),
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = target.Path // exact upstream path; client query is kept
			req.Host = target.Host
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowed != nil && !allowed[r.Method] {
			w.Header().Set("Allow", strings.Join(ep.Methods, ", "))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rp.ServeHTTP(w, r)
	}), nil
}

// methodSet builds an upper-cased allowlist; nil means any method is allowed.
func methodSet(methods []string) map[string]bool {
	if len(methods) == 0 {
		return nil
	}
	set := make(map[string]bool, len(methods))
	for _, m := range methods {
		set[strings.ToUpper(strings.TrimSpace(m))] = true
	}
	return set
}
