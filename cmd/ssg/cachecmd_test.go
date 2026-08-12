package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func TestCacheNamespaces(t *testing.T) {
	t.Chdir(t.TempDir())
	// Defaults, no config, no legacy dir.
	ns := cacheNamespaces(nil)
	if len(ns) != 3 || ns[0].name != "images" || ns[2].dir != filepath.Join(".ssg-cache", "ai") {
		t.Fatalf("default namespaces = %+v", ns)
	}
	// Config overrides win.
	cfg := &config.Config{}
	cfg.ExternalSources.CacheDir = "custom-ext"
	cfg.AI.CacheDir = "custom-ai"
	ns = cacheNamespaces(cfg)
	if ns[1].dir != "custom-ext" || ns[2].dir != "custom-ai" {
		t.Fatalf("overrides ignored: %+v", ns)
	}
	// A leftover legacy AI root shows up as its own row.
	if err := os.Mkdir(".ai-cache", 0o755); err != nil {
		t.Fatal(err)
	}
	ns = cacheNamespaces(nil)
	if len(ns) != 4 || ns[3].name != "ai (legacy)" {
		t.Fatalf("legacy root not surfaced: %+v", ns)
	}
}

func TestFilterNamespacesAndFlags(t *testing.T) {
	all := []cacheNamespace{{"images", "a"}, {"ai", "b"}, {"ai (legacy)", "c"}}
	if got := filterNamespaces(all, ""); len(got) != 3 {
		t.Fatal("empty selector keeps all")
	}
	if got := filterNamespaces(all, "ai"); len(got) != 2 { // ai + ai (legacy)
		t.Fatalf("ai selector = %+v", got)
	}
	if got := filterNamespaces(all, "images"); len(got) != 1 || got[0].dir != "a" {
		t.Fatalf("images selector = %+v", got)
	}

	nsName, dry, ok := parseCacheFlags([]string{"--namespace=ai", "--dry"})
	if !ok || nsName != "ai" || !dry {
		t.Fatalf("flags = %q %v %v", nsName, dry, ok)
	}
	if _, _, ok := parseCacheFlags([]string{"--bogus"}); ok {
		t.Fatal("unknown flag must fail")
	}
}

func TestRunCacheEndToEnd(t *testing.T) {
	t.Chdir(t.TempDir())
	// Seed the images namespace with one entry.
	dir := filepath.Join(".ssg-cache", "images")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.abc.webp"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := runCache([]string{"stats"}); code != 0 {
		t.Fatalf("stats exit %d", code)
	}
	if code := runCache([]string{"gc", "--dry"}); code != 0 {
		t.Fatalf("gc exit %d", code)
	}
	if code := runCache([]string{"clean", "--namespace=images"}); code != 0 {
		t.Fatalf("clean exit %d", code)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("clean should remove the images namespace")
	}

	// Bad invocations.
	if code := runCache(nil); code != 2 {
		t.Fatalf("no subcommand should exit 2, got %d", code)
	}
	if code := runCache([]string{"bogus"}); code != 2 {
		t.Fatalf("unknown subcommand should exit 2, got %d", code)
	}
	if code := runCache([]string{"clean", "--namespace=nope"}); code != 1 {
		t.Fatalf("unmatched namespace should exit 1, got %d", code)
	}
	if code := runCache([]string{"clean", "--bogus"}); code != 2 {
		t.Fatalf("bad flag should exit 2, got %d", code)
	}
}

func TestCacheDispatch(t *testing.T) {
	t.Chdir(t.TempDir())
	// Known noun → handled; unknown noun (a source named "cache") → falls through.
	if code, handled := dispatchSubcommand([]string{"cache", "stats"}); !handled || code != 0 {
		t.Fatalf("cache stats should dispatch: %d %v", code, handled)
	}
	if _, handled := dispatchSubcommand([]string{"cache", "mytemplate", "example.com"}); handled {
		t.Fatal("a source dir named 'cache' must still build normally")
	}
}
