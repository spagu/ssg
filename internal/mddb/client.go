// Package mddb provides a client for the MDDB markdown database
package mddb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Client is the mddb HTTP client
type Client struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
	batchSize  int
}

// Config holds mddb client configuration
type Config struct {
	BaseURL   string // Base URL of mddb server (e.g., "http://localhost:8080")
	APIKey    string // Optional API key for authentication
	Timeout   int    // Timeout in seconds (default: 30)
	BatchSize int    // Batch size for pagination (default: 1000)
}

// Document represents a markdown document from mddb
// This is the normalized format used internally by SSG
type Document struct {
	ID         string         `json:"id"`
	Key        string         `json:"key"`
	Collection string         `json:"collection"`
	Lang       string         `json:"lang"`
	Content    string         `json:"content"`    // Mapped from contentMd
	Metadata   map[string]any `json:"metadata"`   // Mapped from meta
	CreatedAt  time.Time      `json:"created_at"` // Mapped from addedAt
	UpdatedAt  time.Time      `json:"updated_at"` // Mapped from updatedAt
}

// mddbDocument represents the raw document format from MDDB API
type mddbDocument struct {
	ID        string           `json:"id"`
	Key       string           `json:"key"`
	Lang      string           `json:"lang"`
	ContentMd string           `json:"contentMd"`
	Meta      map[string][]any `json:"meta"`
	AddedAt   int64            `json:"addedAt"`
	UpdatedAt int64            `json:"updatedAt"`
}

// toDocument converts mddbDocument to Document
func (m *mddbDocument) toDocument(collection string) Document {
	// Flatten meta arrays to single values where appropriate
	metadata := make(map[string]any)
	for k, v := range m.Meta {
		if len(v) == 1 {
			metadata[k] = v[0]
		} else if len(v) > 1 {
			metadata[k] = v
		}
	}

	return Document{
		ID:         m.ID,
		Key:        m.Key,
		Collection: collection,
		Lang:       m.Lang,
		Content:    m.ContentMd,
		Metadata:   metadata,
		CreatedAt:  unixToTime(m.AddedAt),
		UpdatedAt:  unixToTime(m.UpdatedAt),
	}
}

// unixToTime converts a Unix seconds timestamp to a UTC time.Time. A zero
// timestamp means "no date": it maps to the zero time.Time so IsZero() guards
// fire instead of publishing posts under /1970/01/01/, and .UTC() keeps
// date-based URLs reproducible across build machines (GO-031).
func unixToTime(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

// GetRequest represents a request to fetch a single document
type GetRequest struct {
	Collection string            `json:"collection"`
	Key        string            `json:"key"`
	Lang       string            `json:"lang,omitempty"`
	Env        map[string]string `json:"env,omitempty"` // Template variables
}

// SearchRequest represents a request to search documents
type SearchRequest struct {
	Collection string           `json:"collection"`
	Lang       string           `json:"lang,omitempty"` // Language filter, e.g. "en_US" (GO-013)
	FilterMeta map[string][]any `json:"filterMeta,omitempty"`
	Sort       string           `json:"sort,omitempty"`   // Field to sort by (e.g., "updatedAt")
	Asc        bool             `json:"asc,omitempty"`    // Sort ascending
	Limit      int              `json:"limit,omitempty"`  // Max results
	Offset     int              `json:"offset,omitempty"` // Skip results
}

// NewClient creates a new mddb client
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	baseURL := strings.TrimSuffix(cfg.BaseURL, "/")

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		apiKey:    cfg.APIKey,
		batchSize: batchSize,
	}
}

// Close satisfies MddbClient. The HTTP client holds no long-lived connection
// that must be torn down (the stdlib transport pools and reaps idle
// connections), so this is a no-op provided for interface symmetry (GO-005).
func (c *Client) Close() error {
	return nil
}

// Response-size limits bound a single mddb HTTP response so a malicious or
// broken server cannot exhaust memory by streaming unbounded data (SEC-009).
const (
	maxResponseSize = 64 * 1024 * 1024 // document payloads
	maxErrBodySize  = 64 * 1024        // error message bodies
)

// limitedBody wraps a response body so reads are capped at limit while Close
// still closes the underlying connection.
func limitedBody(body io.ReadCloser, limit int64) io.ReadCloser {
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(body, limit), body}
}

// Get fetches a single document by collection and key
func (c *Client) Get(req GetRequest) (*Document, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := c.doRequest("POST", "/v1/get", body)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Check for error response
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("document not found: %s/%s", req.Collection, req.Key)
	}

	var mddbDoc mddbDocument
	if err := json.NewDecoder(resp.Body).Decode(&mddbDoc); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	doc := mddbDoc.toDocument(req.Collection)
	return &doc, nil
}

// Search fetches multiple documents matching filters. The returned total is
// the server-reported X-Total-Count, or 0 when the header is absent/malformed
// — callers must not treat the batch length as the collection total (GO-015).
func (c *Client) Search(req SearchRequest) ([]Document, int, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := c.doRequest("POST", "/v1/search", body)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Get total count from header; 0 means unknown (GO-015). A malformed
	// value is logged instead of silently swallowed so truncated builds are
	// diagnosable.
	totalCount := 0
	if tc := resp.Header.Get("X-Total-Count"); tc != "" {
		n, convErr := strconv.Atoi(tc)
		if convErr != nil || n < 0 {
			fmt.Fprintf(os.Stderr,
				"Warning: mddb sent malformed X-Total-Count header %q; paginating until an empty batch (GO-015)\n", tc)
		} else {
			totalCount = n
		}
	}

	var mddbDocs []mddbDocument
	if err := json.NewDecoder(resp.Body).Decode(&mddbDocs); err != nil {
		return nil, 0, fmt.Errorf("decoding response: %w", err)
	}

	// Convert to Document format
	docs := make([]Document, len(mddbDocs))
	for i, mddbDoc := range mddbDocs {
		docs[i] = mddbDoc.toDocument(req.Collection)
	}

	return docs, totalCount, nil
}

// filterDocsByLang drops documents in other languages. It is the client-side
// safety net for GO-013: servers that ignore SearchRequest.Lang (and the gRPC
// transport, whose proto SearchRequest has no lang field) still yield a
// single-language result. Documents without a lang are kept so metadata
// collections (categories, media, users) survive the filter.
func filterDocsByLang(docs []Document, lang string) []Document {
	if lang == "" {
		return docs
	}
	filtered := make([]Document, 0, len(docs))
	for _, doc := range docs {
		if doc.Lang == "" || doc.Lang == lang {
			filtered = append(filtered, doc)
		}
	}
	return filtered
}

// getAllPaginated drives search until the collection is exhausted. One
// coherent loop fixes GO-015 and GO-041: the offset advances by the number of
// documents actually received (servers may clamp the page size below the
// requested batch), the loop stops on an empty batch instead of trusting the
// first batch length as a total, and a positive server-reported total still
// allows an early stop without an extra request. Shared by the HTTP and gRPC
// clients (DRY).
func getAllPaginated(search func(SearchRequest) ([]Document, int, error),
	collection, lang string, filterMeta map[string][]any, batchSize int) ([]Document, error) {
	var allDocs []Document
	offset := 0

	for {
		req := SearchRequest{
			Collection: collection,
			Lang:       lang, // GO-013: propagate the language filter
			FilterMeta: filterMeta,
			Limit:      batchSize,
			Offset:     offset,
		}

		docs, total, err := search(req)
		if err != nil {
			return nil, fmt.Errorf("fetching batch at offset %d: %w", offset, err)
		}
		if len(docs) == 0 {
			break
		}

		allDocs = append(allDocs, filterDocsByLang(docs, lang)...)
		offset += len(docs)

		if total > 0 && offset >= total {
			break
		}
	}

	return allDocs, nil
}

// GetAll fetches all documents from a collection with pagination
func (c *Client) GetAll(collection string, lang string, batchSize int) ([]Document, error) {
	if batchSize <= 0 {
		batchSize = c.batchSize
	}

	return getAllPaginated(c.Search, collection, lang, nil, batchSize)
}

// GetByType fetches all documents filtered by type (page or post) with pagination
func (c *Client) GetByType(collection string, docType string, lang string) ([]Document, error) {
	return getAllPaginated(c.Search, collection, lang,
		map[string][]any{"type": {docType}}, c.batchSize)
}

// Health checks if the mddb server is available
func (c *Client) Health() error {
	resp, err := c.doRequest("GET", "/v1/health", nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBodySize))
		return fmt.Errorf("health check failed: %s", string(body))
	}

	return nil
}

// ChecksumResponse represents the response from /v1/checksum endpoint
type ChecksumResponse struct {
	Collection    string `json:"collection"`
	Checksum      string `json:"checksum"`
	DocumentCount int    `json:"documentCount"`
}

// Checksum returns the checksum for a collection (for change detection)
func (c *Client) Checksum(collection string) (*ChecksumResponse, error) {
	// GO-041: escape the collection name so '&', spaces etc. cannot alter the query
	query := url.Values{}
	query.Set("collection", collection)
	endpoint := "/v1/checksum?" + query.Encode()
	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBodySize))
		return nil, fmt.Errorf("checksum request failed: %s", string(body))
	}

	var checksumResp ChecksumResponse
	if err := json.NewDecoder(resp.Body).Decode(&checksumResp); err != nil {
		return nil, fmt.Errorf("decoding checksum response: %w", err)
	}

	return &checksumResp, nil
}

// ensureSecureForAPIKey refuses to attach a Bearer API key over plaintext http://
// to a non-loopback host, preventing credential leakage over untrusted networks
// (SEC-007). https:// and loopback hosts (localhost, 127.0.0.0/8, ::1) are allowed.
func ensureSecureForAPIKey(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parsing mddb URL %q: %w", rawURL, err)
	}
	if u.Scheme == "https" || isLoopbackHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf(
		"refusing to send API key over plaintext %s:// to non-loopback host %q; use https://",
		u.Scheme, u.Hostname())
}

// isLoopbackHost reports whether host is localhost or a loopback IP.
func isLoopbackHost(host string) bool {
	if host == "localhost" || host == "" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// doRequest performs an HTTP request to the mddb server
func (c *Client) doRequest(method, endpoint string, body []byte) (*http.Response, error) {
	url := c.baseURL + endpoint

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.apiKey != "" {
		if err := ensureSecureForAPIKey(c.baseURL); err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request to %s: %w", url, err)
	}

	// Don't treat 404 as error for Get - let caller handle it
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBodySize))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SEC-009: cap every successful body so downstream decoders/readers are bounded.
	resp.Body = limitedBody(resp.Body, maxResponseSize)
	return resp, nil
}
