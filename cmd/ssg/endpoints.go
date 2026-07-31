package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/endpoints"
	"github.com/spagu/ssg/internal/externalsource"
)

// emitEndpoints compiles the configured endpoints: to the selected platform's
// functions in the output tree at build time (#63). A no-op when no platform is
// chosen (the endpoints run self-hosted via --http instead) or none are declared.
func emitEndpoints(cfg *config.Config) error {
	if cfg.EndpointsPlatform == "" || len(cfg.Endpoints) == 0 {
		return nil
	}
	written, err := endpoints.Emit(cfg.EndpointsPlatform, cfg.Endpoints, cfg.OutputDir)
	if err != nil {
		return fmt.Errorf("compiling endpoints for %s: %w", cfg.EndpointsPlatform, err)
	}
	if !cfg.Quiet {
		fmt.Printf("   🔌 Compiled %d endpoint(s) for %s\n", len(written), cfg.EndpointsPlatform)
	}
	return nil
}

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
	var guards []authGuard
	for _, ep := range cfg.Endpoints {
		if ep.Type == "auth" {
			g, err := buildAuthGuard(ep)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  endpoint %q: %v (skipped)\n", ep.Path, err)
				continue
			}
			guards = append(guards, g)
			continue
		}
		h, err := buildEndpoint(ep)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  endpoint %q: %v (skipped)\n", ep.Path, err)
			continue
		}
		routes[ep.Path] = h
	}
	if len(routes) == 0 && len(guards) == 0 {
		return next
	}
	if !cfg.Quiet {
		fmt.Printf("   🔌 Serving %d endpoint(s), %d auth guard(s)\n", len(routes), len(guards))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Auth guards run first so they cover both endpoints and static files
		// under their prefix.
		for _, g := range guards {
			if strings.HasPrefix(r.URL.Path, g.prefix) && !g.authorized(r) {
				w.Header().Set("WWW-Authenticate", `Basic realm="Protected"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if h, ok := routes[r.URL.Path]; ok {
			w.Header().Set("Cache-Control", "no-store")
			h.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authGuard protects a path prefix with HTTP Basic auth on the built-in server.
type authGuard struct {
	prefix, user, pass string
}

// authorized reports whether the request carries the guard's credentials, using
// constant-time comparison so a wrong user/password reveals nothing by timing.
func (g authGuard) authorized(r *http.Request) bool {
	u, p, ok := r.BasicAuth()
	if !ok {
		return false
	}
	userOK := subtle.ConstantTimeCompare([]byte(u), []byte(g.user)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(p), []byte(g.pass)) == 1
	return userOK && passOK
}

// buildAuthGuard resolves an auth endpoint into a guard. The password must come
// from an environment variable ($VAR) so a secret never lives in the config file.
func buildAuthGuard(ep config.Endpoint) (authGuard, error) {
	if !strings.HasPrefix(ep.Path, "/") {
		return authGuard{}, fmt.Errorf("path must start with '/'")
	}
	if ep.User == "" {
		return authGuard{}, fmt.Errorf("auth needs a 'user'")
	}
	pass := ep.Password
	if strings.HasPrefix(pass, "$") {
		pass = os.Getenv(strings.TrimPrefix(pass, "$"))
	}
	if pass == "" {
		return authGuard{}, fmt.Errorf("auth needs a 'password' (reference an env var, e.g. $MEMBERS_PW)")
	}
	return authGuard{prefix: ep.Path, user: ep.User, pass: pass}, nil
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
	case "form":
		return formEndpoint(ep)
	default:
		return nil, fmt.Errorf("unknown type %q (want redirect, proxy or form)", ep.Type)
	}
}

// formEndpoint accepts a POSTed submission, drops obvious bots via the honeypot,
// and delivers the collected fields as JSON to a webhook (To), keeping that
// webhook URL server-side. The delivery client uses the SSRF-hardened transport,
// so the webhook can't be pointed at an internal host unless allow_private is set.
func formEndpoint(ep config.Endpoint) (http.Handler, error) {
	if ep.To == "" {
		return nil, fmt.Errorf("form needs a 'to' (delivery webhook URL)")
	}
	to, err := url.Parse(ep.To)
	if err != nil || to.Host == "" || (to.Scheme != "http" && to.Scheme != "https") {
		return nil, fmt.Errorf("form 'to' %q must be an http(s) URL", ep.To)
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: externalsource.SecureTransport(ep.AllowPrivate)}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "could not parse form", http.StatusBadRequest)
			return
		}
		// Honeypot: a filled trap field is a bot — accept so it gets no signal,
		// but deliver nothing.
		if ep.Honeypot != "" && strings.TrimSpace(r.FormValue(ep.Honeypot)) != "" {
			formDone(w, r, ep)
			return
		}
		payload := collectFields(r, ep)
		body, _ := json.Marshal(payload)
		// #nosec G704 -- ep.To is author config (not request-controlled), validated
		// http(s) at build; delivery uses the SSRF-hardened SecureTransport, which
		// resolves and refuses private/loopback ranges at dial time.
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, ep.To, bytes.NewReader(body))
		if err != nil {
			http.Error(w, "delivery failed", http.StatusBadGateway)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		// #nosec G704 -- see above: trusted config target over the SSRF-hardened client.
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "delivery failed", http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 400 {
			http.Error(w, "delivery rejected", http.StatusBadGateway)
			return
		}
		formDone(w, r, ep)
	}), nil
}

// collectFields gathers the submission into a map: the declared fields, or every
// submitted field when none are declared. The honeypot is never forwarded.
func collectFields(r *http.Request, ep config.Endpoint) map[string]string {
	payload := map[string]string{}
	if len(ep.Fields) > 0 {
		for _, f := range ep.Fields {
			if f != ep.Honeypot {
				payload[f] = r.FormValue(f)
			}
		}
		return payload
	}
	for k := range r.PostForm {
		if k != ep.Honeypot {
			payload[k] = r.FormValue(k)
		}
	}
	return payload
}

// formDone ends a successful submission: a 303 to the configured page, or a small
// JSON ok when none is set.
func formDone(w http.ResponseWriter, r *http.Request, ep config.Endpoint) {
	if ep.Redirect != "" {
		http.Redirect(w, r, ep.Redirect, http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
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
