package mddb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
