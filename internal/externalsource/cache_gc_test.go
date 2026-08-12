package externalsource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestGCExpired: only entries whose freshness AND stale windows both passed are
// reclaimed; fresh and stale-usable entries stay (GO-091).
func TestGCExpired(t *testing.T) {
	dir := t.TempDir()
	c := diskCache{dir: dir}
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	put := func(key string, expires, staleUntil time.Time) {
		t.Helper()
		if err := c.put(key, []byte("body-"+key), cacheMeta{
			Source: key, FetchedAt: now.Add(-2 * time.Hour),
			ExpiresAt: expires, StaleUntil: staleUntil,
		}); err != nil {
			t.Fatal(err)
		}
	}
	put("fresh", now.Add(time.Hour), now.Add(2*time.Hour))
	put("stale-usable", now.Add(-time.Hour), now.Add(time.Hour))
	put("dead", now.Add(-2*time.Hour), now.Add(-time.Hour))

	// Dry run: one entry (body+meta), nothing deleted.
	files, bytes, err := GCExpired(dir, now, true)
	if err != nil || files != 1 || bytes == 0 {
		t.Fatalf("dry = %d files %d bytes %v", files, bytes, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dead"+bodySuffix)); err != nil {
		t.Fatal("dry run must not delete")
	}

	// Real run removes exactly the dead entry.
	if _, _, err := GCExpired(dir, now, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dead"+bodySuffix)); err == nil {
		t.Fatal("dead entry should be evicted")
	}
	for _, key := range []string{"fresh", "stale-usable"} {
		if _, _, ok := c.get(key); !ok {
			t.Fatalf("entry %q should survive GC", key)
		}
	}

	// Missing dir → zero no-op; corrupted meta is skipped (get's job).
	if f, b, err := GCExpired(filepath.Join(dir, "nope"), now, false); err != nil || f != 0 || b != 0 {
		t.Fatalf("missing dir = %d %d %v", f, b, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad"+metaSuffix), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if f, _, err := GCExpired(dir, now, false); err != nil || f != 0 {
		t.Fatalf("corrupted meta must be skipped: %d %v", f, err)
	}
}

// TestSecureTransport: the exported hardened transport refuses loopback unless
// allowPrivate — the same SSRF guard the internal client uses.
func TestSecureTransport(t *testing.T) {
	tr := SecureTransport(false)
	if tr == nil || tr.DialContext == nil || !tr.DisableKeepAlives {
		t.Fatal("transport not hardened")
	}
	if _, err := tr.DialContext(t.Context(), "tcp", "127.0.0.1:80"); err == nil {
		t.Fatal("loopback must be refused without allowPrivate")
	}
	trPriv := SecureTransport(true)
	// With allowPrivate the dial may still fail (nothing listening) — but not
	// with the SSRF refusal; a refused-connection error is acceptable here.
	if _, err := trPriv.DialContext(t.Context(), "tcp", "127.0.0.1:1"); err != nil &&
		strings.Contains(err.Error(), "private") {
		t.Fatalf("allowPrivate must not SSRF-refuse: %v", err)
	}
}
