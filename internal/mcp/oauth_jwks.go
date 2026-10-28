package mcp

// The issuer's signing keys, discovered and cached (#362).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

const (
	// jwksMaxAge is how long a key set is trusted before it is fetched again.
	jwksMaxAge = time.Hour
	// jwksMinRefresh bounds refetching on an unknown key ID, so a stream of
	// tokens with made-up IDs cannot turn this server into a load generator
	// against the issuer.
	jwksMinRefresh = 30 * time.Second
	// maxMetadataBytes bounds a metadata document or key set.
	maxMetadataBytes = 1 << 20
)

// jwksCache holds the issuer's key set.
type jwksCache struct {
	mu      sync.Mutex
	opts    OAuthOptions
	client  *http.Client
	set     jose.JSONWebKeySet
	fetched time.Time
	now     func() time.Time
}

func newJWKSCache(o OAuthOptions) *jwksCache {
	client := o.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &jwksCache{opts: o, client: client, now: time.Now}
}

// key returns the signing key a token names, refreshing the set when it is
// old or does not have that key.
func (c *jwksCache) key(kid string) (*jose.JSONWebKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	age := c.now().Sub(c.fetched)
	k := signingKey(c.set, kid)
	if c.fetched.IsZero() || age > jwksMaxAge || (k == nil && age > jwksMinRefresh) {
		if err := c.refresh(); err != nil {
			return nil, err
		}
		k = signingKey(c.set, kid)
	}
	if k == nil {
		return nil, fmt.Errorf("no signing key %q at the issuer", kid)
	}
	return k, nil
}

// signingKey finds kid among the keys meant for signatures. A token without a
// kid is accepted only when there is exactly one such key to mean.
func signingKey(set jose.JSONWebKeySet, kid string) *jose.JSONWebKey {
	var candidates []jose.JSONWebKey
	for _, k := range set.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		if kid == "" || k.KeyID == kid {
			candidates = append(candidates, k)
		}
	}
	if len(candidates) != 1 {
		return nil
	}
	return &candidates[0]
}

func (c *jwksCache) refresh() error {
	jwksURL := c.opts.JWKSURL
	if jwksURL == "" {
		discovered, err := c.discover()
		if err != nil {
			return err
		}
		jwksURL = discovered
	}
	var set jose.JSONWebKeySet
	if err := c.getJSON(jwksURL, &set); err != nil {
		return fmt.Errorf("fetching the issuer's keys: %w", err)
	}
	c.set, c.fetched = set, c.now()
	return nil
}

// discover reads jwks_uri from the issuer's metadata: RFC 8414 first, then
// OpenID Connect discovery, which most issuers also publish. The document must
// name the configured issuer, or it describes someone else.
func (c *jwksCache) discover() (string, error) {
	var lastErr error
	for _, u := range issuerMetadataURLs(c.opts.Issuer) {
		var meta struct {
			Issuer  string `json:"issuer"`
			JWKSURI string `json:"jwks_uri"`
		}
		if err := c.getJSON(u, &meta); err != nil {
			// A metadata document naming someone else says more than a 404 at
			// the next address, so it is the one reported.
			if lastErr == nil {
				lastErr = err
			}
			continue
		}
		if meta.Issuer != c.opts.Issuer || meta.JWKSURI == "" {
			lastErr = fmt.Errorf("%s names issuer %q, not %q", u, meta.Issuer, c.opts.Issuer)
			continue
		}
		return meta.JWKSURI, nil
	}
	return "", fmt.Errorf("discovering the issuer's keys: %w", lastErr)
}

// issuerMetadataURLs are where an issuer's metadata may be: RFC 8414 inserts
// the well-known segment before the issuer's path, OpenID Connect appends it.
func issuerMetadataURLs(issuer string) []string {
	u, err := url.Parse(issuer)
	if err != nil {
		return nil
	}
	path := strings.TrimSuffix(u.Path, "/")
	base := u.Scheme + "://" + u.Host
	return []string{
		base + "/.well-known/oauth-authorization-server" + path,
		base + path + "/.well-known/openid-configuration",
	}
}

func (c *jwksCache) getJSON(u string, dest any) error {
	resp, err := c.client.Get(u) // #nosec G107 -- the issuer's own metadata, from the operator's config
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return errors.Join(fmt.Errorf("%s: not JSON", u), err)
	}
	return nil
}
