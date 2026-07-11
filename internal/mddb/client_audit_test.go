// Package mddb - regression tests for the audited client defects:
// GO-013 (lang filter no-op), GO-015 (pagination truncation without
// X-Total-Count), GO-031 (epoch/timezone dates) and GO-041 (checksum query
// escaping, offset advance under server page-size clamping).
package mddb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	pb "github.com/spagu/ssg/internal/mddb/proto"
)

// captureStderr redirects os.Stderr for the duration of fn and returns what
// was written (used to assert the GO-015 malformed-header warning).
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// batchingHandler serves slices of keys honoring Offset/Limit (optionally
// clamped) and an optional X-Total-Count header value ("" = omit header).
func batchingHandler(t *testing.T, keys []string, clamp int, totalHeader string, offsets *[]int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		var req SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding search request: %v", err)
		}
		if offsets != nil {
			*offsets = append(*offsets, req.Offset)
		}

		limit := req.Limit
		if clamp > 0 && limit > clamp {
			limit = clamp
		}

		var docs []mddbDocument
		for i := req.Offset; i < len(keys) && i < req.Offset+limit; i++ {
			docs = append(docs, mddbDocument{Key: keys[i], Lang: "en_US"})
		}

		w.Header().Set("Content-Type", "application/json")
		if totalHeader != "" {
			w.Header().Set("X-Total-Count", totalHeader)
		}
		_ = json.NewEncoder(w).Encode(docs)
	}
}

// --- GO-013: lang filter propagation and enforcement ---

func TestClient_GetAll_LangPropagatedToRequestBody(t *testing.T) {
	var rawBodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var raw map[string]any
		_ = json.Unmarshal(body, &raw)
		rawBodies = append(rawBodies, raw)

		// The mock ignores the lang filter and returns mixed languages —
		// the client-side safety net must still yield a single language.
		docs := []mddbDocument{
			{Key: "en-post", Lang: "en_US"},
			{Key: "pl-post", Lang: "pl_PL"},
			{Key: "meta-doc", Lang: ""}, // lang-less docs must survive the filter
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "3")
		_ = json.NewEncoder(w).Encode(docs)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	docs, err := client.GetAll("blog", "en_US", 10)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}

	if len(rawBodies) == 0 {
		t.Fatal("no search request captured")
	}
	if got := rawBodies[0]["lang"]; got != "en_US" {
		t.Errorf(`request body lang = %v, want "en_US" (GO-013)`, got)
	}

	if len(docs) != 2 || docs[0].Key != "en-post" || docs[1].Key != "meta-doc" {
		keys := make([]string, len(docs))
		for i, d := range docs {
			keys[i] = d.Key
		}
		t.Errorf("GetAll docs = %v, want [en-post meta-doc]", keys)
	}
}

func TestClient_GetByType_LangPropagatedToRequestBody(t *testing.T) {
	var captured []SearchRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SearchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		captured = append(captured, req)

		docs := []mddbDocument{{Key: "en-post", Lang: "en_US"}}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total-Count", "1")
		_ = json.NewEncoder(w).Encode(docs)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	if _, err := client.GetByType("blog", "post", "en_US"); err != nil {
		t.Fatalf("GetByType() error = %v", err)
	}

	if len(captured) == 0 {
		t.Fatal("no search request captured")
	}
	if captured[0].Lang != "en_US" {
		t.Errorf(`SearchRequest.Lang = %q, want "en_US" (GO-013)`, captured[0].Lang)
	}
	if tf := captured[0].FilterMeta["type"]; len(tf) != 1 || tf[0] != "post" {
		t.Errorf("SearchRequest.FilterMeta[type] = %v, want [post]", tf)
	}
}

func TestGRPCClient_GetAll_LangFilteredClientSide(t *testing.T) {
	// The proto SearchRequest has no lang field, so the gRPC path must drop
	// foreign-language documents client-side (GO-013).
	srv := &mockMDDBServer{
		searchFunc: func(_ context.Context, _ *pb.SearchRequest) (*pb.SearchResponse, error) {
			return &pb.SearchResponse{
				Documents: []*pb.Document{
					{Key: "en-post", Lang: "en_US"},
					{Key: "pl-post", Lang: "pl_PL"},
					{Key: "meta-doc", Lang: ""}, // lang-less docs must survive the filter
				},
				Total: 3,
			}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	docs, err := grpcClient.GetAll("blog", "en_US", 10)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if len(docs) != 2 || docs[0].Key != "en-post" || docs[1].Key != "meta-doc" {
		keys := make([]string, len(docs))
		for i, d := range docs {
			keys[i] = d.Key
		}
		t.Errorf("GetAll docs = %v, want [en-post meta-doc] (GO-013)", keys)
	}
}

func TestGRPCClient_GetByType_LangFilteredClientSide(t *testing.T) {
	srv := &mockMDDBServer{
		searchFunc: func(_ context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
			if req.FilterMeta == nil {
				t.Error("expected filterMeta to be set")
			}
			return &pb.SearchResponse{
				Documents: []*pb.Document{
					{Key: "en-post", Lang: "en_US"},
					{Key: "pl-post", Lang: "pl_PL"},
				},
				Total: 2,
			}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	docs, err := grpcClient.GetByType("blog", "post", "en_US")
	if err != nil {
		t.Fatalf("GetByType() error = %v", err)
	}
	if len(docs) != 1 || docs[0].Key != "en-post" {
		t.Errorf("GetByType docs = %v, want only en-post (GO-013)", docs)
	}
}

// --- GO-015: pagination without a trustworthy X-Total-Count ---

func TestClient_GetAll_NoTotalHeader_ThreeFullBatches(t *testing.T) {
	keys := []string{"p1", "p2", "p3", "p4", "p5", "p6"}
	var offsets []int
	server := httptest.NewServer(batchingHandler(t, keys, 0, "", &offsets))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	docs, err := client.GetAll("blog", "", 2)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}

	if len(docs) != len(keys) {
		t.Errorf("len(docs) = %v, want %v (all batches without header, GO-015)", len(docs), len(keys))
	}
	// 3 full batches + 1 empty terminator, offsets advanced by batch length
	want := []int{0, 2, 4, 6}
	if fmt.Sprint(offsets) != fmt.Sprint(want) {
		t.Errorf("offsets = %v, want %v", offsets, want)
	}
}

func TestClient_GetAll_MalformedTotalHeader(t *testing.T) {
	for _, malformed := range []string{"abc", "-5"} {
		t.Run(malformed, func(t *testing.T) {
			keys := []string{"p1", "p2", "p3"}
			server := httptest.NewServer(batchingHandler(t, keys, 0, malformed, nil))
			defer server.Close()

			client := NewClient(Config{BaseURL: server.URL})

			var docs []Document
			var err error
			stderr := captureStderr(t, func() {
				docs, err = client.GetAll("blog", "", 2)
			})

			if err != nil {
				t.Fatalf("GetAll() error = %v", err)
			}
			if len(docs) != len(keys) {
				t.Errorf("len(docs) = %v, want %v (malformed header must not truncate, GO-015)", len(docs), len(keys))
			}
			if !strings.Contains(stderr, "X-Total-Count") {
				t.Errorf("stderr = %q, want a malformed X-Total-Count warning", stderr)
			}
		})
	}
}

// --- GO-041: server clamping the page size below the requested batch ---

func TestClient_GetAll_ServerClampsPageSize(t *testing.T) {
	const total = 120
	const clamp = 50
	keys := make([]string, total)
	for i := range keys {
		keys[i] = fmt.Sprintf("post-%d", i)
	}

	var offsets []int
	server := httptest.NewServer(batchingHandler(t, keys, clamp, strconv.Itoa(total), &offsets))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, BatchSize: 1000})
	docs, err := client.GetAll("blog", "", 0)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}

	if len(docs) != total {
		t.Errorf("len(docs) = %v, want %v (clamped pages must not truncate, GO-041)", len(docs), total)
	}
	// Offset must advance by the received batch length, not the requested one,
	// and the trusted total avoids a trailing empty request.
	want := []int{0, 50, 100}
	if fmt.Sprint(offsets) != fmt.Sprint(want) {
		t.Errorf("offsets = %v, want %v", offsets, want)
	}
}

func TestGRPCClient_GetAll_ServerClampsPageSize(t *testing.T) {
	const total = 5
	const clamp = 2
	callCount := 0
	srv := &mockMDDBServer{
		searchFunc: func(_ context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
			callCount++
			var docs []*pb.Document
			for i := int(req.Offset); i < total && i < int(req.Offset)+clamp; i++ {
				docs = append(docs, &pb.Document{Key: fmt.Sprintf("post-%d", i)})
			}
			return &pb.SearchResponse{Documents: docs, Total: total}, nil
		},
	}

	grpcClient, cleanup := newTestGRPCClient(t, srv)
	defer cleanup()

	docs, err := grpcClient.GetAll("blog", "", 1000)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if len(docs) != total {
		t.Errorf("len(docs) = %v, want %v (clamped pages must not truncate, GO-041)", len(docs), total)
	}
	if callCount != 3 {
		t.Errorf("callCount = %v, want 3", callCount)
	}
}

// --- GO-041: checksum query escaping ---

func TestClient_Checksum_EscapesCollectionName(t *testing.T) {
	const collection = "a&b c"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("collection"); got != collection {
			t.Errorf("collection query param = %q, want %q", got, collection)
		}
		if r.URL.RawQuery != "collection=a%26b+c" {
			t.Errorf("RawQuery = %q, want escaped 'collection=a%%26b+c' (GO-041)", r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ChecksumResponse{
			Collection:    collection,
			Checksum:      "abc123",
			DocumentCount: 1,
		})
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	resp, err := client.Checksum(collection)
	if err != nil {
		t.Fatalf("Checksum() error = %v", err)
	}
	if resp.Collection != collection {
		t.Errorf("resp.Collection = %q, want %q", resp.Collection, collection)
	}
}

// --- GO-031: epoch timestamps and timezone-independent dates ---

func TestUnixToTime_ZeroMeansNoDate(t *testing.T) {
	if !unixToTime(0).IsZero() {
		t.Error("unixToTime(0) must be the zero time.Time, not 1970-01-01 (GO-031)")
	}
	if unixToTime(1).IsZero() {
		t.Error("unixToTime(1) must not be zero")
	}
}

func TestMddbDocument_ToDocument_ZeroAddedAt(t *testing.T) {
	doc := (&mddbDocument{Key: "no-date"}).toDocument("blog")
	if !doc.CreatedAt.IsZero() {
		t.Errorf("CreatedAt = %v, want zero time for AddedAt==0 (GO-031)", doc.CreatedAt)
	}
	if !doc.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt = %v, want zero time for UpdatedAt==0 (GO-031)", doc.UpdatedAt)
	}
}

func TestProtoDocToDocument_ZeroAddedAt(t *testing.T) {
	doc := protoDocToDocument(&pb.Document{Key: "no-date"}, "blog")
	if !doc.CreatedAt.IsZero() {
		t.Errorf("CreatedAt = %v, want zero time for AddedAt==0 (GO-031)", doc.CreatedAt)
	}
	if !doc.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt = %v, want zero time for UpdatedAt==0 (GO-031)", doc.UpdatedAt)
	}
}

func TestUnixToTime_UTCDeterministic(t *testing.T) {
	const ts = 1704067200 // 2024-01-01T00:00:00Z

	got := unixToTime(ts)
	if got.Location() != time.UTC {
		t.Errorf("Location() = %v, want UTC (GO-031)", got.Location())
	}
	// The calendar date must not depend on the build machine's local zone:
	// a UTC-5 host converting 2024-01-01T00:00Z must not yield 2023-12-31.
	// (No time.Local mutation here — writing that global races with parallel tests.)
	if d := got.Format("2006/01/02"); d != "2024/01/01" {
		t.Errorf("date = %q, want 2024/01/01 regardless of host zone (GO-031)", d)
	}
	// Sanity check of the premise: the same instant rendered in UTC-5 flips the
	// date — exactly the bug unixToTime prevents by pinning UTC.
	west := time.Unix(ts, 0).In(time.FixedZone("UTC-5", -5*60*60)).Format("2006/01/02")
	if west == "2024/01/01" {
		t.Errorf("premise broken: UTC-5 rendering should flip the date, got %q", west)
	}
}
