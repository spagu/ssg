package mddb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

		resp := GetResponse{
			Success: true,
			Document: Document{
				Key:        req.Key,
				Collection: req.Collection,
				Content:    "# Test Content",
				Metadata: map[string]any{
					"title": "Test Title",
					"type":  "post",
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
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
}

func TestClient_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/search" {
			t.Errorf("Expected /v1/search, got %s", r.URL.Path)
		}

		resp := SearchResponse{
			Success: true,
			Total:   2,
			Documents: []Document{
				{
					Key:        "post-1",
					Collection: "blog",
					Content:    "# Post 1",
					Metadata:   map[string]any{"title": "Post 1"},
				},
				{
					Key:        "post-2",
					Collection: "blog",
					Content:    "# Post 2",
					Metadata:   map[string]any{"title": "Post 2"},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
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

		var docs []Document
		if req.Offset == 0 {
			docs = []Document{
				{Key: "post-1", Collection: "blog"},
				{Key: "post-2", Collection: "blog"},
			}
		}

		resp := SearchResponse{
			Success:   true,
			Total:     2,
			Documents: docs,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
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
			w.WriteHeader(http.StatusOK)
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

		if req.Filters["type"] != "post" {
			t.Errorf("Expected type filter 'post', got %v", req.Filters["type"])
		}

		resp := SearchResponse{
			Success: true,
			Total:   1,
			Documents: []Document{
				{Key: "post-1", Collection: "blog", Metadata: map[string]any{"type": "post"}},
			},
		}

		w.Header().Set("Content-Type", "application/json")
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

		resp := GetResponse{
			Success:  true,
			Document: Document{Key: "test"},
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
	t.Run("server error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := GetResponse{
				Success: false,
				Error:   "document not found",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
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
