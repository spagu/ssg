package externalsource

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPConnector loads remote sources through the hardened client and the
// shared disk cache: fresh cache hits skip the network, --refresh forces a
// fetch, stale-if-error falls back to an expired-but-usable payload, and
// offline mode never touches the network at all.
type HTTPConnector struct {
	cache        diskCache
	allowedHosts []string
	offline      bool
	refresh      bool
	refreshOnly  string
	staleIfError bool
	failOnMiss   bool
}

// Load resolves one HTTP source. The returned Metadata reports FromCache and
// Stale so templates (and the build log) can tell how fresh the data is.
func (h HTTPConnector) Load(src Source) (*Result, error) {
	key := cacheKey(src)
	if h.offline {
		return h.loadOffline(src, key)
	}
	refresh := h.refresh && (h.refreshOnly == "" || h.refreshOnly == src.Name)
	if !refresh {
		if res, ok := h.cachedResult(src, key, false); ok {
			return res, nil
		}
	}
	body, contentType, fetchErr := h.fetch(src)
	if fetchErr != nil {
		if res, ok := h.cachedResult(src, key, true); ok {
			return res, nil
		}
		return nil, fail(src, "fetch", fetchErr)
	}
	now := time.Now()
	meta := cacheMeta{Source: src.Name, Type: src.Type, FetchedAt: now,
		ExpiresAt: now.Add(src.CacheTTL), StaleUntil: now.Add(src.CacheTTL + src.StaleTTL),
		ContentType: contentType}
	if err := h.cache.put(key, body, meta); err != nil {
		fmt.Printf("   ⚠️  Warning: external source %q: cache write failed: %v\n", src.Name, err)
	}
	return h.buildResult(src, body, meta, false, false)
}

// cachedResult serves from the cache when the entry is usable: fresh entries
// always qualify; asStale additionally accepts expired-but-within-stale-TTL
// entries (the stale-if-error path).
func (h HTTPConnector) cachedResult(src Source, key string, asStale bool) (*Result, bool) {
	if asStale && !h.staleIfError {
		return nil, false
	}
	body, meta, ok := h.cache.get(key)
	if !ok {
		return nil, false
	}
	deadline, stale := meta.ExpiresAt, false
	if asStale {
		deadline, stale = meta.StaleUntil, true
	}
	if !time.Now().Before(deadline) {
		return nil, false
	}
	res, err := h.buildResult(src, body, meta, true, stale)
	if err != nil {
		return nil, false
	}
	return res, true
}

// errCacheMissSkip marks an offline cache miss that must only warn, not fail
// the build (fail_on_cache_miss: false), even for required sources.
var errCacheMissSkip = errors.New("offline mode and no cached copy (skipped: fail_on_cache_miss is false)")

// loadOffline serves exclusively from the cache (plan §Cache: offline mode).
func (h HTTPConnector) loadOffline(src Source, key string) (*Result, error) {
	body, meta, ok := h.cache.get(key)
	if !ok {
		if h.failOnMiss {
			return nil, fail(src, "cache", fmt.Errorf("offline mode and no cached copy (run once online or disable fail_on_cache_miss)"))
		}
		return nil, fail(src, "cache", errCacheMissSkip)
	}
	stale := !time.Now().Before(meta.ExpiresAt)
	return h.buildResult(src, body, meta, true, stale)
}

// buildResult parses, transforms and packages a payload with its metadata.
func (h HTTPConnector) buildResult(src Source, body []byte, meta cacheMeta, fromCache, stale bool) (*Result, error) {
	data, err := Parse(src.Format, bytes.NewReader(body), src.CSV)
	if err != nil {
		if fromCache {
			h.cache.evict(cacheKey(src))
		}
		return nil, fail(src, "parse", err)
	}
	data, err = applyTransform(data, src.Transform)
	if err != nil {
		return nil, fail(src, "transform", err)
	}
	u, err := validateURL(src.URL, src, h.allowedHosts)
	if err != nil {
		return nil, fail(src, "config", err)
	}
	return &Result{Name: src.Name, Type: src.Type, Data: data, Metadata: Metadata{
		SourceType: src.Type, Identifier: safeIdentifier(u), FetchedAt: meta.FetchedAt,
		FromCache: fromCache, Stale: stale, Checksum: sha256Hex(body),
		RecordCount: recordCount(data), ContentType: src.Format,
	}}, nil
}

// fetch performs the network request with retries and exponential backoff.
// Retries cover network errors, 429 and 5xx; other statuses fail immediately.
func (h HTTPConnector) fetch(src Source) (body []byte, contentType string, err error) {
	u, err := validateURL(src.URL, src, h.allowedHosts)
	if err != nil {
		return nil, "", err
	}
	client := newHTTPClient(src, h.allowedHosts)
	var lastErr error
	for attempt := 0; attempt <= src.Retries; attempt++ {
		if attempt > 0 {
			time.Sleep(src.RetryBackoff * time.Duration(attempt))
		}
		body, contentType, lastErr = h.doRequest(client, src, u.String())
		if lastErr == nil {
			return body, contentType, nil
		}
		var retriable *retriableError
		if !errors.As(lastErr, &retriable) {
			return nil, "", lastErr
		}
	}
	return nil, "", fmt.Errorf("%w (after %d attempts)", lastErr, src.Retries+1)
}

// retriableError marks failures worth retrying.
type retriableError struct{ err error }

func (e *retriableError) Error() string { return e.err.Error() }
func (e *retriableError) Unwrap() error { return e.err }

// doRequest performs one attempt: build the request, send it, enforce the
// status, content-type and size rules.
func (h HTTPConnector) doRequest(client *http.Client, src Source, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	if len(src.Query) > 0 {
		q := req.URL.Query()
		for k, v := range src.Query {
			q.Set(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}
	for k, v := range src.Headers {
		req.Header.Set(k, v)
	}
	applyAuth(req, src.Auth)
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "ssg-external-sources")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", &retriableError{fmt.Errorf("request to %s failed: %w", safeIdentifier(req.URL), err)}
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return nil, "", &retriableError{fmt.Errorf("%s returned status %d", safeIdentifier(req.URL), resp.StatusCode)}
	default:
		return nil, "", fmt.Errorf("%s returned status %d", safeIdentifier(req.URL), resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !contentTypeAccepted(src.Format, contentType) {
		return nil, "", fmt.Errorf("%s returned content-type %q, which does not match format %q", safeIdentifier(req.URL), contentType, src.Format)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, src.MaxSize+1))
	if err != nil {
		return nil, "", &retriableError{fmt.Errorf("reading %s: %w", safeIdentifier(req.URL), err)}
	}
	if int64(len(body)) > src.MaxSize {
		return nil, "", fmt.Errorf("%s response exceeds the %d-byte limit (defaults.max_response_size)", safeIdentifier(req.URL), src.MaxSize)
	}
	return body, contentType, nil
}

// applyAuth attaches the configured credentials to a request.
func applyAuth(req *http.Request, a AuthConfig) {
	switch a.Type {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+a.Token)
	case "basic":
		req.SetBasicAuth(a.Username, a.Password)
	case "header":
		req.Header.Set(a.Header, a.Value)
	}
}
