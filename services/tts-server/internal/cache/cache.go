// Package cache is a size-bounded LRU for rendered audio, kept in memory or
// on disk. Keys are hex SHA-256 digests (see Key), which are also safe file
// names.
package cache

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
)

// backend stores the bytes; the Cache keeps the LRU order and sizes.
type backend interface {
	get(key string) ([]byte, error)
	put(key string, b []byte) error
	del(key string)
}

// Cache is an LRU bounded by total bytes. A nil *Cache is a disabled cache.
type Cache struct {
	mu    sync.Mutex
	max   int64
	used  int64
	order *list.List // front = most recently used; values are *entry
	items map[string]*list.Element
	store backend
}

type entry struct {
	key  string
	size int64
}

// Key hashes the parts that determine a rendering into a cache key.
func Key(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func newCache(maxBytes int64, store backend) *Cache {
	return &Cache{max: maxBytes, order: list.New(), items: map[string]*list.Element{}, store: store}
}

// Get returns the cached bytes for key and marks them recently used.
func (c *Cache) Get(key string) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	b, err := c.store.get(key)
	if err != nil {
		c.removeLocked(el)
		return nil, false
	}
	c.order.MoveToFront(el)
	return b, true
}

// Put stores b under key, evicting least recently used entries to fit.
// Values larger than the whole budget are not cached.
func (c *Cache) Put(key string, b []byte) {
	size := int64(len(b))
	if c == nil || size > c.max {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeLocked(el)
	}
	if c.store.put(key, b) != nil {
		return
	}
	c.addLocked(key, size)
}

// addLocked records an entry already present in the backend.
func (c *Cache) addLocked(key string, size int64) {
	c.items[key] = c.order.PushFront(&entry{key: key, size: size})
	c.used += size
	for c.used > c.max {
		c.removeLocked(c.order.Back())
	}
}

func (c *Cache) removeLocked(el *list.Element) {
	e := c.order.Remove(el).(*entry)
	delete(c.items, e.key)
	c.used -= e.size
	c.store.del(e.key)
}

// Len is the number of cached entries.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// memory keeps values in a map.
type memory map[string][]byte

func (m memory) get(key string) ([]byte, error) { return m[key], nil }
func (m memory) put(key string, b []byte) error { m[key] = b; return nil }
func (m memory) del(key string)                 { delete(m, key) }

// NewMemory returns an in-memory cache of maxBytes, or nil when maxBytes <= 0.
func NewMemory(maxBytes int64) *Cache {
	if maxBytes <= 0 {
		return nil
	}
	return newCache(maxBytes, memory{})
}
