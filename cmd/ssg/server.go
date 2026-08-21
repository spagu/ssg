package main

import (
	"compress/gzip"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/serverauth"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/netutil"
)

// startServer serves the output directory over HTTP(S) with automatic
// Cache-Control and security headers, optional gzip, connection and soft-memory
// limits, and optional TLS (manual cert/key or automatic Let's Encrypt) (v1.8.1).
func startServer(cfg *config.Config) {
	ln, err := claimPort(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		return
	}

	serveOnClaimedPort(cfg, ln)
}

// startServerAsync claims the port in the foreground and serves in the
// background, so a caller that announces the address (migrate, mcp) prints the
// port the server actually took rather than the one it hoped for (#135).
func startServerAsync(cfg *config.Config) {
	ln, err := claimPort(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		return
	}

	go serveOnClaimedPort(cfg, ln)
}

// serveOnClaimedPort runs the server on an already-bound listener.
func serveOnClaimedPort(cfg *config.Config, ln net.Listener) {
	applyMemLimit(cfg.MemLimit, cfg.Quiet)

	addr, url, exposed := resolveListenAddr(cfg.Host, cfg.Port)
	mode := serverTLSMode(cfg)

	// autocert manager is shared between the TCP (HTTP/1.1+2) and QUIC (HTTP/3) servers.
	var acm *autocert.Manager
	if mode == "auto" {
		acm = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: tlsHostPolicy(cfg.TLSDomain),
			Cache:      autocert.DirCache(autocertCacheDir()),
		}
	}

	handler := buildServerHandler(cfg, mode != "")
	warnTLSMisconfig(cfg, mode)

	// HTTP/3 (QUIC) requires TLS; when enabled it runs alongside HTTPS and TCP
	// responses advertise it via Alt-Svc so browsers upgrade (v1.8.1).
	if cfg.HTTP3 && mode != "" {
		h3 := &http3.Server{Addr: addr, Handler: handler}
		if acm != nil {
			h3.TLSConfig = acm.TLSConfig()
		}
		handler = altSvcMiddleware(handler, altSvcValue(addr))
		startHTTP3(h3, cfg, mode)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	logServerStart(cfg, url, mode, exposed)

	if err := serveOnListener(server, ln, cfg, mode, acm); err != nil && err != http.ErrServerClosed {
		reportServerErr("❌ Server error: %v\n", err)
	}
}

// serverErrf reports a server failure. A variable rather than a direct
// fmt.Fprintf so the paths that only ever run once the server is up — the
// HTTP/3 listener reporting from its own goroutine, a listener that dies under
// the server — can be exercised instead of shipped untested, the same reason
// githubAPIBase is one. Swapping os.Stderr from a test cannot do it here:
// these report from goroutines the test does not own, and the swap races them.
//
// Read under a lock because that is the whole point: the readers are background
// goroutines, so a seam that can only be set safely before any of them start is
// not a seam at all.
var (
	serverErrMu sync.RWMutex
	serverErrf  = func(format string, a ...any) { fmt.Fprintf(os.Stderr, format, a...) }
)

// reportServerErr sends one failure to the current reporter.
func reportServerErr(format string, a ...any) {
	serverErrMu.RLock()
	report := serverErrf
	serverErrMu.RUnlock()
	report(format, a...)
}

// setServerErrf swaps the reporter and returns the previous one.
func setServerErrf(report func(string, ...any)) func(string, ...any) {
	serverErrMu.Lock()
	defer serverErrMu.Unlock()
	previous := serverErrf
	serverErrf = report
	return previous
}

// startHTTP3 launches the QUIC/HTTP-3 listener in the background (v1.8.1).
func startHTTP3(h3 *http3.Server, cfg *config.Config, mode string) {
	go func() {
		var err error
		if mode == "auto" {
			err = h3.ListenAndServe() // uses TLSConfig from the autocert manager
		} else {
			err = h3.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
		}
		if err != nil {
			reportServerErr("⚠️  HTTP/3 server error: %v\n", err)
		}
	}()
}

// altSvcValue builds the Alt-Svc advertisement for the HTTP/3 endpoint from the
// configured listen address. The value is computed locally instead of via
// http3.Server.SetQUICHeaders, which emits nothing until a QUIC listener is
// registered — leaving early TCP responses without Alt-Svc (GO-033).
func altSvcValue(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return ""
	}
	return fmt.Sprintf(`h3=":%s"; ma=2592000`, port)
}

// altSvcMiddleware advertises the HTTP/3 endpoint on every TCP response so
// browsers can upgrade from the very first request (GO-033).
func altSvcMiddleware(next http.Handler, altSvc string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if altSvc != "" {
			w.Header().Set("Alt-Svc", altSvc)
		}
		next.ServeHTTP(w, r)
	})
}

// tlsHostPolicy builds the autocert whitelist from the --tls-domain value, which
// may be a comma-separated list of domains (GO-020).
func tlsHostPolicy(domains string) autocert.HostPolicy {
	return autocert.HostWhitelist(splitCSV(domains)...)
}

// serverTLSMode reports the TLS mode: "manual" (cert+key), "auto" (Let's Encrypt),
// or "" (plain HTTP). Manual takes priority.
func serverTLSMode(cfg *config.Config) string {
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		return "manual"
	}
	if cfg.TLSAuto && cfg.TLSDomain != "" {
		return "auto"
	}
	return ""
}

// warnTLSMisconfig makes every silent TLS/HTTP3 degradation loud (GO-056):
// incomplete flag pairs used to fall back to plain HTTP with no diagnostic,
// so users believed they were serving HTTPS or HTTP/3 when they were not.
func warnTLSMisconfig(cfg *config.Config, mode string) {
	if mode == "" {
		switch {
		case cfg.TLSAuto && cfg.TLSDomain == "":
			fmt.Fprintln(os.Stderr, "⚠️  --tls-auto needs --tls-domain=<domain>; serving plain HTTP")
		case cfg.TLSCert != "" && cfg.TLSKey == "":
			fmt.Fprintln(os.Stderr, "⚠️  --tls-cert given without --tls-key; serving plain HTTP")
		case cfg.TLSKey != "" && cfg.TLSCert == "":
			fmt.Fprintln(os.Stderr, "⚠️  --tls-key given without --tls-cert; serving plain HTTP")
		}
	}
	if cfg.HTTP3 && mode == "" {
		fmt.Fprintln(os.Stderr, "⚠️  --http3 requires TLS (--tls-auto or --tls-cert/--tls-key); HTTP/3 disabled")
	}
}

// logServerStart prints a one-line startup banner unless quiet.
func logServerStart(cfg *config.Config, url, mode string, exposed bool) {
	if cfg.Quiet {
		return
	}
	scheme := "HTTP"
	if mode != "" {
		scheme = "HTTPS"
		url = strings.Replace(url, "http://", "https://", 1)
	}
	fmt.Printf("🌐 Starting %s server at %s\n", scheme, url)
	if mode == "auto" {
		fmt.Printf("   🔐 Let's Encrypt for %s (needs public :80/:443)\n", cfg.TLSDomain)
	}
	if exposed {
		fmt.Printf("   ⚠️  Exposed on ALL network interfaces\n")
	}
	fmt.Printf("   Serving %s/ (gzip:%v, max-conns:%d)\n", cfg.OutputDir, cfg.Gzip, cfg.MaxConns)
}

// newServerListener binds addr and applies the --max-conns cap. Shared by every
// TLS mode so the cap also holds for autocert (GO-019).
func newServerListener(addr string, maxConns int) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if maxConns > 0 {
		ln = netutil.LimitListener(ln, maxConns)
	}
	return ln, nil
}

// serveOnListener serves on ln in the selected TLS mode. The autocert path wraps
// the (possibly connection-capped) listener in a TLS listener instead of calling
// ListenAndServeTLS, so --max-conns is honoured there too (GO-019).
func serveOnListener(server *http.Server, ln net.Listener, cfg *config.Config, mode string, acm *autocert.Manager) error {
	switch mode {
	case "auto":
		server.TLSConfig = acm.TLSConfig()
		// ACME HTTP-01 challenge helper; a failed :80 bind (e.g. missing
		// privileges) must be visible, not silently swallowed (GO-034).
		go func() {
			if err := http.ListenAndServe(":80", acm.HTTPHandler(nil)); err != nil { // #nosec G114 -- ACME HTTP-01 + redirect only
				fmt.Fprintf(os.Stderr, "⚠️  autocert HTTP-01 helper (:80): %v\n", err)
			}
		}()
		return server.Serve(tls.NewListener(ln, server.TLSConfig))
	case "manual":
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		return server.ServeTLS(ln, cfg.TLSCert, cfg.TLSKey)
	default:
		return server.Serve(ln)
	}
}

// autocertCacheDir returns a per-user, owner-private cache directory for the
// Let's Encrypt account key and certificates. It deliberately avoids the shared,
// world-predictable system temp directory (S5445): the cache holds TLS private
// keys, so it must live under a per-user path. autocert.DirCache creates the
// directory with 0700 and stores files 0600.
func autocertCacheDir() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "ssg", "autocert")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".ssg", "autocert")
	}
	// Last resort: a private path under the working directory — never the shared
	// system temp dir, which would expose the private keys to other local users.
	return filepath.Join(".ssg", "autocert")
}

// buildServerHandler wraps the file server with cache-control, security-header,
// (optional) gzip and (optional) access-control middleware. Access control is
// outermost so refused requests never reach the file server.
func buildServerHandler(cfg *config.Config, tlsOn bool) http.Handler {
	static := http.Handler(http.FileServer(noDirListing{http.Dir(cfg.OutputDir)}))
	// The `_redirects` and `_headers` this build wrote are served too, so a
	// redirect can be checked before it is published (#181). Order mirrors
	// Cloudflare Pages: redirect rules are evaluated before static assets, and
	// the header blocks decorate whatever is served after them. Both tables are
	// re-read after every rebuild, not only on a config reload.
	republishOutputRules(cfg)
	files := liveRedirectHandler(liveHeadersHandler(static))
	// Vendor-neutral endpoints intercept their paths before the file server;
	// everything else is still served statically (#63). The routing table is
	// published rather than captured, so a watch reload can replace it without
	// touching the listener (#180).
	publishEndpoints(cfg, files)
	h := liveEndpointHandler(files)
	h = cacheControlMiddleware(h)
	// A preview must never cache: the policy above and the `_headers` rules
	// below it are both written for a published site, and either one will serve
	// the previous build's stylesheet after a reload (#185). Applied inside the
	// live-reload wrapper so the SSE stream keeps its http.Flusher.
	if previewMode(cfg) {
		h = previewNoCacheMiddleware(h)
	}
	h = securityHeadersMiddleware(h, tlsOn)
	if cfg.Gzip {
		h = gzipMiddleware(h)
	}
	authCfg := serverauth.Config{Auth: cfg.ServerAuth, Users: cfg.ServerUsers, JWTSecret: cfg.JWTSecret,
		IPAllowlist: cfg.IPAllowlist, IPBlocklist: cfg.IPBlocklist,
		RateLimit: cfg.RateLimit, RateBurst: cfg.RateBurst}
	if authCfg.Enabled() {
		wrapped, err := serverauth.Middleware(h, authCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Server access-control config: %v\n", err)
			os.Exit(1)
		}
		h = wrapped
		if !cfg.Quiet {
			fmt.Printf("   🔒 Access control: auth=%s allowlist=%d blocklist=%d rate=%g/s\n",
				orOpen(cfg.ServerAuth), len(cfg.IPAllowlist), len(cfg.IPBlocklist), cfg.RateLimit)
		}
	}
	// Outermost: the live-reload endpoint bypasses gzip/auth (a dev-only stream),
	// and HTML injection sees the finished page. A no-op unless --auto-reload
	// created the hub (GO-090).
	h = liveReloadMiddleware(h)
	return h
}

// orOpen labels an empty auth mode for the startup banner.
func orOpen(mode string) string {
	if mode == "" {
		return "open"
	}
	return mode
}

// cacheControlHeader is the HTTP header name set by the cache middleware.
const cacheControlHeader = "Cache-Control"

// fingerprintedAsset matches ASSET-001 hashed asset names (name.<hash8>.ext).
var fingerprintedAsset = regexp.MustCompile(`\.[0-9a-f]{8}\.(css|js)$`)

// cacheControlMiddleware sets Cache-Control by resource type: immutable long cache
// for fingerprinted assets, a medium cache for other static assets, and no-cache
// for HTML so content updates are seen immediately.
func cacheControlMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case fingerprintedAsset.MatchString(p):
			w.Header().Set(cacheControlHeader, "public, max-age=31536000, immutable")
		case p == "" || strings.HasSuffix(p, "/") || strings.HasSuffix(p, ".html"):
			w.Header().Set(cacheControlHeader, "no-cache")
		default:
			w.Header().Set(cacheControlHeader, "public, max-age=3600")
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware adds baseline security headers (and HSTS under TLS).
func securityHeadersMiddleware(next http.Handler, tlsOn bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if tlsOn {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// gzipResponseWriter compresses the response body.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
}

// WriteHeader strips headers that would be wrong for a compressed body: the
// declared length and range support refer to the uncompressed bytes, so leaving
// them in desynchronises the connection (GO-012).
func (g *gzipResponseWriter) WriteHeader(code int) {
	if g.wroteHeader {
		return
	}
	g.wroteHeader = true
	g.Header().Del("Content-Length")
	g.Header().Del("Accept-Ranges")
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.wroteHeader {
		g.WriteHeader(http.StatusOK)
	}
	return g.gz.Write(b)
}

// gzipMiddleware compresses responses when the client accepts gzip. Range
// requests bypass compression entirely: http.ServeContent computes the 206
// Content-Length from the uncompressed slice, so gzipping the body would break
// resumed downloads and media seeking (GO-012).
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		gz := gzip.NewWriter(w)
		defer func() { _ = gz.Close() }()
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
	})
}

// applyMemLimit sets a soft runtime memory limit from a human size (e.g. "512MiB").
func applyMemLimit(s string, quiet bool) {
	if s == "" {
		return
	}
	bytes, err := parseByteSize(s)
	if err != nil || bytes <= 0 {
		if !quiet {
			fmt.Fprintf(os.Stderr, "⚠️  invalid --mem-limit %q: %v\n", s, err)
		}
		return
	}
	debug.SetMemoryLimit(bytes)
	if !quiet {
		fmt.Printf("   🧠 Soft memory limit: %s\n", s)
	}
}

// parseByteSize parses sizes like "512MiB", "1GiB", "256MB", "1048576".
func parseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	units := []struct {
		suffix string
		mult   int64
	}{
		{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10},
		{"GB", 1e9}, {"MB", 1e6}, {"KB", 1e3}, {"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), 64)
			if err != nil {
				return 0, err
			}
			return int64(n * float64(u.mult)), nil
		}
	}
	return strconv.ParseInt(s, 10, 64)
}

// noDirListing serves files but refuses to invent a directory index (#146).
//
// A directory with no index.html used to answer 200 with a <pre> list of file
// names — posts with dated permalinks live in /2014/05/, so a missing date
// archive looked like a working page serving 776 bytes of filenames instead of
// the archive the source served. Every host this project deploys to (Cloudflare
// Pages, Netlify, GitHub Pages) answers 404 there, so the dev server was also
// the one place where a hole in the output did not look like a hole.
type noDirListing struct{ fs http.FileSystem }

func (n noDirListing) Open(name string) (http.File, error) {
	f, err := n.fs.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if info.IsDir() {
		// http.FileServer asks for the directory's index.html itself; reaching
		// here with a directory means there is none, and the listing would be
		// the only thing to show.
		if _, indexErr := n.fs.Open(path.Join(name, "index.html")); indexErr != nil {
			_ = f.Close()
			return nil, os.ErrNotExist
		}
	}
	return f, nil
}
