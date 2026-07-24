// Package fetch retrieves configuration from a path or URL with optional
// authentication, over a bounded, size-capped HTTP client. It is the shared
// machinery behind YAML includes (GO-076): .ssg.yaml can `include:` other
// config files from a path or a URL, so a project's config splits across files.
// The same Auth model is reused by remote worker sources in the worker phase.
//
// Secrets never live in the config file: auth token/password/header values must
// reference an environment variable ("$GITHUB_TOKEN"), resolved by ExpandAuth.
package fetch

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	timeout      = 30 * time.Second
	maxRedirects = 5
	// DefaultMaxBytes caps a single config include; generous for YAML, small
	// enough that a runaway URL cannot exhaust memory.
	DefaultMaxBytes int64 = 5 * 1024 * 1024
)

// Auth authenticates a fetch. Type is "" (none), "bearer", "basic" or "header".
// Secret fields hold already-resolved values (see ExpandAuth); Username and
// Header are plain.
type Auth struct {
	Type     string
	Token    string // bearer
	Username string // basic
	Password string // basic
	Header   string // header: the header name, e.g. "X-Api-Key"
	Value    string // header: the header value
}

// apply sets the request's auth header. Unknown/empty types add nothing.
func (a Auth) apply(req *http.Request) error {
	switch a.Type {
	case "", "none":
		return nil
	case "bearer":
		if a.Token == "" {
			return fmt.Errorf("auth type \"bearer\" needs a token")
		}
		req.Header.Set("Authorization", "Bearer "+a.Token)
	case "basic":
		if a.Username == "" {
			return fmt.Errorf("auth type \"basic\" needs a username")
		}
		req.SetBasicAuth(a.Username, a.Password)
	case "header":
		if a.Header == "" {
			return fmt.Errorf("auth type \"header\" needs a header name")
		}
		req.Header.Set(a.Header, a.Value)
	default:
		return fmt.Errorf("unsupported auth type %q (use bearer, basic or header)", a.Type)
	}
	return nil
}

// ExpandAuth resolves "$NAME"/"${NAME}" env references in the secret fields
// (token, password, header value) and rejects a literal there, so a credential
// never sits in the config file. Username and header name pass through. A
// referenced-but-unset variable is an error naming the variable, never a value.
func ExpandAuth(a Auth) (Auth, error) {
	var err error
	if a.Token, err = expandSecret("auth.token", a.Token); err != nil {
		return Auth{}, err
	}
	if a.Password, err = expandSecret("auth.password", a.Password); err != nil {
		return Auth{}, err
	}
	if a.Value, err = expandSecret("auth.value", a.Value); err != nil {
		return Auth{}, err
	}
	return a, nil
}

// expandSecret enforces the env-reference form for one secret field.
func expandSecret(field, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, "$") {
		return "", fmt.Errorf("%s must reference an environment variable (e.g. \"$TOKEN\"), not a literal secret", field)
	}
	name := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(value, "$"), "{"), "}")
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return "", fmt.Errorf("%s references $%s, which is not set in the environment", field, name)
	}
	return v, nil
}

// client is the shared bounded HTTP client: a timeout, a redirect cap, and — the
// security-relevant part — it strips the auth credential on any redirect that
// leaves the original origin. Go forwards a custom request header (the "header"
// auth type, e.g. X-Api-Key) to redirect targets unconditionally, and only drops
// Authorization across a *different domain*, so without this a configured server
// could 302 a private-source credential to an attacker host (or downgrade to
// http and leak a bearer/basic token in cleartext).
func client(auth Auth) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			orig := via[0].URL
			leftOrigin := req.URL.Host != orig.Host ||
				(orig.Scheme == "https" && req.URL.Scheme != "https")
			if leftOrigin {
				req.Header.Del("Authorization")
				req.Header.Del("Cookie")
				if auth.Type == "header" && auth.Header != "" {
					req.Header.Del(auth.Header)
				}
			}
			return nil
		},
	}
}

// Bytes fetches url with auth and returns the body, refusing a response larger
// than maxBytes (0 uses DefaultMaxBytes). Used for config includes.
func Bytes(url string, auth Auth, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid url %q: %w", url, err)
	}
	if err := auth.apply(req); err != nil {
		return nil, err
	}
	resp, err := client(auth).Do(req) // #nosec G107 -- url comes from the user's own config include
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", safeURL(url), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: HTTP %d", safeURL(url), resp.StatusCode)
	}
	// +1 so a body exactly at the cap is still detected as over.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", safeURL(url), err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes; refusing", safeURL(url), maxBytes)
	}
	return body, nil
}

// safeURL strips the query string AND any userinfo (https://<token>@host/…, a
// form some Git hosts accept) so a credential passed in a URL never lands in an
// error message.
func safeURL(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		u.RawQuery = ""
		u.User = nil
		return u.String()
	}
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i]
	}
	return raw
}

// IsURL reports whether s is an http(s) URL rather than a local path.
func IsURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
