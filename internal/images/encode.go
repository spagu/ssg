package images

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// extFor maps a normalized format to its file extension.
func extFor(format string) string {
	if format == "jpeg" {
		return "jpg"
	}
	return format
}

// encode writes img to f in the requested format. JPEG/PNG use the standard
// library; WebP shells out to the optional cwebp tool (same dependency as the
// --webp pipeline) and returns a descriptive error when it is missing.
func (p *Processor) encode(f *os.File, img image.Image, format string, quality int) error {
	switch format {
	case "jpeg":
		if quality <= 0 {
			quality = p.cfg.JPEGQuality
		}
		return jpeg.Encode(f, img, &jpeg.Options{Quality: quality})
	case "png":
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		return enc.Encode(f, img)
	case "webp":
		if quality <= 0 {
			quality = p.cfg.WebPQuality
		}
		return encodeWebP(f, img, quality)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

// encodeWebP writes a lossy WebP via cwebp: the pixels go to a temporary PNG
// which cwebp converts into the target file. Pure-Go lossy WebP encoding does
// not exist without CGO, so this mirrors the project's existing optional-tool
// approach (documented limitation).
func encodeWebP(f *os.File, img image.Image, quality int) error {
	cwebp, err := exec.LookPath("cwebp") // NOSONAR S4036: optional tool intentionally resolved from PATH, like --webp
	if err != nil {
		return fmt.Errorf("webp output requires the optional cwebp tool (install the webp package)")
	}
	tmpPNG, err := os.CreateTemp(filepath.Dir(f.Name()), "tmp-*.png")
	if err != nil {
		return err
	}
	tmpName := tmpPNG.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := png.Encode(tmpPNG, img); err != nil {
		_ = tmpPNG.Close()
		return err
	}
	if err := tmpPNG.Close(); err != nil {
		return err
	}
	// #nosec G204 -- fixed optional tool; only sanitized temp paths vary (SEC-011)
	cmd := exec.Command(cwebp, "-quiet", "-q", strconv.Itoa(quality), safeImgArg(tmpName), "-o", safeImgArg(f.Name()))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cwebp: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// safeImgArg prefixes relative paths with "./" so they can never be parsed as
// CLI options (SEC-011 pattern shared with cwebp/dart-sass call sites).
func safeImgArg(p string) string {
	if p == "" || filepath.IsAbs(p) || strings.HasPrefix(p, ".") {
		return p
	}
	return "./" + p
}
