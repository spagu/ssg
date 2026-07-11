package mddb

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pb "github.com/spagu/ssg/internal/mddb/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantURL string
	}{
		{
			name:    "basic config",
			cfg:     Config{BaseURL: "http://localhost:8080"},
			wantURL: "http://localhost:8080",
		},
		{
			name:    "trailing slash removed",
			cfg:     Config{BaseURL: "http://localhost:8080/"},
			wantURL: "http://localhost:8080",
		},
		{
			name:    "with api key",
			cfg:     Config{BaseURL: "http://localhost:8080", APIKey: "secret"},
			wantURL: "http://localhost:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.cfg)
			if client.baseURL != tt.wantURL {
				t.Errorf("NewClient().baseURL = %v, want %v", client.baseURL, tt.wantURL)
			}
			if client.apiKey != tt.cfg.APIKey {
				t.Errorf("NewClient().apiKey = %v, want %v", client.apiKey, tt.cfg.APIKey)
			}
		})
	}
}

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/get" {
			t.Errorf("Expected /v1/get, got %s", r.URL.Path)
		}

		var req GetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}

		// Return MDDB format response
		resp := mddbDocument{
			ID:        "doc|blog|hello-world|en_US",
			Key:       req.Key,
			Lang:      "en_US",
			ContentMd: "# Test Content",
			Meta: map[string][]any{
				"title": {"Test Title"},
				"type":  {"post"},
			},
			AddedAt:   1704844800,
			UpdatedAt: 1704931200,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})

	doc, err := client.Get(GetRequest{
		Collection: "blog",
		Key:        "hello-world",
	})

	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if doc.Key != "hello-world" {
		t.Errorf("doc.Key = %v, want hello-world", doc.Key)
	}

	if doc.Metadata["title"] != "Test Title" {
		t.Errorf("doc.Metadata[title] = %v, want Test Title", doc.Metadata["title"])
	}

	if doc.Content != "# Test Content" {
		t.Errorf("doc.Content = %v, want # Test Content", doc.Content)
	}
}

func TestClient_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/search" {
			t.Errorf("Expected /v1/search, got %s", r.URL.Path)
		}

		// Return MDDB format - array with X-Total-Count header
		resp := []mddbDocument{
			{
				ID:        "doc|blog|post-1|en_US",
				Key:       "post-1",
				Lang:      "en_US",
				ContentMd: "# Post 1",
				Meta:      map[string][]any{"title": {"Post 1"}},
				AddedAt:   1704844800,
				UpdatedAt: 1704931200,
			},
			{
				ID:        "doc|blog|post-2|en_US",
				Key:       "post-2",
				Lang:      "en_US",
				ContentMd: "# Post 2",
				Meta:      map[string][]any{"title": {"Post 2"}},
				AddedAt:   1704844801,
				UpdatedAt: 1704931201,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "2")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})

	docs, total, err := client.Search(SearchRequest{
		Collection: "blog",
		Limit:      10,
	})

	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if total != 2 {
		t.Errorf("total = %v, want 2", total)
	}

	if len(docs) != 2 {
		t.Errorf("len(docs) = %v, want 2", len(docs))
	}
}

func TestClient_GetAll(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		var req SearchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		var docs []mddbDocument
		if req.Offset == 0 {
			docs = []mddbDocument{
				{Key: "post-1", Lang: "en_US", ContentMd: "# Post 1"},
				{Key: "post-2", Lang: "en_US", ContentMd: "# Post 2"},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "2")
		_ = json.NewEncoder(w).Encode(docs)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})

	docs, err := client.GetAll("blog", "", 10)

	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("len(docs) = %v, want 2", len(docs))
	}
}

func TestClient_Health(t *testing.T) {
	t.Run("healthy server", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/health" {
				t.Errorf("Expected /v1/health, got %s", r.URL.Path)
			}
			if r.Method != "GET" {
				t.Errorf("Expected GET, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"healthy","mode":"wr"}`))
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL})

		if err := client.Health(); err != nil {
			t.Errorf("Health() error = %v, want nil", err)
		}
	})

	t.Run("unhealthy server", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"service unavailable"}`))
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL})

		if err := client.Health(); err == nil {
			t.Error("Health() error = nil, want error")
		}
	})
}

func TestClient_GetByType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SearchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Check filterMeta instead of filters
		if req.FilterMeta == nil {
			t.Error("Expected filterMeta to be set")
		}
		typeFilter, ok := req.FilterMeta["type"]
		if !ok || len(typeFilter) == 0 || typeFilter[0] != "post" {
			t.Errorf("Expected filterMeta[type] = [post], got %v", req.FilterMeta["type"])
		}

		resp := []mddbDocument{
			{
				Key:       "post-1",
				Lang:      "en_US",
				ContentMd: "# Post 1",
				Meta:      map[string][]any{"type": {"post"}},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "1")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})

	docs, err := client.GetByType("blog", "post", "")

	if err != nil {
		t.Fatalf("GetByType() error = %v", err)
	}

	if len(docs) != 1 {
		t.Errorf("len(docs) = %v, want 1", len(docs))
	}
}

func TestClient_WithAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			t.Errorf("Expected Authorization header 'Bearer test-api-key', got '%s'", auth)
		}

		resp := mddbDocument{
			Key:       "test",
			Lang:      "en_US",
			ContentMd: "# Test",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "test-api-key",
	})

	_, err := client.Get(GetRequest{Collection: "blog", Key: "test"})

	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestClient_ErrorHandling(t *testing.T) {
	t.Run("document not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"document not found"}`))
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL})

		_, err := client.Get(GetRequest{Collection: "blog", Key: "missing"})

		if err == nil {
			t.Error("Get() error = nil, want error")
		}
	})

	t.Run("HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL})

		_, err := client.Get(GetRequest{Collection: "blog", Key: "test"})

		if err == nil {
			t.Error("Get() error = nil, want error")
		}
	})
}

func TestClient_Checksum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/checksum" {
			t.Errorf("Expected /v1/checksum, got %s", r.URL.Path)
		}

		// Check query parameter
		collection := r.URL.Query().Get("collection")
		if collection != "blog" {
			t.Errorf("Expected collection=blog, got %s", collection)
		}

		resp := ChecksumResponse{
			Collection:    "blog",
			Checksum:      "a1b2c3d4",
			DocumentCount: 42,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})

	checksumResp, err := client.Checksum("blog")

	if err != nil {
		t.Fatalf("Checksum() error = %v", err)
	}

	if checksumResp.Collection != "blog" {
		t.Errorf("checksumResp.Collection = %v, want blog", checksumResp.Collection)
	}

	if checksumResp.Checksum != "a1b2c3d4" {
		t.Errorf("checksumResp.Checksum = %v, want a1b2c3d4", checksumResp.Checksum)
	}

	if checksumResp.DocumentCount != 42 {
		t.Errorf("checksumResp.DocumentCount = %v, want 42", checksumResp.DocumentCount)
	}
}

func TestNewClient_Defaults(t *testing.T) {
	client := NewClient(Config{BaseURL: "http://localhost:8080"})

	if client.batchSize != 1000 {
		t.Errorf("default batchSize = %v, want 1000", client.batchSize)
	}

	if client.httpClient.Timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", client.httpClient.Timeout)
	}
}

func TestNewClient_CustomValues(t *testing.T) {
	client := NewClient(Config{
		BaseURL:   "http://localhost:8080",
		Timeout:   60,
		BatchSize: 500,
	})

	if client.batchSize != 500 {
		t.Errorf("batchSize = %v, want 500", client.batchSize)
	}

	if client.httpClient.Timeout != 60*time.Second {
		t.Errorf("timeout = %v, want 60s", client.httpClient.Timeout)
	}
}

func TestClient_Get_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	_, err := client.Get(GetRequest{Collection: "blog", Key: "missing"})

	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "document not found") {
		t.Errorf("error = %v, want 'document not found'", err)
	}
}

func TestClient_Get_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	_, err := client.Get(GetRequest{Collection: "blog", Key: "test"})

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "decoding response") {
		t.Errorf("error = %v, want 'decoding response'", err)
	}
}

func TestClient_Search_NoTotalCountHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := []mddbDocument{
			{Key: "post-1", Lang: "en_US", ContentMd: "# Post 1"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	docs, total, err := client.Search(SearchRequest{Collection: "blog"})

	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	// GO-015: a missing header means the total is unknown (0) — the batch
	// length must NOT be reported as the collection total, or pagination
	// would silently stop after the first batch.
	if total != 0 {
		t.Errorf("total = %v, want 0 (unknown without X-Total-Count)", total)
	}
	if len(docs) != 1 {
		t.Errorf("len(docs) = %v, want 1", len(docs))
	}
}

func TestClient_Search_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	_, _, err := client.Search(SearchRequest{Collection: "blog"})

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestClient_Search_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	_, _, err := client.Search(SearchRequest{Collection: "blog"})

	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestClient_GetAll_DefaultBatchSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := []mddbDocument{{Key: "post-1", Lang: "en_US"}}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "1")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, BatchSize: 50})
	docs, err := client.GetAll("blog", "", 0)

	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("len(docs) = %v, want 1", len(docs))
	}
}

func TestClient_GetAll_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	_, err := client.GetAll("blog", "", 10)

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "fetching batch") {
		t.Errorf("error = %v, want 'fetching batch'", err)
	}
}

func TestClient_GetAll_Pagination(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req SearchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		var docs []mddbDocument
		if req.Offset == 0 {
			docs = []mddbDocument{
				{Key: "post-1", Lang: "en_US"},
				{Key: "post-2", Lang: "en_US"},
			}
		} else {
			docs = []mddbDocument{
				{Key: "post-3", Lang: "en_US"},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "3")
		_ = json.NewEncoder(w).Encode(docs)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	docs, err := client.GetAll("blog", "", 2)

	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if len(docs) != 3 {
		t.Errorf("len(docs) = %v, want 3", len(docs))
	}
	if callCount != 2 {
		t.Errorf("callCount = %v, want 2", callCount)
	}
}

func TestClient_GetByType_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	_, err := client.GetByType("blog", "post", "")

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_GetByType_Pagination(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req SearchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		var docs []mddbDocument
		if req.Offset == 0 {
			docs = []mddbDocument{
				{Key: "post-1", Lang: "en_US", Meta: map[string][]any{"type": {"post"}}},
				{Key: "post-2", Lang: "en_US", Meta: map[string][]any{"type": {"post"}}},
			}
		} else {
			docs = []mddbDocument{
				{Key: "post-3", Lang: "en_US", Meta: map[string][]any{"type": {"post"}}},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "3")
		_ = json.NewEncoder(w).Encode(docs)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, BatchSize: 2})
	docs, err := client.GetByType("blog", "post", "")

	if err != nil {
		t.Fatalf("GetByType() error = %v", err)
	}
	if len(docs) != 3 {
		t.Errorf("len(docs) = %v, want 3", len(docs))
	}
}

func TestClient_Health_NonOKBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("maintenance mode"))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	err := client.Health()

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "request failed with status 503") {
		t.Errorf("error = %v, want status 503", err)
	}
}

func TestClient_Health_ConnectionRefused(t *testing.T) {
	client := NewClient(Config{BaseURL: "http://127.0.0.1:1", Timeout: 1})
	err := client.Health()

	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestClient_Checksum_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	_, err := client.Checksum("blog")

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_Checksum_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	_, err := client.Checksum("blog")

	if err == nil {
		t.Fatal("expected error for non-OK status")
	}
}

func TestClient_Checksum_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	_, err := client.Checksum("blog")

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "decoding checksum response") {
		t.Errorf("error = %v, want 'decoding checksum response'", err)
	}
}

func TestClient_DoRequest_NoAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			t.Errorf("expected no Authorization header, got %q", authHeader)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	resp, err := client.doRequest("GET", "/v1/health", nil)
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	_ = resp.Body.Close()
}

func TestClient_DoRequest_WithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", contentType)
		}
		acceptHeader := r.Header.Get("Accept")
		if acceptHeader != "application/json" {
			t.Errorf("Accept = %q, want application/json", acceptHeader)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	resp, err := client.doRequest("POST", "/v1/get", []byte(`{"key":"test"}`))
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	_ = resp.Body.Close()
}

func TestMddbDocument_ToDocument_EmptyMeta(t *testing.T) {
	mddbDoc := mddbDocument{
		ID:        "doc|blog|test|en_US",
		Key:       "test",
		Lang:      "en_US",
		ContentMd: "content",
		Meta:      map[string][]any{},
		AddedAt:   1000,
		UpdatedAt: 2000,
	}

	doc := mddbDoc.toDocument("blog")

	if len(doc.Metadata) != 0 {
		t.Errorf("expected empty metadata, got %v", doc.Metadata)
	}
}

func TestMddbDocument_ToDocument_EmptyArrayMeta(t *testing.T) {
	mddbDoc := mddbDocument{
		ID:   "doc|blog|test|en_US",
		Key:  "test",
		Meta: map[string][]any{"emptyField": {}},
	}

	doc := mddbDoc.toDocument("blog")

	if _, exists := doc.Metadata["emptyField"]; exists {
		t.Error("empty array meta field should not be in metadata")
	}
}

// --- gRPC test infrastructure ---

const bufSize = 1024 * 1024

type mockMDDBServer struct {
	pb.UnimplementedMDDBServer
	getFunc    func(ctx context.Context, req *pb.GetRequest) (*pb.Document, error)
	searchFunc func(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error)
	statsFunc  func(ctx context.Context, req *pb.StatsRequest) (*pb.StatsResponse, error)
}

func (m *mockMDDBServer) Get(ctx context.Context, req *pb.GetRequest) (*pb.Document, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, req)
	}
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func (m *mockMDDBServer) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, req)
	}
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func (m *mockMDDBServer) Stats(ctx context.Context, req *pb.StatsRequest) (*pb.StatsResponse, error) {
	if m.statsFunc != nil {
		return m.statsFunc(ctx, req)
	}
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func newTestGRPCClient(t *testing.T, srv *mockMDDBServer) (*GRPCClient, func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	pb.RegisterMDDBServer(grpcServer, srv)

	go func() { _ = grpcServer.Serve(lis) }()

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}

	grpcClient := &GRPCClient{
		conn:      conn,
		client:    pb.NewMDDBClient(conn),
		apiKey:    "test-key",
		batchSize: 1000,
		timeout:   5 * time.Second,
	}

	cleanup := func() {
		_ = conn.Close()
		grpcServer.Stop()
		_ = lis.Close()
	}

	return grpcClient, cleanup
}

// --- gRPC client tests ---

func TestNewGRPCClient(t *testing.T) {
	client, err := NewGRPCClient(GRPCConfig{
		Address: "localhost:11024",
	})
	if err != nil {
		t.Fatalf("NewGRPCClient() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	if client.batchSize != 1000 {
		t.Errorf("default batchSize = %v, want 1000", client.batchSize)
	}
	if client.timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", client.timeout)
	}
}

func TestNewGRPCClient_CustomValues(t *testing.T) {
	client, err := NewGRPCClient(GRPCConfig{
		Address:   "localhost:11024",
		APIKey:    "my-key",
		Timeout:   60,
		BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("NewGRPCClient() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	if client.batchSize != 500 {
		t.Errorf("batchSize = %v, want 500", client.batchSize)
	}
	if client.timeout != 60*time.Second {
		t.Errorf("timeout = %v, want 60s", client.timeout)
	}
	if client.apiKey != "my-key" {
		t.Errorf("apiKey = %v, want my-key", client.apiKey)
	}
}

func TestNewGRPCClient_StripProtocol(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{"grpc prefix", "grpc://localhost:11024"},
		{"http prefix", "http://localhost:11024"},
		{"https prefix", "https://localhost:11024"},
		{"no prefix", "localhost:11024"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewGRPCClient(GRPCConfig{Address: tt.address})
			if err != nil {
				t.Fatalf("NewGRPCClient() error = %v", err)
			}
			_ = client.Close()
		})
	}
}

func TestGRPCClient_Close(t *testing.T) {
	client, err := NewGRPCClient(GRPCConfig{Address: "localhost:11024"})
	if err != nil {
		t.Fatalf("NewGRPCClient() error = %v", err)
	}

	if err := client.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestGRPCClient_Get(t *testing.T) {
	srv := &mockMDDBServer{
		getFunc: func(_ context.Context, req *pb.GetRequest) (*pb.Document, error) {
			return &pb.Document{
				Id:        "doc|blog|hello|en_US",
				Key:       req.Key,
				Lang:      req.Lang,
				ContentMd: "# Hello",
				Meta: map[string]*pb.MetaValues{
					"title": {Values: []string{"Hello World"}},
				},
				AddedAt:   1704844800,
				UpdatedAt: 1704931200,
			}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	doc, err := grpcClient.Get(GetRequest{
		Collection: "blog",
		Key:        "hello",
		Lang:       "en_US",
		Env:        map[string]string{"foo": "bar"},
	})

	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if doc.Key != "hello" {
		t.Errorf("doc.Key = %v, want hello", doc.Key)
	}
	if doc.Content != "# Hello" {
		t.Errorf("doc.Content = %v, want '# Hello'", doc.Content)
	}
	if doc.Metadata["title"] != "Hello World" {
		t.Errorf("doc.Metadata[title] = %v, want 'Hello World'", doc.Metadata["title"])
	}
	if doc.Collection != "blog" {
		t.Errorf("doc.Collection = %v, want blog", doc.Collection)
	}
}

func TestGRPCClient_Get_Error(t *testing.T) {
	srv := &mockMDDBServer{
		getFunc: func(_ context.Context, _ *pb.GetRequest) (*pb.Document, error) {
			return nil, status.Errorf(codes.NotFound, "document not found")
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	_, err := grpcClient.Get(GetRequest{Collection: "blog", Key: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gRPC Get") {
		t.Errorf("error = %v, want 'gRPC Get'", err)
	}
}

func TestGRPCClient_Search(t *testing.T) {
	srv := &mockMDDBServer{
		searchFunc: func(_ context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
			return &pb.SearchResponse{
				Documents: []*pb.Document{
					{
						Id:        "doc|blog|p1|en_US",
						Key:       "p1",
						Lang:      "en_US",
						ContentMd: "# P1",
						Meta: map[string]*pb.MetaValues{
							"title": {Values: []string{"Post 1"}},
						},
					},
					{
						Id:        "doc|blog|p2|en_US",
						Key:       "p2",
						Lang:      "en_US",
						ContentMd: "# P2",
					},
				},
				Total: 2,
			}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	docs, total, err := grpcClient.Search(SearchRequest{
		Collection: "blog",
		FilterMeta: map[string][]any{"type": {"post"}},
		Sort:       "updatedAt",
		Asc:        true,
		Limit:      10,
		Offset:     0,
	})

	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if total != 2 {
		t.Errorf("total = %v, want 2", total)
	}
	if len(docs) != 2 {
		t.Errorf("len(docs) = %v, want 2", len(docs))
	}
}

func TestGRPCClient_Search_Error(t *testing.T) {
	srv := &mockMDDBServer{
		searchFunc: func(_ context.Context, _ *pb.SearchRequest) (*pb.SearchResponse, error) {
			return nil, status.Errorf(codes.Internal, "search failed")
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	_, _, err := grpcClient.Search(SearchRequest{Collection: "blog"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGRPCClient_Search_BoundsCheck(t *testing.T) {
	srv := &mockMDDBServer{
		searchFunc: func(_ context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
			if req.Limit < 0 || req.Offset < 0 {
				t.Error("limit or offset is negative")
			}
			return &pb.SearchResponse{Documents: nil, Total: 0}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	_, _, err := grpcClient.Search(SearchRequest{
		Collection: "blog",
		Limit:      3000000000,
		Offset:     3000000000,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
}

func TestGRPCClient_GetAll(t *testing.T) {
	callCount := 0
	srv := &mockMDDBServer{
		searchFunc: func(_ context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
			callCount++
			if req.Offset == 0 {
				return &pb.SearchResponse{
					Documents: []*pb.Document{
						{Key: "p1", Lang: "en_US"},
						{Key: "p2", Lang: "en_US"},
					},
					Total: 3,
				}, nil
			}
			return &pb.SearchResponse{
				Documents: []*pb.Document{
					{Key: "p3", Lang: "en_US"},
				},
				Total: 3,
			}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	docs, err := grpcClient.GetAll("blog", "", 2)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if len(docs) != 3 {
		t.Errorf("len(docs) = %v, want 3", len(docs))
	}
	if callCount != 2 {
		t.Errorf("callCount = %v, want 2", callCount)
	}
}

func TestGRPCClient_GetAll_DefaultBatchSize(t *testing.T) {
	srv := &mockMDDBServer{
		searchFunc: func(_ context.Context, _ *pb.SearchRequest) (*pb.SearchResponse, error) {
			return &pb.SearchResponse{Documents: []*pb.Document{{Key: "p1"}}, Total: 1}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	docs, err := grpcClient.GetAll("blog", "", 0)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("len(docs) = %v, want 1", len(docs))
	}
}

func TestGRPCClient_GetAll_Error(t *testing.T) {
	srv := &mockMDDBServer{
		searchFunc: func(_ context.Context, _ *pb.SearchRequest) (*pb.SearchResponse, error) {
			return nil, status.Errorf(codes.Internal, "fail")
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	_, err := grpcClient.GetAll("blog", "", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGRPCClient_GetByType(t *testing.T) {
	srv := &mockMDDBServer{
		searchFunc: func(_ context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
			if req.FilterMeta == nil {
				t.Error("expected filterMeta to be set")
			}
			return &pb.SearchResponse{
				Documents: []*pb.Document{
					{Key: "p1", Lang: "en_US"},
				},
				Total: 1,
			}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	docs, err := grpcClient.GetByType("blog", "post", "")
	if err != nil {
		t.Fatalf("GetByType() error = %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("len(docs) = %v, want 1", len(docs))
	}
}

func TestGRPCClient_GetByType_Error(t *testing.T) {
	srv := &mockMDDBServer{
		searchFunc: func(_ context.Context, _ *pb.SearchRequest) (*pb.SearchResponse, error) {
			return nil, status.Errorf(codes.Internal, "fail")
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	_, err := grpcClient.GetByType("blog", "post", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGRPCClient_GetByType_Pagination(t *testing.T) {
	callCount := 0
	srv := &mockMDDBServer{
		searchFunc: func(_ context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
			callCount++
			if req.Offset == 0 {
				return &pb.SearchResponse{
					Documents: []*pb.Document{
						{Key: "p1"},
						{Key: "p2"},
					},
					Total: 3,
				}, nil
			}
			return &pb.SearchResponse{
				Documents: []*pb.Document{{Key: "p3"}},
				Total:     3,
			}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()
	grpcClient.batchSize = 2

	docs, err := grpcClient.GetByType("blog", "post", "")
	if err != nil {
		t.Fatalf("GetByType() error = %v", err)
	}
	if len(docs) != 3 {
		t.Errorf("len(docs) = %v, want 3", len(docs))
	}
}

func TestGRPCClient_Health(t *testing.T) {
	srv := &mockMDDBServer{
		statsFunc: func(_ context.Context, _ *pb.StatsRequest) (*pb.StatsResponse, error) {
			return &pb.StatsResponse{Mode: "wr"}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	if err := grpcClient.Health(); err != nil {
		t.Errorf("Health() error = %v", err)
	}
}

func TestGRPCClient_Health_Error(t *testing.T) {
	srv := &mockMDDBServer{
		statsFunc: func(_ context.Context, _ *pb.StatsRequest) (*pb.StatsResponse, error) {
			return nil, status.Errorf(codes.Unavailable, "down")
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	if err := grpcClient.Health(); err == nil {
		t.Error("expected error")
	}
}

func TestGRPCClient_Stats(t *testing.T) {
	srv := &mockMDDBServer{
		statsFunc: func(_ context.Context, _ *pb.StatsRequest) (*pb.StatsResponse, error) {
			return &pb.StatsResponse{
				Mode:           "wr",
				TotalDocuments: 100,
				Collections: []*pb.CollectionStats{
					{Name: "blog", DocumentCount: 50, RevisionCount: 200},
				},
			}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	statsResp, err := grpcClient.Stats()
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if statsResp.TotalDocuments != 100 {
		t.Errorf("TotalDocuments = %v, want 100", statsResp.TotalDocuments)
	}
}

func TestGRPCClient_Stats_Error(t *testing.T) {
	srv := &mockMDDBServer{
		statsFunc: func(_ context.Context, _ *pb.StatsRequest) (*pb.StatsResponse, error) {
			return nil, status.Errorf(codes.Internal, "stats fail")
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	_, err := grpcClient.Stats()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGRPCClient_Checksum(t *testing.T) {
	srv := &mockMDDBServer{
		statsFunc: func(_ context.Context, _ *pb.StatsRequest) (*pb.StatsResponse, error) {
			return &pb.StatsResponse{
				Collections: []*pb.CollectionStats{
					{Name: "blog", DocumentCount: 42, RevisionCount: 100},
					{Name: "pages", DocumentCount: 10, RevisionCount: 20},
				},
			}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	checksumResp, err := grpcClient.Checksum("blog")
	if err != nil {
		t.Fatalf("Checksum() error = %v", err)
	}
	if checksumResp.Collection != "blog" {
		t.Errorf("Collection = %v, want blog", checksumResp.Collection)
	}
	if checksumResp.DocumentCount != 42 {
		t.Errorf("DocumentCount = %v, want 42", checksumResp.DocumentCount)
	}
	if checksumResp.Checksum != "42-100" {
		t.Errorf("Checksum = %v, want '42-100'", checksumResp.Checksum)
	}
}

func TestGRPCClient_Checksum_CollectionNotFound(t *testing.T) {
	srv := &mockMDDBServer{
		statsFunc: func(_ context.Context, _ *pb.StatsRequest) (*pb.StatsResponse, error) {
			return &pb.StatsResponse{
				Collections: []*pb.CollectionStats{
					{Name: "other", DocumentCount: 1},
				},
			}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	_, err := grpcClient.Checksum("blog")
	if err == nil {
		t.Fatal("expected error for missing collection")
	}
	if !strings.Contains(err.Error(), "collection blog not found") {
		t.Errorf("error = %v, want 'collection blog not found'", err)
	}
}

func TestGRPCClient_Checksum_StatsError(t *testing.T) {
	srv := &mockMDDBServer{
		statsFunc: func(_ context.Context, _ *pb.StatsRequest) (*pb.StatsResponse, error) {
			return nil, status.Errorf(codes.Internal, "fail")
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	_, err := grpcClient.Checksum("blog")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGRPCClient_ContextWithAuth_NoAPIKey(t *testing.T) {
	client := &GRPCClient{
		apiKey:  "",
		timeout: 5 * time.Second,
	}

	ctx, cancel := client.contextWithAuth()
	defer cancel()

	if ctx == nil {
		t.Fatal("context should not be nil")
	}
}

func TestProtoMetaToMetadata(t *testing.T) {
	protoMeta := map[string]*pb.MetaValues{
		"title":  {Values: []string{"Hello"}},
		"tags":   {Values: []string{"go", "grpc"}},
		"empty":  nil,
		"noVals": {Values: []string{}},
	}

	metadata := protoMetaToMetadata(protoMeta)

	if metadata["title"] != "Hello" {
		t.Errorf("title = %v, want Hello", metadata["title"])
	}

	tags, ok := metadata["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Errorf("tags = %v, want [go grpc]", metadata["tags"])
	}

	if _, exists := metadata["empty"]; exists {
		t.Error("nil MetaValues should not appear in metadata")
	}

	if _, exists := metadata["noVals"]; exists {
		t.Error("empty Values should not appear in metadata")
	}
}

func TestProtoDocToDocument(t *testing.T) {
	protoDoc := &pb.Document{
		Id:        "doc|blog|test|en_US",
		Key:       "test",
		Lang:      "en_US",
		ContentMd: "# Test",
		Meta: map[string]*pb.MetaValues{
			"title": {Values: []string{"Test Title"}},
		},
		AddedAt:   1704844800,
		UpdatedAt: 1704931200,
	}

	doc := protoDocToDocument(protoDoc, "blog")

	if doc.ID != "doc|blog|test|en_US" {
		t.Errorf("ID = %v, want doc|blog|test|en_US", doc.ID)
	}
	if doc.Collection != "blog" {
		t.Errorf("Collection = %v, want blog", doc.Collection)
	}
	if doc.Content != "# Test" {
		t.Errorf("Content = %v, want '# Test'", doc.Content)
	}
	if doc.CreatedAt.Unix() != 1704844800 {
		t.Errorf("CreatedAt = %v, want 1704844800", doc.CreatedAt.Unix())
	}
}

func TestMetadataToProtoMeta(t *testing.T) {
	meta := map[string][]any{
		"type":  {"post"},
		"tags":  {"go", 42},
		"empty": {},
	}

	protoMeta := metadataToProtoMeta(meta)

	if len(protoMeta["type"].Values) != 1 || protoMeta["type"].Values[0] != "post" {
		t.Errorf("type = %v, want [post]", protoMeta["type"].Values)
	}
	if len(protoMeta["tags"].Values) != 2 {
		t.Errorf("tags len = %v, want 2", len(protoMeta["tags"].Values))
	}
	if protoMeta["tags"].Values[1] != "42" {
		t.Errorf("tags[1] = %v, want '42'", protoMeta["tags"].Values[1])
	}
	if len(protoMeta["empty"].Values) != 0 {
		t.Errorf("empty = %v, want []", protoMeta["empty"].Values)
	}
}

// --- NewMddbClient factory tests ---

func TestNewMddbClient_HTTP(t *testing.T) {
	client, err := NewMddbClient(ClientConfig{
		URL:       "http://localhost:8080",
		Protocol:  "http",
		APIKey:    "key",
		Timeout:   10,
		BatchSize: 200,
	})

	if err != nil {
		t.Fatalf("NewMddbClient() error = %v", err)
	}

	httpClient, ok := client.(*Client)
	if !ok {
		t.Fatal("expected *Client for HTTP protocol")
	}
	if httpClient.apiKey != "key" {
		t.Errorf("apiKey = %v, want key", httpClient.apiKey)
	}
}

func TestNewMddbClient_DefaultHTTP(t *testing.T) {
	client, err := NewMddbClient(ClientConfig{
		URL: "http://localhost:8080",
	})

	if err != nil {
		t.Fatalf("NewMddbClient() error = %v", err)
	}

	if _, ok := client.(*Client); !ok {
		t.Fatal("expected *Client for default protocol")
	}
}

func TestNewMddbClient_GRPC(t *testing.T) {
	client, err := NewMddbClient(ClientConfig{
		URL:       "localhost:11024",
		Protocol:  "grpc",
		APIKey:    "key",
		Timeout:   10,
		BatchSize: 200,
	})

	if err != nil {
		t.Fatalf("NewMddbClient() error = %v", err)
	}

	grpcClient, ok := client.(*GRPCClient)
	if !ok {
		t.Fatal("expected *GRPCClient for gRPC protocol")
	}
	defer func() { _ = grpcClient.Close() }()

	if grpcClient.apiKey != "key" {
		t.Errorf("apiKey = %v, want key", grpcClient.apiKey)
	}
}

func TestMddbDocument_ToDocument(t *testing.T) {
	mddbDoc := mddbDocument{
		ID:        "doc|blog|test|en_US",
		Key:       "test",
		Lang:      "en_US",
		ContentMd: "# Hello World",
		Meta: map[string][]any{
			"title":    {"Test Title"},
			"tags":     {"go", "markdown"},
			"category": {"blog"},
		},
		AddedAt:   1704844800,
		UpdatedAt: 1704931200,
	}

	doc := mddbDoc.toDocument("blog")

	if doc.ID != "doc|blog|test|en_US" {
		t.Errorf("doc.ID = %v, want doc|blog|test|en_US", doc.ID)
	}

	if doc.Key != "test" {
		t.Errorf("doc.Key = %v, want test", doc.Key)
	}

	if doc.Content != "# Hello World" {
		t.Errorf("doc.Content = %v, want # Hello World", doc.Content)
	}

	if doc.Metadata["title"] != "Test Title" {
		t.Errorf("doc.Metadata[title] = %v, want Test Title", doc.Metadata["title"])
	}

	// Multi-value field should remain as array
	tags, ok := doc.Metadata["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Errorf("doc.Metadata[tags] = %v, want [go markdown]", doc.Metadata["tags"])
	}

	if doc.CreatedAt.Unix() != 1704844800 {
		t.Errorf("doc.CreatedAt.Unix() = %v, want 1704844800", doc.CreatedAt.Unix())
	}

	if doc.UpdatedAt.Unix() != 1704931200 {
		t.Errorf("doc.UpdatedAt.Unix() = %v, want 1704931200", doc.UpdatedAt.Unix())
	}
}
