// Package mddb provides a client for the MDDB markdown database
package mddb

// MddbClient defines the interface for MDDB client operations
// Both HTTP and gRPC clients implement this interface
type MddbClient interface {
	// Get fetches a single document by collection and key
	Get(req GetRequest) (*Document, error)

	// Search fetches multiple documents matching filters
	Search(req SearchRequest) ([]Document, int, error)

	// GetAll fetches all documents from a collection with pagination
	GetAll(collection string, lang string, batchSize int) ([]Document, error)

	// GetByType fetches all documents filtered by type (page or post) with pagination
	GetByType(collection string, docType string, lang string) ([]Document, error)

	// Health checks if the mddb server is available
	Health() error

	// Checksum returns the checksum for a collection (for change detection)
	Checksum(collection string) (*ChecksumResponse, error)
}

// ClientConfig holds configuration for creating an MDDB client
type ClientConfig struct {
	URL       string // HTTP URL (e.g., "http://localhost:11023") or gRPC address (e.g., "localhost:11024")
	Protocol  string // "http" (default) or "grpc"
	APIKey    string // Optional API key for authentication
	Timeout   int    // Timeout in seconds (default: 30)
	BatchSize int    // Batch size for pagination (default: 1000)
}

// NewMddbClient creates a new MDDB client based on protocol configuration
func NewMddbClient(cfg ClientConfig) (MddbClient, error) {
	switch cfg.Protocol {
	case "grpc":
		return NewGRPCClient(GRPCConfig{
			Address:   cfg.URL,
			APIKey:    cfg.APIKey,
			Timeout:   cfg.Timeout,
			BatchSize: cfg.BatchSize,
		})
	default:
		// Default to HTTP
		return NewClient(Config{
			BaseURL:   cfg.URL,
			APIKey:    cfg.APIKey,
			Timeout:   cfg.Timeout,
			BatchSize: cfg.BatchSize,
		}), nil
	}
}

// Ensure both clients implement MddbClient interface
var _ MddbClient = (*Client)(nil)
var _ MddbClient = (*GRPCClient)(nil)
