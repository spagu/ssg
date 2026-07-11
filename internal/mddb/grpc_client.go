// Package mddb provides a client for the MDDB markdown database
package mddb

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	pb "github.com/spagu/ssg/internal/mddb/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// GRPCClient is the mddb gRPC client
type GRPCClient struct {
	conn      *grpc.ClientConn
	client    pb.MDDBClient
	apiKey    string
	batchSize int
	timeout   time.Duration
}

// GRPCConfig holds mddb gRPC client configuration
type GRPCConfig struct {
	Address   string // gRPC server address (e.g., "localhost:11024")
	APIKey    string // Optional API key for authentication
	Timeout   int    // Timeout in seconds (default: 30)
	BatchSize int    // Batch size for pagination (default: 1000)
}

// NewGRPCClient creates a new mddb gRPC client
func NewGRPCClient(cfg GRPCConfig) (*GRPCClient, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	// Resolve transport security from the address scheme (SEC-004):
	//   grpcs:// or https:// → TLS; grpc:// or http:// → explicit insecure;
	//   no scheme → infer (loopback = insecure, otherwise TLS).
	address, useTLS := resolveGRPCTransport(cfg.Address)

	// Never leak a Bearer API key over an unencrypted channel to a remote host.
	if cfg.APIKey != "" && !useTLS && !isLoopbackAddr(address) {
		return nil, fmt.Errorf(
			"refusing to send API key over insecure gRPC to non-loopback host %q; use grpcs:// or a loopback address",
			address)
	}

	var creds credentials.TransportCredentials
	if useTLS {
		creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		creds = insecure.NewCredentials()
	}

	// Connect to gRPC server
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("connecting to gRPC server: %w", err)
	}

	return &GRPCClient{
		conn:      conn,
		client:    pb.NewMDDBClient(conn),
		apiKey:    cfg.APIKey,
		batchSize: batchSize,
		timeout:   time.Duration(timeout) * time.Second,
	}, nil
}

// resolveGRPCTransport strips the scheme from a gRPC address and reports whether
// TLS must be used (SEC-004). Explicit grpcs://https:// force TLS; explicit
// grpc://http:// force insecure; a bare host is inferred (loopback = insecure,
// otherwise TLS-by-default for safety).
func resolveGRPCTransport(address string) (host string, useTLS bool) {
	switch {
	case strings.HasPrefix(address, "grpcs://"):
		return strings.TrimPrefix(address, "grpcs://"), true
	case strings.HasPrefix(address, "https://"):
		return strings.TrimPrefix(address, "https://"), true
	case strings.HasPrefix(address, "grpc://"):
		return strings.TrimPrefix(address, "grpc://"), false
	case strings.HasPrefix(address, "http://"):
		return strings.TrimPrefix(address, "http://"), false
	default:
		return address, !isLoopbackAddr(address)
	}
}

// isLoopbackAddr reports whether a host[:port] address targets a loopback host.
func isLoopbackAddr(address string) bool {
	host := address
	if h, _, err := net.SplitHostPort(address); err == nil {
		host = h
	}
	if host == "localhost" || host == "" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// Close closes the gRPC connection
func (c *GRPCClient) Close() error {
	return c.conn.Close()
}

// contextWithAuth creates a context with auth metadata and timeout
func (c *GRPCClient) contextWithAuth() (context.Context, context.CancelFunc) {
	// #nosec G118 -- cancel is returned to the caller, which defers it (known FP)
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)

	if c.apiKey != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.apiKey)
	}

	return ctx, cancel
}

// protoMetaToMetadata converts proto MetaValues map to map[string]any
func protoMetaToMetadata(protoMeta map[string]*pb.MetaValues) map[string]any {
	metadata := make(map[string]any)
	for k, v := range protoMeta {
		if v != nil && len(v.Values) > 0 {
			if len(v.Values) == 1 {
				metadata[k] = v.Values[0]
			} else {
				// Convert []string to []any
				values := make([]any, len(v.Values))
				for i, val := range v.Values {
					values[i] = val
				}
				metadata[k] = values
			}
		}
	}
	return metadata
}

// protoDocToDocument converts proto Document to Document
func protoDocToDocument(doc *pb.Document, collection string) Document {
	return Document{
		ID:         doc.Id,
		Key:        doc.Key,
		Collection: collection,
		Lang:       doc.Lang,
		Content:    doc.ContentMd,
		Metadata:   protoMetaToMetadata(doc.Meta),
		CreatedAt:  unixToTime(doc.AddedAt),
		UpdatedAt:  unixToTime(doc.UpdatedAt),
	}
}

// metadataToProtoMeta converts map[string][]any to proto MetaValues
func metadataToProtoMeta(meta map[string][]any) map[string]*pb.MetaValues {
	protoMeta := make(map[string]*pb.MetaValues)
	for k, v := range meta {
		values := make([]string, len(v))
		for i, val := range v {
			values[i] = fmt.Sprintf("%v", val)
		}
		protoMeta[k] = &pb.MetaValues{Values: values}
	}
	return protoMeta
}

// Get fetches a single document by collection and key
func (c *GRPCClient) Get(req GetRequest) (*Document, error) {
	ctx, cancel := c.contextWithAuth()
	defer cancel()

	protoReq := &pb.GetRequest{
		Collection: req.Collection,
		Key:        req.Key,
		Lang:       req.Lang,
	}

	if req.Env != nil {
		protoReq.Env = req.Env
	}

	resp, err := c.client.Get(ctx, protoReq)
	if err != nil {
		return nil, fmt.Errorf("gRPC Get: %w", err)
	}

	doc := protoDocToDocument(resp, req.Collection)
	return &doc, nil
}

// Search fetches multiple documents matching filters
func (c *GRPCClient) Search(req SearchRequest) ([]Document, int, error) {
	ctx, cancel := c.contextWithAuth()
	defer cancel()

	// Bounds check for int32 conversion (max ~2 billion)
	limit := req.Limit
	if limit > 2147483647 {
		limit = 2147483647
	}
	offset := req.Offset
	if offset > 2147483647 {
		offset = 2147483647
	}

	// Note: the proto SearchRequest message has no lang field, so req.Lang
	// cannot be sent over the wire; GetAll/GetByType apply the language
	// filter client-side via filterDocsByLang instead (GO-013).
	protoReq := &pb.SearchRequest{
		Collection: req.Collection,
		Sort:       req.Sort,
		Asc:        req.Asc,
		Limit:      int32(limit),  // #nosec G115 -- bounds checked above
		Offset:     int32(offset), // #nosec G115 -- bounds checked above
	}

	if req.FilterMeta != nil {
		protoReq.FilterMeta = metadataToProtoMeta(req.FilterMeta)
	}

	resp, err := c.client.Search(ctx, protoReq)
	if err != nil {
		return nil, 0, fmt.Errorf("gRPC Search: %w", err)
	}

	docs := make([]Document, len(resp.Documents))
	for i, protoDoc := range resp.Documents {
		docs[i] = protoDocToDocument(protoDoc, req.Collection)
	}

	return docs, int(resp.Total), nil
}

// GetAll fetches all documents from a collection with pagination
func (c *GRPCClient) GetAll(collection string, lang string, batchSize int) ([]Document, error) {
	if batchSize <= 0 {
		batchSize = c.batchSize
	}

	return getAllPaginated(c.Search, collection, lang, nil, batchSize)
}

// GetByType fetches all documents filtered by type (page or post) with pagination
func (c *GRPCClient) GetByType(collection string, docType string, lang string) ([]Document, error) {
	return getAllPaginated(c.Search, collection, lang,
		map[string][]any{"type": {docType}}, c.batchSize)
}

// Health checks if the mddb server is available using Stats RPC
func (c *GRPCClient) Health() error {
	ctx, cancel := c.contextWithAuth()
	defer cancel()

	_, err := c.client.Stats(ctx, &pb.StatsRequest{})
	if err != nil {
		return fmt.Errorf("gRPC Stats (health check): %w", err)
	}

	return nil
}

// Stats returns server statistics
func (c *GRPCClient) Stats() (*pb.StatsResponse, error) {
	ctx, cancel := c.contextWithAuth()
	defer cancel()

	resp, err := c.client.Stats(ctx, &pb.StatsRequest{})
	if err != nil {
		return nil, fmt.Errorf("gRPC Stats: %w", err)
	}

	return resp, nil
}

// Checksum returns the checksum for a collection (for change detection)
// Note: gRPC API doesn't have a direct Checksum RPC, so we use Stats and derive from collection info
func (c *GRPCClient) Checksum(collection string) (*ChecksumResponse, error) {
	stats, err := c.Stats()
	if err != nil {
		return nil, fmt.Errorf("getting stats for checksum: %w", err)
	}

	// Find the collection in stats
	for _, col := range stats.Collections {
		if col.Name == collection {
			// Generate a pseudo-checksum from document count + revision count
			// This is a workaround since gRPC doesn't expose the same checksum endpoint
			checksum := fmt.Sprintf("%d-%d", col.DocumentCount, col.RevisionCount)
			return &ChecksumResponse{
				Collection:    collection,
				Checksum:      checksum,
				DocumentCount: int(col.DocumentCount),
			}, nil
		}
	}

	return nil, fmt.Errorf("collection %s not found", collection)
}
