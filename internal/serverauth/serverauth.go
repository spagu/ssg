// Package serverauth hardens the built-in HTTP server with config-driven
// access control: IP blocklist/allowlist, a per-IP rate limiter, HTTP Basic
// auth and HS256 JWT bearer verification. All of it is opt-in from
// .ssg.yaml; secrets (passwords, the JWT secret) must reference environment
// variables. SSO and LDAP are deliberately out of scope (deferred).
package serverauth

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

// Config is the server-access block of the site configuration.
type Config struct {
	Auth        string   // "" (open) | "basic" | "jwt"
	Users       []string // basic auth: "login:$PASS_ENV" entries
	JWTSecret   string   // jwt: "$JWT_SECRET" (HS256 shared secret)
	IPAllowlist []string // IPs/CIDRs; non-empty = only these may connect
	IPBlocklist []string // IPs/CIDRs refused before anything else
	RateLimit   float64  // requests/second per client IP; 0 = off
	RateBurst   int      // bucket size (default: 2×rate, minimum 1)
}

// Enabled reports whether any access-control feature is configured.
func (c Config) Enabled() bool {
	return c.Auth != "" || len(c.IPAllowlist) > 0 || len(c.IPBlocklist) > 0 || c.RateLimit > 0
}

// Middleware validates the configuration and wraps next with the access
// chain, outermost first: blocklist → allowlist → rate limit → auth.
func Middleware(next http.Handler, cfg Config) (http.Handler, error) {
	h := next
	switch cfg.Auth {
	case "":
	case "basic":
		users, err := parseUsers(cfg.Users)
		if err != nil {
			return nil, err
		}
		h = basicAuthMiddleware(h, users)
	case "jwt":
		secret, err := expandSecret("jwt_secret", cfg.JWTSecret)
		if err != nil {
			return nil, err
		}
		h = jwtMiddleware(h, []byte(secret))
	default:
		return nil, fmt.Errorf("unsupported server_auth %q (supported: basic, jwt; sso/ldap are deferred)", cfg.Auth)
	}
	if cfg.RateLimit > 0 {
		h = rateLimitMiddleware(h, newLimiter(cfg.RateLimit, cfg.RateBurst))
	}
	if len(cfg.IPAllowlist) > 0 {
		nets, err := parseCIDRs("ip_allowlist", cfg.IPAllowlist)
		if err != nil {
			return nil, err
		}
		h = ipFilterMiddleware(h, nets, true)
	}
	if len(cfg.IPBlocklist) > 0 {
		nets, err := parseCIDRs("ip_blocklist", cfg.IPBlocklist)
		if err != nil {
			return nil, err
		}
		h = ipFilterMiddleware(h, nets, false)
	}
	return h, nil
}

// expandSecret resolves a "$NAME" environment reference; literals are
// rejected so secrets never live in the config file.
func expandSecret(field, value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if !strings.HasPrefix(value, "$") {
		return "", fmt.Errorf("%s must reference an environment variable (e.g. \"$JWT_SECRET\"), not a literal", field)
	}
	name := strings.TrimPrefix(value, "$")
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return "", fmt.Errorf("%s references $%s, which is not set in the environment", field, name)
	}
	return v, nil
}

// clientIP extracts the connection's client IP. X-Forwarded-For is
// deliberately NOT trusted: this server terminates its own connections.
func clientIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

// parseCIDRs parses IPs and CIDR ranges into networks (a bare IP is /32 or /128).
func parseCIDRs(field string, entries []string) ([]*net.IPNet, error) {
	out := make([]*net.IPNet, 0, len(entries))
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if !strings.Contains(e, "/") {
			if ip := net.ParseIP(e); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				e = fmt.Sprintf("%s/%d", e, bits)
			}
		}
		_, network, err := net.ParseCIDR(e)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid entry %q (want an IP or CIDR)", field, e)
		}
		out = append(out, network)
	}
	return out, nil
}

// ipFilterMiddleware enforces the allow/block lists. allow=true means the
// request IP must match one of the networks; allow=false refuses matches.
func ipFilterMiddleware(next http.Handler, nets []*net.IPNet, allow bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		matched := ipMatches(ip, nets)
		if (allow && !matched) || (!allow && matched) {
			http.Error(w, "403 forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ipMatches reports whether ip falls inside any of the networks.
func ipMatches(ip net.IP, nets []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
