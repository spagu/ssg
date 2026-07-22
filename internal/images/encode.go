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
	case "avif":
		if quality <= 0 {
			quality = p.cfg.AVIFQuality
		}
		return encodeAVIF(f, img, quality, p.cfg.AVIFSpeed)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

// encodeAVIF writes AVIF via the optional avifenc tool (libavif). Like WebP, no
// portable CGO-free encoder exists, so this mirrors the cwebp approach: pixels
// go to a temporary PNG which avifenc converts. A missing binary is a
// descriptive error, so a theme requesting AVIF still builds on a machine
// without the encoder when the caller skips unavailable formats (issue #43).
func encodeAVIF(f *os.File, img image.Image, quality, speed int) error {
	avifenc, err := exec.LookPath("avifenc") // NOSONAR S4036: optional tool intentionally resolved from PATH, like cwebp
	if err != nil {
		return fmt.Errorf("avif output requires the optional avifenc tool (install libavif)")
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
	cmd := exec.Command(avifenc, "-q", strconv.Itoa(quality), "-s", strconv.Itoa(speed), safeImgArg(tmpName), safeImgArg(f.Name()))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("avifenc: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
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
