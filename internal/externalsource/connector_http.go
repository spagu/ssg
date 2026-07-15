package externalsource

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

// fetch performs the network request(s): a single GET, or a paginated series
// when pagination is configured (GO-062).
func (h HTTPConnector) fetch(src Source) (body []byte, contentType string, err error) {
	u, err := validateURL(src.URL, src, h.allowedHosts)
	if err != nil {
		return nil, "", err
	}
	client := newHTTPClient(src, h.allowedHosts)
	if src.Pagination.Mode != "" {
		return h.fetchPaginated(client, src, u.String())
	}
	body, contentType, _, err = h.fetchURL(client, src, u.String(), nil)
	return body, contentType, err
}

// fetchURL performs one page fetch with retries and exponential backoff.
// Retries cover network errors, 429 and 5xx; other statuses fail immediately.
func (h HTTPConnector) fetchURL(client *http.Client, src Source, rawURL string,
	pageQuery map[string]string) (body []byte, contentType, nextLink string, err error) {
	var lastErr error
	for attempt := 0; attempt <= src.Retries; attempt++ {
		if attempt > 0 {
			time.Sleep(src.RetryBackoff * time.Duration(attempt))
		}
		body, contentType, nextLink, lastErr = h.doRequest(client, src, rawURL, pageQuery)
		if lastErr == nil {
			return body, contentType, nextLink, nil
		}
		var retriable *retriableError
		if !errors.As(lastErr, &retriable) {
			return nil, "", "", lastErr
		}
	}
	return nil, "", "", fmt.Errorf("%w (after %d attempts)", lastErr, src.Retries+1)
}

// fetchPaginated fetches successive pages and concatenates their JSON arrays
// into one aggregated payload (GO-062). The aggregate is cached as a single
// entry under the source's cache key (which fingerprints the pagination
// settings, see cacheKey), so cached builds re-parse exactly what a fresh
// multi-page fetch produced. Fetching stops on an empty response, an empty
// (or non-array) JSON page, a missing Link rel="next" header (mode: link) or
// the max_pages guard. Retries, backoff, auth and size limits apply to every
// page request individually.
func (h HTTPConnector) fetchPaginated(client *http.Client, src Source, baseURL string) ([]byte, string, error) {
	st := &pageCursor{url: baseURL, items: make([]interface{}, 0)}
	for page := 0; page < src.Pagination.MaxPages && !st.done; page++ {
		if err := h.fetchPage(client, src, page, st); err != nil {
			return nil, "", err
		}
		if st.object != nil { // non-array first page, served verbatim
			return st.object, st.contentType, nil
		}
	}
	if !st.done {
		fmt.Printf("   ⚠️  Warning: external source %q: pagination stopped at max_pages=%d; more pages may exist\n",
			src.Name, src.Pagination.MaxPages)
	}
	out, err := json.Marshal(st.items)
	if err != nil {
		return nil, "", err
	}
	return out, st.contentType, nil
}

// pageCursor carries the pagination state between page fetches.
type pageCursor struct {
	items       []interface{}
	contentType string
	url         string
	done        bool   // a natural stop was reached before max_pages
	object      []byte // non-array first page, served verbatim
}

// fetchPage fetches one page, appends its array items and advances the cursor.
func (h HTTPConnector) fetchPage(client *http.Client, src Source, page int, st *pageCursor) error {
	body, ct, next, err := h.fetchURL(client, src, st.url, pageParams(src.Pagination, page))
	if err != nil {
		return fmt.Errorf("page %d: %w", page+1, err)
	}
	if st.contentType == "" {
		st.contentType = ct
	}
	if len(bytes.TrimSpace(body)) == 0 {
		st.done = true
		return nil
	}
	arr, err := decodePageArray(src, page, body)
	if err != nil {
		return err
	}
	if arr == nil { // non-array payload: keep the first page, drop later ones
		if page == 0 {
			st.object = body
		}
		st.done = true
		return nil
	}
	if len(arr) == 0 {
		st.done = true
		return nil
	}
	st.items = append(st.items, arr...)
	return h.advanceCursor(src, next, st)
}

// decodePageArray parses one page body. A nil slice with a nil error marks a
// non-array payload (API envelope) that has no defined concatenation.
func decodePageArray(src Source, page int, body []byte) ([]interface{}, error) {
	var parsed interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("page %d: invalid JSON: %w", page+1, err)
	}
	arr, ok := parsed.([]interface{})
	if !ok {
		if page == 0 {
			fmt.Printf("   ⚠️  Warning: external source %q: paginated response is not a JSON array; keeping the first page only\n", src.Name)
		} else {
			fmt.Printf("   ⚠️  Warning: external source %q: page %d is not a JSON array; stopping pagination\n", src.Name, page+1)
		}
		return nil, nil
	}
	return arr, nil
}

// pageParams builds the mode=page query parameters for the nth page (0-based).
func pageParams(p PaginationConfig, page int) map[string]string {
	if p.Mode != "page" {
		return nil
	}
	q := map[string]string{p.Param: strconv.Itoa(p.StartPage + page)}
	if p.PerPage > 0 {
		q[p.PerPageParam] = strconv.Itoa(p.PerPage)
	}
	return q
}

// advanceCursor moves to the next page: mode=link follows the validated Link
// rel="next" target; mode=page advances through pageParams instead.
func (h HTTPConnector) advanceCursor(src Source, next string, st *pageCursor) error {
	if src.Pagination.Mode != "link" {
		return nil
	}
	if next == "" {
		st.done = true
		return nil
	}
	resolved, err := h.resolveNextPageURL(src, st.url, next)
	if err != nil {
		return fmt.Errorf("following Link rel=\"next\": %w", err)
	}
	st.url = resolved
	return nil
}

// resolveNextPageURL resolves a (possibly relative) Link rel="next" target
// against the current page URL and re-validates it, so a hostile header
// cannot steer pagination outside the scheme/allowlist rules.
func (h HTTPConnector) resolveNextPageURL(src Source, current, next string) (string, error) {
	base, err := url.Parse(current)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(next)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}
	resolved := base.ResolveReference(ref)
	if _, err := validateURL(resolved.String(), src, h.allowedHosts); err != nil {
		return "", err
	}
	return resolved.String(), nil
}

// nextLinkURL extracts the rel="next" target from a Link header (RFC 8288).
func nextLinkURL(header string) string {
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		if len(fields) < 2 {
			continue
		}
		target := strings.Trim(strings.TrimSpace(fields[0]), "<>")
		for _, param := range fields[1:] {
			switch strings.ToLower(strings.TrimSpace(param)) {
			case `rel="next"`, "rel=next", "rel='next'":
				return target
			}
		}
	}
	return ""
}

// retriableError marks failures worth retrying.
type retriableError struct{ err error }

func (e *retriableError) Error() string { return e.err.Error() }
func (e *retriableError) Unwrap() error { return e.err }

// doRequest performs one attempt: build the request, send it, enforce the
// status, content-type and size rules. pageQuery (pagination, GO-062) is
// applied after src.Query so the page counter wins over a static parameter;
// nextLink carries the Link rel="next" target for mode=link pagination.
func (h HTTPConnector) doRequest(client *http.Client, src Source, rawURL string,
	pageQuery map[string]string) (body []byte, contentType, nextLink string, err error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", "", err
	}
	if len(src.Query) > 0 || len(pageQuery) > 0 {
		q := req.URL.Query()
		for k, v := range src.Query {
			q.Set(k, v)
		}
		for k, v := range pageQuery {
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
		return nil, "", "", &retriableError{fmt.Errorf("request to %s failed: %w", safeIdentifier(req.URL), err)}
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return nil, "", "", &retriableError{fmt.Errorf("%s returned status %d", safeIdentifier(req.URL), resp.StatusCode)}
	default:
		return nil, "", "", fmt.Errorf("%s returned status %d", safeIdentifier(req.URL), resp.StatusCode)
	}

	contentType = resp.Header.Get("Content-Type")
	if !contentTypeAccepted(src.Format, contentType) {
		return nil, "", "", fmt.Errorf("%s returned content-type %q, which does not match format %q", safeIdentifier(req.URL), contentType, src.Format)
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, src.MaxSize+1))
	if err != nil {
		return nil, "", "", &retriableError{fmt.Errorf("reading %s: %w", safeIdentifier(req.URL), err)}
	}
	if int64(len(body)) > src.MaxSize {
		return nil, "", "", fmt.Errorf("%s response exceeds the %d-byte limit (defaults.max_response_size)", safeIdentifier(req.URL), src.MaxSize)
	}
	return body, contentType, nextLinkURL(resp.Header.Get("Link")), nil
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
