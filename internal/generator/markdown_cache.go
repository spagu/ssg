package generator

// Markdown conversions kept between builds (#270).
//
// GO-094 narrowed the render phase and found the ceiling: on a 5 000-post
// corpus, editing one post renders 268 pages instead of 5 251 and the wall
// clock barely moves, because rendering is a quarter of a warm build and
// loading content is half. Most of that half is this: every build converts
// Markdown for every page, whether or not the page will be written, because a
// listing template may render any page's body.
//
// A conversion is a pure function of its input. The same bytes, through the
// same renderer, produce the same HTML — which `scripts/determinism.sh` has
// proved for the whole build since BUILD-PARALLEL. So the answer can be kept.
//
// The danger is the one every cache has, and here it is the expensive kind: a
// stale hit is wrong HTML shipped under a green build. Everything below is
// about making the key describe every input that can change the answer, and
// refusing to cache at all where it cannot.

import (
	"os"
	"runtime/debug"
	"strconv"
	"sync"

	"github.com/spagu/ssg/internal/cache"
)

// MarkdownCacheDirName is the namespace under the shared cache root (GO-091).
const MarkdownCacheDirName = "markdown"

// markdownCacheVersion is the implementation tag in every key. **Bump it
// whenever anything about the conversion pipeline changes** — an extension
// added, a transformer's behaviour altered, a pre-processing pass moved. It is
// the one part of the key a compiler cannot derive, and the one that makes an
// upgrade cost a rebuild instead of shipping last version's HTML.
const markdownCacheVersion = "1"

// markdownFingerprint is the part of a cache key that is the same for every
// page in one build: the renderer's configuration and the code that runs it.
//
// It is computed once and reused, because it hashes files from disk and a
// per-page recomputation would cost more than the conversions it saves.
type markdownFingerprint struct {
	once   sync.Once
	value  string
	usable bool
	reason string
}

// markdownCacheDir is where conversions are kept.
func (g *Generator) markdownCacheDir() string {
	root := g.config.CacheDir
	if root == "" {
		root = cache.DefaultRoot
	}
	return cache.Dir(root, MarkdownCacheDirName)
}

// markdownCacheUsable reports whether this build's conversions can be kept, and
// why not when they cannot.
//
// **Render hooks disable it.** A hook is a template, and a template can call
// the build's helpers — `recentPosts`, a taxonomy lookup, anything that reads
// the whole site. A conversion that depends on the rest of the site is not a
// function of its own input any more, and a key over the input would be a lie.
// Hooks are opt-in and new, so this costs almost nobody anything, and the
// alternative is the failure this cache exists to avoid.
func (g *Generator) markdownCacheUsable() (string, bool, string) {
	g.mdFingerprint.once.Do(func() {
		if !g.config.MarkdownCache {
			g.mdFingerprint.reason = "markdown_cache is off"
			return
		}
		if len(g.config.RenderHooks) > 0 {
			g.mdFingerprint.reason = "render hooks can read the whole site, so a conversion is not a function of its own input"
			return
		}
		k := cache.NewKeyer(markdownCacheVersion, 0)
		k.WriteDelim("ssg=" + g.config.Version)
		k.WriteDelim("goldmark=" + goldmarkVersion())
		k.WriteDelim("highlight=" + strconv.FormatBool(g.config.Highlight))
		k.WriteDelim("style=" + g.config.HighlightStyle)
		k.WriteDelim("lineno=" + strconv.FormatBool(g.config.HighlightLineNumbers))
		g.mdFingerprint.value = k.Sum()
		g.mdFingerprint.usable = true
	})
	return g.mdFingerprint.value, g.mdFingerprint.usable, g.mdFingerprint.reason
}

// goldmarkVersion is the version of the renderer this binary was built with, so
// upgrading the dependency invalidates every key without anybody remembering to.
func goldmarkVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/yuin/goldmark" {
			if dep.Replace != nil {
				return dep.Replace.Path + "@" + dep.Replace.Version
			}
			return dep.Version
		}
	}
	return "unknown"
}

// markdownCacheKey names one conversion: the fingerprint of the renderer, plus
// the exact source it is given.
//
// The source is the string AFTER this build's pre-processing — smart-punctuation
// folding, math fences, mermaid fences — which is what makes those settings part
// of the key without being named in it.
func markdownCacheKey(fingerprint, source string) string {
	k := cache.NewKeyer(markdownCacheVersion, 0)
	k.WriteDelim(fingerprint)
	k.WriteDelim(source)
	return k.Sum()
}

// lookupMarkdownCache returns a conversion kept by an earlier build.
func (g *Generator) lookupMarkdownCache(source string) (string, bool) {
	fingerprint, usable, _ := g.markdownCacheUsable()
	if !usable {
		return "", false
	}
	data, err := os.ReadFile(markdownCachePath(g.markdownCacheDir(), markdownCacheKey(fingerprint, source))) // #nosec G304 -- the build's own cache
	if err != nil {
		return "", false
	}
	return string(data), true
}

// storeMarkdownCache keeps one conversion for the next build. A cache that
// cannot be written is not a build failure: the site is already correct.
func (g *Generator) storeMarkdownCache(source, html string) {
	fingerprint, usable, _ := g.markdownCacheUsable()
	if !usable {
		return
	}
	dir := g.markdownCacheDir()
	name := markdownCacheKey(fingerprint, source)
	// #nosec G306 -- a cache file beside the project's other caches
	_ = cache.WriteAtomicBytes(cacheShardDir(dir, name), name, 0o644, []byte(html))
}

// markdownCachePath is where one key lives.
func markdownCachePath(dir, name string) string {
	return cacheShardDir(dir, name) + string(os.PathSeparator) + name
}

// cacheShardDir spreads entries over 256 directories by the key's first byte.
// A five-thousand-page site is five thousand files, and a single directory that
// size is slow to open on every filesystem that still has a linear lookup.
func cacheShardDir(dir, name string) string {
	if len(name) < 2 {
		return dir
	}
	return dir + string(os.PathSeparator) + name[:2]
}
