package main

import (
	"compress/gzip"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/spagu/ssg/internal/config"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/netutil"
)

// startServer serves the output directory over HTTP(S) with automatic
// Cache-Control and security headers, optional gzip, connection and soft-memory
// limits, and optional TLS (manual cert/key or automatic Let's Encrypt) (v1.8.1).
func startServer(cfg *config.Config) {
	applyMemLimit(cfg.MemLimit, cfg.Quiet)

	addr, url, exposed := resolveListenAddr(cfg.Host, cfg.Port)
	mode := serverTLSMode(cfg)

	// autocert manager is shared between the TCP (HTTP/1.1+2) and QUIC (HTTP/3) servers.
	var acm *autocert.Manager
	if mode == "auto" {
		acm = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(cfg.TLSDomain),
			Cache:      autocert.DirCache(autocertCacheDir()),
		}
	}

	handler := buildServerHandler(cfg, mode != "")

	// HTTP/3 (QUIC) requires TLS; when enabled it runs alongside HTTPS and TCP
	// responses advertise it via Alt-Svc so browsers upgrade (v1.8.1).
	if cfg.HTTP3 && mode != "" {
		h3 := &http3.Server{Addr: addr, Handler: handler}
		if acm != nil {
			h3.TLSConfig = acm.TLSConfig()
		}
		handler = altSvcMiddleware(handler, h3)
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

	if err := listenAndServe(server, cfg, mode, acm); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "❌ Server error: %v\n", err)
	}
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
			fmt.Fprintf(os.Stderr, "⚠️  HTTP/3 server error: %v\n", err)
		}
	}()
}

// altSvcMiddleware advertises the HTTP/3 endpoint on TCP responses via Alt-Svc.
func altSvcMiddleware(next http.Handler, h3 *http3.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = h3.SetQUICHeaders(w.Header())
		next.ServeHTTP(w, r)
	})
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

// listenAndServe binds the listener (with optional connection cap) and serves in
// the selected TLS mode. For autocert it uses the shared manager acm.
func listenAndServe(server *http.Server, cfg *config.Config, mode string, acm *autocert.Manager) error {
	if mode == "auto" {
		server.TLSConfig = acm.TLSConfig()
		go func() { _ = http.ListenAndServe(":80", acm.HTTPHandler(nil)) }() // #nosec G114 -- ACME HTTP-01 + redirect only
		return server.ListenAndServeTLS("", "")
	}
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	if cfg.MaxConns > 0 {
		ln = netutil.LimitListener(ln, cfg.MaxConns)
	}
	if mode == "manual" {
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		return server.ServeTLS(ln, cfg.TLSCert, cfg.TLSKey)
	}
	return server.Serve(ln)
}

// autocertCacheDir returns a per-user cache directory for Let's Encrypt certs.
func autocertCacheDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".ssg", "autocert")
	}
	return filepath.Join(os.TempDir(), "ssg-autocert")
}

// buildServerHandler wraps the file server with cache-control, security-header and
// (optional) gzip middleware.
func buildServerHandler(cfg *config.Config, tlsOn bool) http.Handler {
	h := http.Handler(http.FileServer(http.Dir(cfg.OutputDir)))
	h = cacheControlMiddleware(h)
	h = securityHeadersMiddleware(h, tlsOn)
	if cfg.Gzip {
		h = gzipMiddleware(h)
	}
	return h
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
	gz *gzip.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) { return g.gz.Write(b) }

// gzipMiddleware compresses responses when the client accepts gzip.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
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
