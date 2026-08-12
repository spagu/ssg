package images

import (
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/cache"
)

// cacheKey derives the deterministic content-addressed key: source bytes hash +
// normalized operations JSON + processor version. Mtime is never used. The
// formula is golden-tested (TestCacheKeyGolden) — changing it invalidates every
// user's image cache.
func (p *Processor) cacheKey(path string, ops []request) (string, error) {
	k := cache.NewKeyer(processorVersion, 10)
	if err := k.WriteFileContents(path); err != nil {
		return "", err
	}
	opsJSON, err := json.Marshal(ops)
	if err != nil {
		return "", err
	}
	k.Write(opsJSON)
	return k.Sum(), nil
}

// outputName builds the deterministic published name: <base>.<hash>.<ext>.
func outputName(source, key, format string) string {
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	return fmt.Sprintf("%s.%s.%s", base, key, extFor(format))
}

// finalFormat resolves the effective output format for a pipeline: the last
// explicit format wins; `auto`/empty keeps the source format (never silently
// converting alpha-capable sources to JPEG).
func finalFormat(ops []request, sourceFormat string) string {
	format := ""
	for _, op := range ops {
		if op.Format != "" {
			format = strings.ToLower(op.Format)
		}
	}
	if format == "" || format == "auto" {
		if sourceFormat == "" {
			return "png"
		}
		return sourceFormat
	}
	if format == "jpg" {
		return "jpeg"
	}
	return format
}

// finalQuality resolves the effective quality (last explicit wins; 0 = default).
func finalQuality(ops []request) int {
	q := 0
	for _, op := range ops {
		if op.Quality > 0 {
			q = op.Quality
		}
	}
	return q
}

// cached returns a previously published result when both the cache entry and
// the published output already exist.
func (p *Processor) cached(source, path, key string, ops []request) (ImageResult, bool) {
	name := outputName(source, key, finalFormat(ops, formatFromPath(path)))
	cachePath := filepath.Join(p.cfg.CacheDir, name)
	outPath := filepath.Join(p.cfg.OutputDir, p.cfg.URLPrefix, name)

	cacheInfo, err := os.Stat(cachePath)
	if err != nil {
		return ImageResult{}, false
	}
	if _, err := os.Stat(outPath); err != nil {
		// Cache hit but not yet published into this build's output: copy through.
		if err := copyFile(cachePath, outPath); err != nil {
			return ImageResult{}, false
		}
	}
	cfg, format, derr := decodeConfigAt(cachePath)
	if derr != nil {
		return ImageResult{}, false
	}
	p.markManifest(name)
	return ImageResult{
		URL:        "/" + p.cfg.URLPrefix + "/" + name,
		StaticPath: filepath.ToSlash(filepath.Join(p.cfg.URLPrefix, name)),
		SourcePath: filepath.ToSlash(source),
		Width:      cfg.Width,
		Height:     cfg.Height,
		Format:     format,
		FileSize:   cacheInfo.Size(),
		CacheKey:   key,
	}, true
}

// publish encodes the processed image to a temp file, atomically renames it
// into the cache and copies it into the build output. Partial output is never
// visible.
func (p *Processor) publish(helper, source, path, key string, ops []request, img image.Image, info ImageInfo) (ImageResult, error) {
	format := finalFormat(ops, info.Format)
	name := outputName(source, key, format)
	cachePath := filepath.Join(p.cfg.CacheDir, name)

	// Atomic publish via the shared cache engine (GO-091); 0o600 matches the
	// historical CreateTemp mode of cache entries.
	if err := cache.WriteAtomic(p.cfg.CacheDir, name, 0o600, func(tmp *os.File) error {
		return p.encode(tmp, img, format, finalQuality(ops))
	}); err != nil {
		return ImageResult{}, fmt.Errorf("%s: %w", helper, err)
	}

	outPath := filepath.Join(p.cfg.OutputDir, p.cfg.URLPrefix, name)
	if err := copyFile(cachePath, outPath); err != nil {
		return ImageResult{}, fmt.Errorf("%s: publishing output: %w", helper, err)
	}
	st, err := os.Stat(cachePath)
	if err != nil {
		return ImageResult{}, fmt.Errorf("%s: %w", helper, err)
	}
	b := img.Bounds()
	p.markManifest(name)
	return ImageResult{
		URL:            "/" + p.cfg.URLPrefix + "/" + name,
		StaticPath:     filepath.ToSlash(filepath.Join(p.cfg.URLPrefix, name)),
		SourcePath:     filepath.ToSlash(source),
		Width:          b.Dx(),
		Height:         b.Dy(),
		OriginalWidth:  info.Width,
		OriginalHeight: info.Height,
		Format:         format,
		FileSize:       st.Size(),
		CacheKey:       key,
	}, nil
}

// markManifest records a cache entry as referenced by the current build.
func (p *Processor) markManifest(name string) {
	p.mu.Lock()
	p.manifest[name] = true
	p.mu.Unlock()
}

// GC removes cache entries not referenced by the current build (and stale temp
// files), reporting the number of files and bytes reclaimed. dryRun only counts.
// Delegates to the shared engine (GO-091); the manifest read is what needs the
// processor lock.
func (p *Processor) GC(dryRun bool) (files int, bytes int64, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return cache.GCKeep(p.cfg.CacheDir, func(name string) bool { return p.manifest[name] }, dryRun)
}

// copyFile copies src to dst, creating parent directories.
func copyFile(src, dst string) error {
	// #nosec G301 -- web output directories must be world-traversable
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src) // #nosec G304 -- paths derived from validated cache entries
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst) // #nosec G304 -- publishes into the build output dir
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	return err
}

// decodeConfigAt reads dimensions/format of an on-disk image.
func decodeConfigAt(path string) (image.Config, string, error) {
	f, err := os.Open(path) // #nosec G304 -- cache-internal path
	if err != nil {
		return image.Config{}, "", err
	}
	defer func() { _ = f.Close() }()
	return image.DecodeConfig(f)
}
