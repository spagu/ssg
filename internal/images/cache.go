package images

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// cacheKey derives the deterministic content-addressed key: source bytes hash +
// normalized operations JSON + processor version. Mtime is never used.
func (p *Processor) cacheKey(path string, ops []request) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- path validated by resolve()
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	opsJSON, err := json.Marshal(ops)
	if err != nil {
		return "", err
	}
	h.Write(opsJSON)
	h.Write([]byte("v" + processorVersion))
	return hex.EncodeToString(h.Sum(nil))[:10], nil
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

	// #nosec G301 -- cache/output directories hold public build artifacts
	if err := os.MkdirAll(p.cfg.CacheDir, 0o755); err != nil {
		return ImageResult{}, fmt.Errorf("%s: %w", helper, err)
	}
	tmp, err := os.CreateTemp(p.cfg.CacheDir, "tmp-*."+extFor(format))
	if err != nil {
		return ImageResult{}, fmt.Errorf("%s: %w", helper, err)
	}
	tmpName := tmp.Name()
	if err := p.encode(tmp, img, format, finalQuality(ops)); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return ImageResult{}, fmt.Errorf("%s: %w", helper, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return ImageResult{}, fmt.Errorf("%s: %w", helper, err)
	}
	if err := os.Rename(tmpName, cachePath); err != nil { // atomic publish
		_ = os.Remove(tmpName)
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
func (p *Processor) GC(dryRun bool) (files int, bytes int64, err error) {
	entries, err := os.ReadDir(p.cfg.CacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if p.manifest[name] && !strings.HasPrefix(name, "tmp-") {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		files++
		bytes += info.Size()
		if !dryRun {
			_ = os.Remove(filepath.Join(p.cfg.CacheDir, name))
		}
	}
	return files, bytes, nil
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
