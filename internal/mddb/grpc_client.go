// Package mddb provides a client for the MDDB markdown database
package mddb

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "github.com/spagu/ssg/internal/mddb/proto"
	"google.golang.org/grpc"
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

	// Remove protocol prefix if present
	address := cfg.Address
	address = strings.TrimPrefix(address, "grpc://")
	address = strings.TrimPrefix(address, "http://")
	address = strings.TrimPrefix(address, "https://")

	// Connect to gRPC server
	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
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

// Close closes the gRPC connection
func (c *GRPCClient) Close() error {
	return c.conn.Close()
}

// contextWithAuth creates a context with auth metadata and timeout
func (c *GRPCClient) contextWithAuth() (context.Context, context.CancelFunc) {
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
		CreatedAt:  time.Unix(doc.AddedAt, 0),
		UpdatedAt:  time.Unix(doc.UpdatedAt, 0),
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

	protoReq := &pb.SearchRequest{
		Collection: req.Collection,
		Sort:       req.Sort,
		Asc:        req.Asc,
		Limit:      int32(req.Limit),
		Offset:     int32(req.Offset),
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
func (c *GRPCClient) GetByType(collection string, docType string, lang string) ([]Document, error) {
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
