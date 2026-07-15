package externalsource

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// The shared disk cache (plan §Cache): every entry is a <hash>.body payload
// plus a <hash>.meta.json descriptor under cache_dir. One cache key per source
// configuration; checksum mismatches are treated as corruption and evicted.

// DefaultCacheDir is used when cache_dir is not configured.
const DefaultCacheDir = ".ssg-cache/external-sources"

// Cache entry file suffixes (S1192).
const (
	bodySuffix = ".body"
	metaSuffix = ".meta.json"
)

// sha256Hex is the shared payload checksum used by connectors and the cache.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// cacheMeta is the on-disk descriptor of one cached payload.
type cacheMeta struct {
	Source      string    `json:"source"`
	Type        string    `json:"type"`
	FetchedAt   time.Time `json:"fetched_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	StaleUntil  time.Time `json:"stale_until"`
	Checksum    string    `json:"checksum"`
	ContentType string    `json:"content_type"`
}

// diskCache stores payloads under a directory; a zero dir disables persistence.
type diskCache struct{ dir string }

// cacheKey fingerprints everything that can change a source's payload.
func cacheKey(src Source) string {
	h := sha256.New()
	write := func(parts ...string) {
		for _, p := range parts {
			h.Write([]byte(p))
			h.Write([]byte{0})
		}
	}
	write(src.Name, src.Type, src.Format, src.URL, src.Path, src.Transform.Select,
		src.Auth.Type, src.Auth.Token, src.Auth.Username, src.Auth.Password, src.Auth.Header, src.Auth.Value)
	for _, m := range []map[string]string{src.Headers, src.Query} {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			write(k, m[k])
		}
	}
	// Paginated sources (GO-062) cache one aggregated payload per source; the
	// pagination settings are part of the fingerprint so changing them forces
	// a re-fetch. Written only when configured, so pre-pagination cache
	// entries of plain sources stay valid.
	if p := src.Pagination; p.Mode != "" {
		write(p.Mode, p.Param, p.PerPageParam,
			strconv.Itoa(p.StartPage), strconv.Itoa(p.PerPage), strconv.Itoa(p.MaxPages))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// get loads a cached payload, verifying integrity. Corrupted entries are
// evicted and reported as a miss.
func (c diskCache) get(key string) ([]byte, cacheMeta, bool) {
	if c.dir == "" {
		return nil, cacheMeta{}, false
	}
	metaRaw, err := os.ReadFile(filepath.Join(c.dir, key+metaSuffix)) // #nosec G304 -- key is a local hash under our cache dir
	if err != nil {
		return nil, cacheMeta{}, false
	}
	var meta cacheMeta
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		c.evict(key)
		return nil, cacheMeta{}, false
	}
	body, err := os.ReadFile(filepath.Join(c.dir, key+bodySuffix)) // #nosec G304 -- key is a local hash under our cache dir
	if err != nil {
		return nil, cacheMeta{}, false
	}
	if sha256Hex(body) != meta.Checksum {
		c.evict(key)
		return nil, cacheMeta{}, false
	}
	return body, meta, true
}

// put persists a payload and its descriptor.
func (c diskCache) put(key string, body []byte, meta cacheMeta) error {
	if c.dir == "" {
		return nil
	}
	if err := os.MkdirAll(c.dir, 0o750); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}
	meta.Checksum = sha256Hex(body)
	metaRaw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.dir, key+bodySuffix), body, 0o600); err != nil {
		return fmt.Errorf("writing cache body: %w", err)
	}
	if err := os.WriteFile(filepath.Join(c.dir, key+metaSuffix), metaRaw, 0o600); err != nil {
		return fmt.Errorf("writing cache metadata: %w", err)
	}
	return nil
}

// evict removes a (possibly corrupted) entry.
func (c diskCache) evict(key string) {
	_ = os.Remove(filepath.Join(c.dir, key+bodySuffix))
	_ = os.Remove(filepath.Join(c.dir, key+metaSuffix))
}

// ClearCache removes the entire external-source cache directory
// (--clear-external-cache).
func ClearCache(dir string) error {
	if dir == "" {
		dir = DefaultCacheDir
	}
	return os.RemoveAll(dir)
}
