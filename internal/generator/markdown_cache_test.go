package generator

// Conversions kept between builds (#270).
//
// The one property that matters is the same one the incremental build rests
// on: a warm cache must produce exactly the site a cold cache does. A stale
// hit here is wrong HTML under a green build, which is the failure this whole
// area of the codebase is arranged to avoid.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cacheFixture builds a small site whose Markdown exercises the parts of the
// pipeline the key has to describe: a heading, a fenced block, a list, a link.
func cacheFixture(t *testing.T) Config {
	t.Helper()
	body := "---\ntitle: %s\nslug: %s\nstatus: publish\ntype: page\n---\n\n" +
		"# A heading with a [link](/x/)\n\nSome **prose** and a list:\n\n- one\n- two\n\n" +
		"```go\nfunc main() {}\n```\n\nA closing line.\n"
	files := map[string]string{}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		files["pages/"+name+".md"] = strings.ReplaceAll(body, "%s", name)
	}
	cfg := newSiteFixture(t, `{"categories":[],"media":[],"users":[]}`, files, func(name string) string {
		if name == "page.html" || name == "post.html" {
			return `<html><head><title>{{ .Title }}</title></head><body>{{ .Content | safeHTML }}</body></html>`
		}
		return `<html><head><title>x</title></head><body><p>x</p></body></html>`
	})
	cfg.CacheDir = filepath.Join(t.TempDir(), "cache")
	cfg.MarkdownCache = true
	cfg.Quiet = true
	return cfg
}

// TestAWarmCacheBuildsTheSameSiteAsAColdOne, byte for byte. The king criterion
// of this feature, exactly as it was for the incremental build.
func TestAWarmCacheBuildsTheSameSiteAsAColdOne(t *testing.T) {
	cold := cacheFixture(t)
	cold.MarkdownCache = false
	buildSiteFixture(t, cold)

	warm := cacheFixture(t)
	buildSiteFixture(t, warm) // fills the cache
	first := treeOf(t, warm.OutputDir)
	if err := os.RemoveAll(warm.OutputDir); err != nil {
		t.Fatal(err)
	}
	buildSiteFixture(t, warm) // reads it back

	compareTreesExactly(t, treeOf(t, cold.OutputDir), first, "cold vs first warm build")
	compareTreesExactly(t, treeOf(t, cold.OutputDir), treeOf(t, warm.OutputDir), "cold vs cached build")
}

// TestASecondBuildConvertsNothing: the point of the cache. Without it every
// build converts every page, whether or not the page will be written.
func TestASecondBuildConvertsNothing(t *testing.T) {
	cfg := cacheFixture(t)
	cfg.Profile = ProfileText

	if counterValue(buildAndProfile(t, cfg), "markdown conversions") == 0 {
		t.Fatal("the first build should have converted something")
	}
	second := buildAndProfile(t, cfg)
	if got := counterValue(second, "markdown conversions"); got != 0 {
		t.Errorf("a second build converted %d document(s) it had already converted", got)
	}
	if counterValue(second, "markdown cache hits (disk)") == 0 {
		t.Error("nothing was read back from the cache")
	}
}

// TestAChangedRendererIsNotAStaleHit.
//
// The whole safety of this rests on the key describing every input that can
// change the answer. Syntax highlighting is the one a person turns on and off
// most, and a build that reused yesterday's plain <pre> for it would look like
// the setting had no effect.
func TestAChangedRendererIsNotAStaleHit(t *testing.T) {
	plain := cacheFixture(t)
	buildSiteFixture(t, plain)
	before := mustRead(t, filepath.Join(plain.OutputDir, "alpha", "index.html"))

	highlighted := plain
	highlighted.Highlight = true
	buildSiteFixture(t, highlighted)
	after := mustRead(t, filepath.Join(highlighted.OutputDir, "alpha", "index.html"))

	if before == after {
		t.Error("turning highlighting on reused the conversion made without it")
	}
	if !strings.Contains(after, "<span") {
		t.Errorf("the highlighted build produced no spans:\n%s", after)
	}
}

// TestRenderHooksTurnTheCacheOff.
//
// A hook is a template, and a template can call the build's helpers — anything
// that reads the whole site. A conversion that depends on the rest of the site
// is not a function of its own input, so a key over the input would be a lie.
func TestRenderHooksTurnTheCacheOff(t *testing.T) {
	cfg := cacheFixture(t)
	hook := filepath.Join(t.TempDir(), "image.html")
	mustWrite(t, hook, `<figure><img src="{{ .Src }}" alt="{{ .Alt }}"></figure>`)
	cfg.RenderHooks = map[string]string{"image": hook}

	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	if _, usable, reason := gen.markdownCacheUsable(); usable {
		t.Error("a build with render hooks must not keep conversions")
	} else if !strings.Contains(reason, "whole site") {
		t.Errorf("reason = %q", reason)
	}
	if _, err := os.Stat(filepath.Join(cfg.CacheDir, MarkdownCacheDirName)); err == nil {
		t.Error("nothing should have been written to the cache")
	}
}

// TestTurningTheCacheOffKeepsTheSameOutput, so the setting is a performance
// decision and never a correctness one.
func TestTurningTheCacheOffKeepsTheSameOutput(t *testing.T) {
	on := cacheFixture(t)
	buildSiteFixture(t, on)
	off := cacheFixture(t)
	off.MarkdownCache = false
	buildSiteFixture(t, off)
	compareTreesExactly(t, treeOf(t, off.OutputDir), treeOf(t, on.OutputDir), "cache off vs on")
}

// TestACacheThatCannotBeWrittenIsNotABuildFailure: the site is already correct
// without it.
func TestACacheThatCannotBeWrittenIsNotABuildFailure(t *testing.T) {
	cfg := cacheFixture(t)
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.CacheDir = blocker // a file where the cache root has to go
	buildSiteFixture(t, cfg)
	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "alpha", "index.html")); err != nil {
		t.Errorf("the site should still have been built: %v", err)
	}
}

// counterValue reads one of a build's counters, zero when it never fired.
func counterValue(p *Profile, name string) int64 {
	for _, c := range p.Counters() {
		if c.Name == name {
			return c.Value
		}
	}
	return 0
}

// buildAndProfile runs one build and returns what it measured.
func buildAndProfile(t *testing.T, cfg Config) *Profile {
	t.Helper()
	gen, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(); err != nil {
		t.Fatal(err)
	}
	return gen.Profile()
}

// TestTheCacheKeyDescribesTheRendererAndTheSource, which is the property every
// other test here depends on: two different sources, or two different
// renderers, must never name the same entry.
func TestTheCacheKeyDescribesTheRendererAndTheSource(t *testing.T) {
	if markdownCacheKey("fp", "a") == markdownCacheKey("fp", "b") {
		t.Error("two sources share a key")
	}
	if markdownCacheKey("one", "a") == markdownCacheKey("two", "a") {
		t.Error("two renderers share a key")
	}
	// The delimiter is what stops "ab"+"c" colliding with "a"+"bc".
	if markdownCacheKey("ab", "c") == markdownCacheKey("a", "bc") {
		t.Error("the key material is not delimited")
	}
	// Stable across calls: a key that varied would make every build a miss.
	first := markdownCacheKey("fp", "a")
	if markdownCacheKey("fp", string([]byte{'a'})) != first {
		t.Error("the key is not stable")
	}
}

// TestTheCacheLandsBesideTheOtherCaches, and follows cache_dir when a project
// puts them somewhere else.
func TestTheCacheLandsBesideTheOtherCaches(t *testing.T) {
	g := &Generator{}
	if got := g.markdownCacheDir(); got != filepath.Join(".ssg-cache", MarkdownCacheDirName) {
		t.Errorf("default dir = %q", got)
	}
	g.config.CacheDir = filepath.Join("var", "caches")
	if got := g.markdownCacheDir(); got != filepath.Join("var", "caches", MarkdownCacheDirName) {
		t.Errorf("configured dir = %q", got)
	}
}

// TestEntriesAreSpreadOverSubdirectories: a five-thousand-page site is five
// thousand files, and one directory that size is slow to open on every
// filesystem that still looks entries up linearly.
func TestEntriesAreSpreadOverSubdirectories(t *testing.T) {
	if got := cacheShardDir("root", "abcdef"); got != filepath.Join("root", "ab") {
		t.Errorf("shard = %q", got)
	}
	// A name too short to shard stays at the root rather than panicking.
	if got := cacheShardDir("root", "a"); got != "root" {
		t.Errorf("shard = %q", got)
	}
}

// TestTheRendererVersionIsPartOfTheKey: upgrading goldmark has to invalidate
// every entry, and nobody is going to remember to do it by hand.
func TestTheRendererVersionIsPartOfTheKey(t *testing.T) {
	if got := goldmarkVersion(); got == "" || got == "unknown" {
		t.Errorf("goldmarkVersion = %q — the key cannot see a dependency upgrade", got)
	}
}
