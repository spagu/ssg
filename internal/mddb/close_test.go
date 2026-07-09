// Package mddb - tests for the GO-005 client Close() lifecycle.
package mddb

import "testing"

// TestClient_Close ensures the HTTP client's Close is a safe no-op and can be
// called (and re-called) without error.
func TestClient_Close(t *testing.T) {
	c := NewClient(Config{BaseURL: "http://localhost:11023"})
	if err := c.Close(); err != nil {
		t.Errorf("HTTP Client.Close() = %v, want nil", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second HTTP Client.Close() = %v, want nil", err)
	}
}

// TestNewMddbClientImplementsClose ensures every client returned by the factory
// exposes Close() via the MddbClient interface (HTTP path), so callers can defer
// Close without a type assertion (the fix that stops watch-mode leaks).
func TestNewMddbClientImplementsClose(t *testing.T) {
	client, err := NewMddbClient(ClientConfig{URL: "http://localhost:11023"})
	if err != nil {
		t.Fatalf("NewMddbClient (http) failed: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("client.Close() = %v, want nil", err)
	}
}
