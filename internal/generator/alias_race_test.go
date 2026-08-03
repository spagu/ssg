package generator

import (
	"fmt"
	"sync"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestAliasRedirectsUnderParallelRender reproduces the data race that shipped
// with parallel rendering: writeAliasStubs appends to the shared aliasRedirects
// slice from the render worker pool. Concurrent appends both lose entries (two
// goroutines read the same length and write the same index) and race on the
// slice header, so a site could silently emit fewer 301s in _redirects than it
// declared aliases.
//
// The existing corpora never caught it because none of them use frontmatter
// aliases, so the write path never ran under -race. This drives the same append
// path the renderer uses, with every page declaring an alias.
func TestAliasRedirectsUnderParallelRender(t *testing.T) {
	const pages = 200

	g := newTestGen(t, "")
	g.config.AliasStubsOff = true // only the redirect record, no stub files

	items := make([]models.Page, 0, pages)
	for i := 0; i < pages; i++ {
		items = append(items, models.Page{
			Slug:    fmt.Sprintf("page-%03d", i),
			Type:    "page",
			Status:  "publish",
			Aliases: []string{fmt.Sprintf("/old/page-%03d", i)},
		})
	}

	// Same shape as renderContent: a worker pool calling into writeAliasStubs.
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, p := range items {
		wg.Add(1)
		go func(p models.Page) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			g.writeAliasStubs(p)
		}(p)
	}
	wg.Wait()

	if got := len(g.aliasRedirects); got != pages {
		t.Fatalf("recorded %d alias redirects, want %d — concurrent appends lost entries", got, pages)
	}
	// Every alias must be present exactly once.
	seen := make(map[string]int, pages)
	for _, r := range g.aliasRedirects {
		seen[r.From]++
	}
	for i := 0; i < pages; i++ {
		from := fmt.Sprintf("/old/page-%03d", i)
		if seen[from] != 1 {
			t.Errorf("alias %s recorded %d times, want exactly 1", from, seen[from])
		}
	}
}

// TestAliasRedirectOrderIsDeterministic pins the other half of the same bug: the
// pool records aliases in scheduler order, so the emitted _redirects differed
// between builds of identical content. Whatever order they arrive in, the rules
// must come out the same.
func TestAliasRedirectOrderIsDeterministic(t *testing.T) {
	rules := []RedirectRule{
		{From: "/c", To: "/three", Status: 301},
		{From: "/a", To: "/one", Status: 301},
		{From: "/b", To: "/two", Status: 301},
		{From: "/a", To: "/one-bis", Status: 302},
	}
	want := sortedRedirects(rules)

	// Every rotation of the input must produce the same output.
	for shift := 0; shift < len(rules); shift++ {
		shuffled := append(append([]RedirectRule(nil), rules[shift:]...), rules[:shift]...)
		got := sortedRedirects(shuffled)
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("rotation %d changed the order at %d: got %+v, want %+v", shift, i, got[i], want[i])
			}
		}
	}
	// Sorted by From, then To, then Status.
	if want[0].From != "/a" || want[0].To != "/one" || want[1].To != "/one-bis" || want[3].From != "/c" {
		t.Errorf("unexpected order: %+v", want)
	}
	// The input slice must not be reordered in place — callers keep using it.
	if rules[0].From != "/c" {
		t.Error("sortedRedirects must not mutate its argument")
	}
}
