// Package mddb provides a client for the MDDB markdown database
package mddb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is the mddb HTTP client
type Client struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
}

// Config holds mddb client configuration
type Config struct {
	BaseURL string // Base URL of mddb server (e.g., "http://localhost:8080")
	APIKey  string // Optional API key for authentication
	Timeout int    // Timeout in seconds (default: 30)
}

// Document represents a markdown document from mddb
type Document struct {
	Key        string         `json:"key"`
	Collection string         `json:"collection"`
	Lang       string         `json:"lang"`
	Content    string         `json:"content"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// GetRequest represents a request to fetch a single document
type GetRequest struct {
	Collection string `json:"collection"`
	Key        string `json:"key"`
	Lang       string `json:"lang,omitempty"`
}

// GetResponse represents response from /v1/get endpoint
type GetResponse struct {
	Document Document `json:"document"`
	Success  bool     `json:"success"`
	Error    string   `json:"error,omitempty"`
}

// SearchRequest represents a request to search documents
type SearchRequest struct {
	Collection string         `json:"collection"`
	Lang       string         `json:"lang,omitempty"`
	Filters    map[string]any `json:"filters,omitempty"`
	Limit      int            `json:"limit,omitempty"`
	Offset     int            `json:"offset,omitempty"`
	OrderBy    string         `json:"order_by,omitempty"`
	OrderDir   string         `json:"order_dir,omitempty"` // "asc" or "desc"
}

// SearchResponse represents response from /v1/search endpoint
type SearchResponse struct {
	Documents []Document `json:"documents"`
	Total     int        `json:"total"`
	Success   bool       `json:"success"`
	Error     string     `json:"error,omitempty"`
}

// NewClient creates a new mddb client
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	baseURL := strings.TrimSuffix(cfg.BaseURL, "/")

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		apiKey: cfg.APIKey,
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

	var getResp GetResponse
	if err := json.NewDecoder(resp.Body).Decode(&getResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if !getResp.Success {
		return nil, fmt.Errorf("mddb error: %s", getResp.Error)
	}

	return &getResp.Document, nil
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

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, 0, fmt.Errorf("decoding response: %w", err)
	}

	if !searchResp.Success {
		return nil, 0, fmt.Errorf("mddb error: %s", searchResp.Error)
	}

	return searchResp.Documents, searchResp.Total, nil
}

// GetAll fetches all documents from a collection with pagination
func (c *Client) GetAll(collection string, lang string, batchSize int) ([]Document, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	var allDocs []Document
	offset := 0

	for {
		req := SearchRequest{
			Collection: collection,
			Lang:       lang,
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

// GetByType fetches documents filtered by type (page or post)
func (c *Client) GetByType(collection string, docType string, lang string) ([]Document, error) {
	req := SearchRequest{
		Collection: collection,
		Lang:       lang,
		Filters: map[string]any{
			"type": docType,
		},
		Limit: 1000, // Reasonable default for most sites
	}

	docs, _, err := c.Search(req)
	if err != nil {
		return nil, err
	}

	return docs, nil
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

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}
