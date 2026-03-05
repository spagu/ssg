// Package mddb provides a client for the MDDB markdown database
package mddb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
		CreatedAt:  time.Unix(m.AddedAt, 0),
		UpdatedAt:  time.Unix(m.UpdatedAt, 0),
	}
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
	Collection string             `json:"collection"`
	FilterMeta map[string][]any   `json:"filterMeta,omitempty"`
	Sort       string             `json:"sort,omitempty"`   // Field to sort by (e.g., "updatedAt")
	Asc        bool               `json:"asc,omitempty"`    // Sort ascending
	Limit      int                `json:"limit,omitempty"`  // Max results
	Offset     int                `json:"offset,omitempty"` // Skip results
}

// ErrorResponse represents an error from MDDB
type ErrorResponse struct {
	Error string `json:"error"`
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

// Search fetches multiple documents matching filters
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

	// Get total count from header
	totalCount := 0
	if tc := resp.Header.Get("X-Total-Count"); tc != "" {
		totalCount, _ = strconv.Atoi(tc)
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

	// If no header, use array length
	if totalCount == 0 {
		totalCount = len(docs)
	}

	return docs, totalCount, nil
}

// GetAll fetches all documents from a collection with pagination
func (c *Client) GetAll(collection string, lang string, batchSize int) ([]Document, error) {
	if batchSize <= 0 {
		batchSize = c.batchSize
	}

	var allDocs []Document
	offset := 0

	for {
		req := SearchRequest{
			Collection: collection,
			Limit:      batchSize,
			Offset:     offset,
		}

		docs, total, err := c.Search(req)
		if err != nil {
			return nil, fmt.Errorf("fetching batch at offset %d: %w", offset, err)
		}

		allDocs = append(allDocs, docs...)

		if len(allDocs) >= total || len(docs) < batchSize {
			break
		}

		offset += batchSize
	}

	return allDocs, nil
}

// GetByType fetches all documents filtered by type (page or post) with pagination
func (c *Client) GetByType(collection string, docType string, lang string) ([]Document, error) {
	batchSize := c.batchSize

	var allDocs []Document
	offset := 0

	for {
		req := SearchRequest{
			Collection: collection,
			FilterMeta: map[string][]any{
				"type": {docType},
			},
			Limit:  batchSize,
			Offset: offset,
		}

		docs, total, err := c.Search(req)
		if err != nil {
			return nil, fmt.Errorf("fetching batch at offset %d: %w", offset, err)
		}

		allDocs = append(allDocs, docs...)

		if len(allDocs) >= total || len(docs) < batchSize {
			break
		}

		offset += batchSize
	}

	return allDocs, nil
}

// Health checks if the mddb server is available
func (c *Client) Health() error {
	resp, err := c.doRequest("GET", "/v1/health", nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health check failed: %s", string(body))
	}

	return nil
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
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request to %s: %w", url, err)
	}

	// Don't treat 404 as error for Get - let caller handle it
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}
