package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKey(t *testing.T) {
	if Key("a", "b") == Key("a", "c") || len(Key("x")) != 64 {
		t.Fatal("Key")
	}
}

func TestNilCache(t *testing.T) {
	var c *Cache
	c.Put("k", []byte("v"))
	if _, ok := c.Get("k"); ok || c.Len() != 0 || NewMemory(0) != nil {
		t.Fatal("nil cache must be inert")
	}
}

func TestMemoryLRU(t *testing.T) {
	c := NewMemory(10)
	c.Put("a", []byte("1234"))
	c.Put("b", []byte("1234"))
	if _, ok := c.Get("a"); !ok { // a becomes most recent
		t.Fatal("miss a")
	}
	c.Put("c", []byte("1234")) // evicts b
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should be evicted")
	}
	c.Put("a", []byte("12")) // replace
	c.Put("huge", make([]byte, 11))
	if b, ok := c.Get("a"); !ok || string(b) != "12" || c.Len() != 2 {
		t.Fatal("replace", c.Len())
	}
	if _, ok := c.Get("huge"); ok {
		t.Fatal("oversized value cached")
	}
}

func key(n byte) string { return Key(string(n)) }

func TestDisk(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, key(1))
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(old, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	_ = os.WriteFile(filepath.Join(dir, key(2)), []byte("new"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "junk"), []byte("x"), 0o600)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	c, err := NewDisk(root, 7)
	if err != nil || c.Len() != 2 {
		t.Fatal(err, c.Len())
	}
	c.Put(key(3), []byte("abc")) // 9 bytes > 7: evicts the oldest (key 1)
	if _, ok := c.Get(key(1)); ok {
		t.Fatal("oldest should be evicted")
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("evicted file still on disk")
	}
	if b, ok := c.Get(key(3)); !ok || string(b) != "abc" {
		t.Fatal("get 3")
	}
	_ = os.Remove(filepath.Join(dir, key(2)))
	if _, ok := c.Get(key(2)); ok || c.Len() != 1 {
		t.Fatal("vanished file must be dropped")
	}
	if nilCache, err := NewDisk(root, 0); nilCache != nil || err != nil {
		t.Fatal("disabled disk cache")
	}
}

func TestDiskErrors(t *testing.T) {
	dir := t.TempDir()
	root, _ := os.OpenRoot(dir)
	c, _ := NewDisk(root, 100)
	_ = os.Mkdir(filepath.Join(dir, key(9)+".tmp"), 0o700) // temp path blocked
	c.Put(key(9), []byte("x"))
	if c.Len() != 0 {
		t.Fatal("failed put must not be indexed")
	}
	_ = root.Close()
	if _, err := NewDisk(root, 10); err == nil {
		t.Fatal("closed root must fail")
	}
}
