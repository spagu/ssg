package externalsource

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTP hardening (plan §Bezpieczeństwo HTTP): HTTPS by default, optional host
// allowlist, localhost/private-IP blocking enforced AT DIAL TIME (which also
// defeats DNS rebinding — the IP actually connected to is the IP checked),
// bounded redirects with re-validation, response size limits and content-type
// validation. Errors carry the URL without its query string, so query-borne
// secrets never reach logs.

// maxRedirects bounds redirect chains.
const maxRedirects = 5

// safeIdentifier is the loggable form of a URL: scheme://host/path, no query.
func safeIdentifier(u *url.URL) string {
	c := *u
	c.RawQuery = ""
	c.User = nil
	return c.String()
}

// validateURL enforces scheme and allowlist rules for one request URL.
func validateURL(raw string, src Source, allowedHosts []string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !src.AllowHTTP {
			return nil, fmt.Errorf("plain http is disabled for %s — use https or set allow_http: true", safeIdentifier(u))
		}
	default:
		return nil, fmt.Errorf("unsupported scheme %q in %s", u.Scheme, safeIdentifier(u))
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("missing host in %s", safeIdentifier(u))
	}
	if !hostAllowed(u.Hostname(), allowedHosts) {
		return nil, fmt.Errorf("host %q is not in external_sources.allowed_hosts", u.Hostname())
	}
	return u, nil
}

// hostAllowed matches a hostname against the allowlist ("api.example.com" or
// "*.example.com"). An empty allowlist allows every (public) host.
func hostAllowed(host string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	host = strings.ToLower(host)
	for _, a := range allowed {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == host {
			return true
		}
		if suffix, ok := strings.CutPrefix(a, "*."); ok && strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// blockedIP reports whether an IP must not be dialed (SSRF protection).
func blockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()
}

// secureDialContext resolves the host itself and dials a vetted IP literal, so
// a DNS answer that changes between validation and connection (DNS rebinding)
// still cannot reach a private address.
func secureDialContext(allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range ips {
			if !allowPrivate && blockedIP(ip.IP) {
				lastErr = fmt.Errorf("host %q resolves to blocked address %s (localhost/private ranges are refused; set allow_private: true for self-hosted APIs)", host, ip.IP)
				continue
			}
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("no addresses found for %q", host)
		}
		return nil, lastErr
	}
}

// newHTTPClient builds the hardened client for one source.
func newHTTPClient(src Source, allowedHosts []string) *http.Client {
	transport := &http.Transport{
		DialContext:           secureDialContext(src.AllowPrivate),
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: src.Timeout,
		DisableKeepAlives:     true,
	}
	return &http.Client{
		Timeout:   src.Timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			// Every redirect target passes the same scheme/allowlist rules.
			_, err := validateURL(req.URL.String(), src, allowedHosts)
			return err
		},
	}
}

// contentTypeAccepted validates a response Content-Type against the parser
// format. An absent header is accepted; a clearly conflicting one is not.
func contentTypeAccepted(format, contentType string) bool {
	if contentType == "" {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	// text/plain and octet-stream are accepted for every format: servers
	// without an explicit header get text/plain from content sniffing (Go's
	// httptest included), and raw files behind CDNs commonly ship as plain
	// text or octet-stream. Clear conflicts (text/html, cross-format types)
	// still fail.
	if ct == "text/plain" || ct == "application/octet-stream" {
		return true
	}
	accepted := map[string][]string{
		"json": {"application/json", "text/json", "+json"},
		"csv":  {"text/csv", "application/csv"},
		"xml":  {"application/xml", "text/xml", "+xml"},
		"yaml": {"application/yaml", "text/yaml", "application/x-yaml"},
		"toml": {"application/toml"},
	}
	for _, want := range accepted[format] {
		if strings.HasSuffix(ct, want) || ct == want {
			return true
		}
	}
	return false
}
