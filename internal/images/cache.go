package images

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
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
	// A quality the request leaves to the config is part of the variant: without
	// it, changing webp_quality/avif_quality would keep serving variants encoded
	// at the old setting (#375). Explicit qualities are already in opsJSON, so
	// their keys — and the golden formula — are unchanged.
	if q := p.configQuality(finalFormat(ops, formatFromPath(path)), ops); q > 0 {
		k.WriteString(fmt.Sprintf("|quality=%d", q))
	}
	return k.Sum(), nil
}

// configQuality returns the configured quality a WebP/AVIF variant will be
// encoded at when no operation sets one, or 0 when it does not apply.
func (p *Processor) configQuality(format string, ops []request) int {
	if finalQuality(ops) > 0 {
		return 0
	}
	switch format {
	case "webp":
		return p.cfg.WebPQuality
	case "avif":
		return p.cfg.AVIFQuality
	default:
		return 0
	}
}

// outputName builds the deterministic published name: <base>.<hash>.<ext>.
func outputName(source, key, format string) string {
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	return fmt.Sprintf("%s.%s.%s", base, key, extFor(format))
}

// finalFormat resolves the effective output format for a pipeline: the last
// explicit format wins; `auto`/empty keeps the source format (never silently
// converting alpha-capable sources to JPEG). There is no GIF encoder, so a
// static GIF under `auto` becomes PNG — lossless and transparency-preserving
// (#375).
func finalFormat(ops []request, sourceFormat string) string {
	format := ""
	for _, op := range ops {
		if op.Format != "" {
			format = strings.ToLower(op.Format)
		}
	}
	if format == "" || format == "auto" {
		if sourceFormat == "" || sourceFormat == "gif" {
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
	cfg, format, derr := cachedConfigAt(cachePath, finalFormat(ops, formatFromPath(path)))
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

// cachedConfigAt reads the dimensions/format of a cached variant. AVIF has no
// registered decoder, so it is verified from its own header instead — decoding
// it always failed, which made every AVIF variant a cache miss (#375).
func cachedConfigAt(path, format string) (image.Config, string, error) {
	if format == "avif" {
		cfg, err := avifConfigAt(path)
		return cfg, format, err
	}
	return decodeConfigAt(path)
}

// avifHeaderLimit bounds how much of a cached AVIF is scanned for its ispe box;
// avifenc writes the meta box (which holds it) right after ftyp.
const avifHeaderLimit = 64 << 10

// avifConfigAt reads an AVIF's dimensions from its ftyp + ispe boxes: the file
// must start with an ftyp box and carry a non-zero image spatial extent.
func avifConfigAt(path string) (image.Config, error) {
	f, err := os.Open(path) // #nosec G304 -- cache-internal path
	if err != nil {
		return image.Config{}, err
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, avifHeaderLimit)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	if len(head) < 12 || string(head[4:8]) != "ftyp" {
		return image.Config{}, fmt.Errorf("%s: not an AVIF (no ftyp box)", path)
	}
	i := bytes.Index(head, []byte("ispe"))
	// ispe: 4-byte type, 4-byte version/flags, then 32-bit width and height.
	if i < 0 || i+16 > len(head) {
		return image.Config{}, fmt.Errorf("%s: AVIF without a readable ispe box", path)
	}
	w := binary.BigEndian.Uint32(head[i+8:])
	h := binary.BigEndian.Uint32(head[i+12:])
	if w == 0 || h == 0 || w > math.MaxInt32 || h > math.MaxInt32 {
		return image.Config{}, fmt.Errorf("%s: AVIF with invalid dimensions %dx%d", path, w, h)
	}
	return image.Config{Width: int(w), Height: int(h)}, nil
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
