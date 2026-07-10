package mddb

import "testing"

// TestEnsureSecureForAPIKey covers SEC-007: refuse to send a Bearer key over
// plaintext http:// to a non-loopback host.
func TestEnsureSecureForAPIKey(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"https://mddb.example.com", false},
		{"http://localhost:11023", false},
		{"http://127.0.0.1:11023", false},
		{"http://[::1]:11023", false},
		{"http://mddb.example.com", true},
		{"http://10.0.0.5:11023", true},
	}
	for _, tt := range tests {
		err := ensureSecureForAPIKey(tt.url)
		if (err != nil) != tt.wantErr {
			t.Errorf("ensureSecureForAPIKey(%q) err=%v, wantErr=%v", tt.url, err, tt.wantErr)
		}
	}
}

// TestDoRequestRefusesPlaintextKey exercises the guard through the client: a
// non-loopback http:// URL with an API key must fail before sending.
func TestDoRequestRefusesPlaintextKey(t *testing.T) {
	c := NewClient(Config{BaseURL: "http://mddb.example.com", APIKey: "secret"})
	_, err := c.doRequest("GET", "/v1/checksum", nil)
	if err == nil {
		t.Fatalf("expected refusal sending API key over plaintext http:// to remote host")
	}
}

// TestResolveGRPCTransport covers SEC-004 scheme → TLS resolution.
func TestResolveGRPCTransport(t *testing.T) {
	tests := []struct {
		addr     string
		wantHost string
		wantTLS  bool
	}{
		{"grpcs://mddb.example.com:443", "mddb.example.com:443", true},
		{"https://mddb.example.com:443", "mddb.example.com:443", true},
		{"grpc://localhost:11024", "localhost:11024", false},
		{"http://localhost:11024", "localhost:11024", false},
		{"localhost:11024", "localhost:11024", false},              // bare loopback → insecure
		{"mddb.example.com:11024", "mddb.example.com:11024", true}, // bare remote → TLS
	}
	for _, tt := range tests {
		host, tls := resolveGRPCTransport(tt.addr)
		if host != tt.wantHost || tls != tt.wantTLS {
			t.Errorf("resolveGRPCTransport(%q) = (%q,%v), want (%q,%v)", tt.addr, host, tls, tt.wantHost, tt.wantTLS)
		}
	}
}

// TestIsLoopbackAddr covers loopback detection with and without ports.
func TestIsLoopbackAddr(t *testing.T) {
	loopback := []string{"localhost:1", "127.0.0.1:1", "127.0.0.1", "[::1]:1", "localhost"}
	remote := []string{"example.com:1", "10.0.0.1:1", "8.8.8.8"}
	for _, a := range loopback {
		if !isLoopbackAddr(a) {
			t.Errorf("isLoopbackAddr(%q) = false, want true", a)
		}
	}
	for _, a := range remote {
		if isLoopbackAddr(a) {
			t.Errorf("isLoopbackAddr(%q) = true, want false", a)
		}
	}
}

// TestNewGRPCClientRefusesInsecureKey covers SEC-004: an API key over an explicit
// insecure grpc:// channel to a remote host is refused at construction.
func TestNewGRPCClientRefusesInsecureKey(t *testing.T) {
	_, err := NewGRPCClient(GRPCConfig{Address: "grpc://mddb.example.com:11024", APIKey: "secret"})
	if err == nil {
		t.Fatalf("expected refusal of API key over insecure gRPC to remote host")
	}
	// Loopback insecure with a key is allowed.
	c, err := NewGRPCClient(GRPCConfig{Address: "grpc://localhost:11024", APIKey: "secret"})
	if err != nil {
		t.Fatalf("unexpected error for loopback insecure client: %v", err)
	}
	_ = c.Close()
}
