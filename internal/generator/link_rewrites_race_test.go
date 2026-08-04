package generator

import (
	"strings"
	"sync"
	"testing"
)

// TestLinkRewritePrefixesUnderParallelRender covers the sibling of the alias
// race: linkRewritePrefixes memoized its sorted prefix list with an unguarded
// check-then-write, and it runs on the render pool — applyLinkRewrites is called
// from the safeHTML template helper, which executes inside ExecuteTemplate inside
// generatePage/generatePost.
//
// Concurrent workers hitting an unwarmed memo raced on the slice header, so a
// page could rewrite links against a torn or empty prefix list. Whether it bit
// depended on whether an earlier sequential render happened to warm the memo,
// which is theme-dependent — the same intermittent masking that hid the alias bug.
func TestLinkRewritePrefixesUnderParallelRender(t *testing.T) {
	g := newTestGen(t, "")
	g.config.LinkRewrites = map[string]string{
		"/docs/":            "/documentation/",
		"/docs/old/":        "/documentation/legacy/",
		"/a/":               "/b/",
		"https://old.test/": "https://new.test/",
	}

	var wg sync.WaitGroup
	results := make([][]string, 64)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got := g.linkRewritePrefixes()
			cp := make([]string, len(got))
			copy(cp, got)
			results[i] = cp
		}(i)
	}
	wg.Wait()

	// Every goroutine must see the same complete, correctly ordered list.
	want := len(g.config.LinkRewrites)
	for i, got := range results {
		if len(got) != want {
			t.Fatalf("goroutine %d saw %d prefixes, want %d: %v", i, len(got), want, got)
		}
		for j := 1; j < len(got); j++ {
			if len(got[j-1]) < len(got[j]) {
				t.Fatalf("goroutine %d saw prefixes out of longest-first order: %v", i, got)
			}
		}
	}
}

// TestApplyLinkRewritesConcurrent drives the rewrite itself concurrently and
// checks every worker produces the correct, most-specific rewrite.
func TestApplyLinkRewritesConcurrent(t *testing.T) {
	g := newTestGen(t, "")
	g.config.LinkRewrites = map[string]string{
		"/docs/":     "/documentation/",
		"/docs/old/": "/documentation/legacy/",
	}

	const in = `<a href="/docs/old/guide">a</a><a href="/docs/guide">b</a>`
	const want = `<a href="/documentation/legacy/guide">a</a><a href="/documentation/guide">b</a>`

	var wg sync.WaitGroup
	bad := make(chan string, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := g.applyLinkRewrites(in); got != want {
				bad <- got
			}
		}()
	}
	wg.Wait()
	close(bad)
	if got, ok := <-bad; ok {
		t.Errorf("concurrent rewrite produced %q, want %q", got, want)
	}
	// The longer prefix must win regardless of map iteration order.
	if !strings.Contains(g.applyLinkRewrites(in), "/documentation/legacy/guide") {
		t.Error("most-specific prefix must win")
	}
}
