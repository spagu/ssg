package webp

// Site-level AVIF, beside the WebP pass (#178).
//
// AVIF has been reachable since #43 — but only from a template helper, per
// call. A site could not simply ask for it, which meant a hosted builder
// exposing ssg's settings had no key to write, and a migrated WordPress site
// full of camera JPEGs kept paying for them. The measurement from the report,
// on one hero image: 571 KB JPEG → 279 KB WebP (−51%) → 96 KB AVIF (−83%).
//
// This adds the derivative and the markup, not a second pipeline. WebP stays
// the default (the non-goal from #43 is unchanged); AVIF is emitted alongside
// it when asked for, and the <img> the WebP pass produced becomes a <picture>
// offering AVIF first with the WebP as fallback. A browser that understands
// neither still gets the original.
//
// The optional-binary rule from #43 holds: no avifenc means the format is
// skipped with one warning and the build carries on. A site is never broken by
// a tool the machine does not have.

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
	"github.com/spagu/ssg/internal/images"
)

// AVIFOptions configures the site-level AVIF pass.
type AVIFOptions struct {
	// Quality is avifenc's -q (0..100). Lower than the WebP default on
	// purpose: AVIF holds detail at settings where WebP visibly softens.
	Quality int
	// Speed is avifenc's -s (0..10). Higher is faster and larger; 6 keeps a
	// full-site pass tolerable without giving up much.
	Speed   int
	Sizes   []int
	Force   bool
	Quiet   bool
	Workers int
}

// avifTargetPath maps an image path to its .avif sibling, stripping the
// original extension by length so "Photo.JPG" does not become "Photo.JPG.avif"
// (the same trap webpTargetPath documents).
func avifTargetPath(imgPath string) string {
	return imgPath[:len(imgPath)-len(filepath.Ext(imgPath))] + ".avif"
}

// avifVariantPath is the responsive-variant name for a width.
func avifVariantPath(avifPath string, width int) string {
	return strings.TrimSuffix(avifPath, ".avif") + fmt.Sprintf("-%d.avif", width)
}

// AVIFAvailable reports whether the encoder is installed, so a caller can warn
// once for the site rather than once per image.
func AVIFAvailable() bool {
	_, err := exec.LookPath("avifenc") // NOSONAR S4036: optional tool, resolved from PATH like cwebp
	return err == nil
}

// ConvertDirectoryAVIF writes an .avif beside every convertible image under
// dir, plus one per configured responsive width, and returns how many it made.
//
// It converts from the ORIGINAL rather than from the .webp: re-encoding a lossy
// file into another lossy format compounds both encoders' artefacts, and the
// original is still on disk whenever the WebP pass kept it. Where it is not,
// the .webp is the only source there is and is used as one.
func ConvertDirectoryAVIF(dir string, opts AVIFOptions) (int, error) {
	if !AVIFAvailable() {
		return 0, fmt.Errorf("avif requested but the avifenc tool is not installed " +
			"(Debian/Ubuntu: apt install libavif-bin; Alpine: apk add libavif-apps; macOS: brew install libavif)")
	}
	if opts.Quality <= 0 || opts.Quality > 100 {
		opts.Quality = 45
	}
	if opts.Speed <= 0 || opts.Speed > 10 {
		opts.Speed = 6
	}

	sources, err := avifSources(dir)
	if err != nil {
		return 0, err
	}

	var (
		mu    sync.Mutex
		made  int
		sem   = make(chan struct{}, resolveWorkers(opts.Workers))
		group sync.WaitGroup
	)
	for _, src := range sources {
		group.Add(1)
		go func(src string) {
			defer group.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			n := convertOneAVIF(src, opts)
			mu.Lock()
			made += n
			mu.Unlock()
		}(src)
	}
	group.Wait()
	return made, nil
}

// avifSources lists the images worth an AVIF derivative: the JPEGs and PNGs a
// build published.
//
// Originals only, and that is why this pass runs BEFORE the WebP one. avifenc
// cannot read WebP at all — it reports "unrecognized file format" — and even
// where a decoder existed, re-encoding one lossy format into another compounds
// both encoders' artefacts. The WebP pass replaces originals in place unless
// keep_original is set, so converting first is the only ordering that always
// has a source to work from.
func avifSources(dir string) ([]string, error) {
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil //nolint:nilerr // an unreadable subtree is not worth failing the build
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".jpg", ".jpeg", ".png":
			// Skip a responsive variant: it is a derivative, and its own AVIF
			// is made from the original at the same width.
			if isVariantName(path) {
				return nil
			}
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// isVariantName reports whether a file is a generated -<width> derivative.
func isVariantName(path string) bool {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	i := strings.LastIndex(base, "-")
	if i < 0 || i == len(base)-1 {
		return false
	}
	_, err := strconv.Atoi(base[i+1:])
	return err == nil
}

// convertOneAVIF writes the full-size AVIF and its responsive variants,
// returning how many files it produced.
func convertOneAVIF(src string, opts AVIFOptions) int {
	made := 0
	dst := avifTargetPath(src)
	if opts.Force || !fileExists(dst) {
		if err := encodeAVIFFile(src, dst, opts.Quality, opts.Speed, 0); err != nil {
			if !opts.Quiet {
				fmt.Printf("   ⚠️  AVIF failed for %s: %v\n", filepath.Base(src), err)
			}
			return 0
		}
		made++
	}
	origWidth, ok := imageWidth(src)
	if !ok {
		return made
	}
	for _, w := range opts.Sizes {
		if w <= 0 || w >= origWidth {
			continue // never upscale
		}
		vdst := avifVariantPath(dst, w)
		if !opts.Force && fileExists(vdst) {
			continue
		}
		if err := encodeAVIFFile(src, vdst, opts.Quality, opts.Speed, w); err != nil {
			if !opts.Quiet {
				fmt.Printf("   ⚠️  AVIF %dw variant failed for %s: %v\n", w, filepath.Base(src), err)
			}
			continue
		}
		made++
	}
	return made
}

// fileExists is the cheap existence check the conversion loop needs.
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// encodeAVIFFile runs avifenc on a file, optionally resizing first.
//
// avifenc has no resize flag, so a width goes through cwebp's resizer into a
// temporary PNG. That costs one extra process per variant and is still worth
// it: the alternative is decoding and scaling in-process, which would pull a
// resampler into a package whose whole design is "shell out to the encoder the
// distribution already ships".
func encodeAVIFFile(src, dst string, quality, speed, width int) error {
	input := src
	if width > 0 {
		tmp, err := os.CreateTemp(filepath.Dir(dst), "avif-src-*.png")
		if err != nil {
			return err
		}
		tmpName := tmp.Name()
		_ = tmp.Close()
		defer func() { _ = os.Remove(tmpName) }()
		if err := resizeToPNG(src, tmpName, width); err != nil {
			return err
		}
		input = tmpName
	}
	// #nosec G204 -- fixed optional tool (avifenc); only sanitized paths and
	// numeric args vary, exactly as convertImage does for cwebp (SEC-011).
	cmd := exec.Command("avifenc", // NOSONAR S4036: optional tool intentionally resolved from PATH
		"-q", strconv.Itoa(quality),
		"-s", strconv.Itoa(speed),
		safeArgPath(input), safeArgPath(dst))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("avifenc: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// resizeToPNG scales an image to width and writes PNG for avifenc to read.
//
// In process, with the resampler this project already depends on. The obvious
// alternative — piping through cwebp's -resize and back out with dwebp — costs
// two extra processes per variant and a lossy round trip, to avoid a dependency
// that is already in go.mod for the template helpers.
func resizeToPNG(src, dst string, width int) error {
	if err := refuseUndecodable(src); err != nil {
		return err
	}
	img, err := imaging.Open(src, imaging.AutoOrientation(true))
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Base(src), err)
	}
	// Height 0 keeps the aspect ratio. Lanczos because these are photographs
	// being shrunk, which is exactly what it is for.
	resized := imaging.Resize(img, width, 0, imaging.Lanczos)
	out, err := os.Create(dst) // #nosec G304 -- a temp file this function just named
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	return png.Encode(out, resized)
}

// refuseUndecodable rejects a source whose real format is outside the set ssg
// processes, before imaging touches it (#198).
//
// The caller selected this file by its extension, and that is not the same
// question: image.Decode dispatches on magic bytes, and importing imaging
// registers the BMP and TIFF decoders too. A crafted TIFF named "photo.png"
// therefore decodes — and CVE-2023-36308 panics in imaging's scanner, with no
// fixed release upstream to upgrade to. The allowlist is the mitigation, so a
// path that skips it has nothing behind it (SEC-013).
func refuseUndecodable(src string) error {
	f, err := os.Open(src) // #nosec G304 -- enumerated from the output tree by avifSources
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Base(src), err)
	}
	defer func() { _ = f.Close() }()

	_, format, err := image.DecodeConfig(f)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Base(src), err)
	}
	if !images.Decodable(format) {
		return fmt.Errorf("%s decodes as %q despite its extension, which ssg does not process",
			filepath.Base(src), format)
	}
	return nil
}
