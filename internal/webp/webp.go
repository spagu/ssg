// Package webp provides WebP image conversion using the cwebp command-line tool.
package webp

import (
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder for image.DecodeConfig
	_ "image/png"  // register PNG decoder for image.DecodeConfig
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ConvertOptions holds WebP conversion options
type ConvertOptions struct {
	Quality int   // 1-100, default 60
	Quiet   bool  // Suppress output
	Force   bool  // Force reconversion even if WebP exists
	Sizes   []int // Responsive width presets (px); empty = single size (ASSET-004)
}

// ConvertDirectory converts all JPG/PNG images in a directory to WebP
func ConvertDirectory(dir string, opts ConvertOptions) (converted int, savedBytes int64, err error) {
	// Check if cwebp is available
	if _, err := exec.LookPath("cwebp"); err != nil {
		return 0, 0, fmt.Errorf("cwebp tool not found: please install 'webp' package")
	}

	if opts.Quality <= 0 || opts.Quality > 100 {
		opts.Quality = 60
	}

	// First pass: collect images that need conversion, delete originals if webp exists
	var imagePaths []string
	var skipped int
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			webpPath := strings.TrimSuffix(path, ext) + ".webp"
			// If WebP already exists, just delete the original (unless Force reconvert)
			if !opts.Force {
				if _, statErr := os.Stat(webpPath); statErr == nil {
					// WebP exists - delete the original jpg/png from output
					_ = os.Remove(path) // #nosec G122 -- CLI tool operates on user's output files
					skipped++
					return nil
				}
			}
			imagePaths = append(imagePaths, path)
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	total := len(imagePaths)
	if total == 0 {
		if skipped > 0 && !opts.Quiet {
			fmt.Printf("🖼️  WebP: all %d images already converted, skipping\n", skipped)
		}
		return 0, 0, nil
	}

	// Print header only when there are images to convert
	if !opts.Quiet {
		fmt.Printf("🖼️  Converting %d images to WebP (quality: %d)...\n", total, opts.Quality)
		if skipped > 0 {
			fmt.Printf("   ⏭️  Skipping %d images (WebP already exists)\n", skipped)
		}
	}

	// Second pass: convert with progress
	for i, path := range imagePaths {
		info, statErr := os.Stat(path)
		if statErr != nil {
			continue
		}

		ext := strings.ToLower(filepath.Ext(path))
		originalSize := info.Size()
		webpPath := strings.TrimSuffix(path, ext) + ".webp"

		if !opts.Quiet {
			fmt.Printf("   🖼️  Converting %d/%d: %s\n", i+1, total, filepath.Base(path))
		}

		if convErr := convertImage(path, webpPath, opts.Quality); convErr != nil {
			if !opts.Quiet {
				fmt.Printf("   ⚠️  Failed to convert %s: %v\n", filepath.Base(path), convErr)
			}
			continue
		}

		// Get new size
		if newInfo, statErr := os.Stat(webpPath); statErr == nil {
			savedBytes += originalSize - newInfo.Size()
		}

		// Responsive variants: derive smaller widths from the original before it is
		// removed, so quality is best and no upscaling occurs (ASSET-004).
		if len(opts.Sizes) > 0 {
			generateResponsiveVariants(path, webpPath, opts)
		}

		// Remove original
		if rmErr := os.Remove(path); rmErr != nil && !opts.Quiet {
			fmt.Printf("   ⚠️  Failed to remove original %s: %v\n", filepath.Base(path), rmErr)
		}

		converted++
	}

	return converted, savedBytes, nil
}

// safeArgPath guards against argument injection into cwebp: a filename that
// begins with '-' (e.g. "-o.png") would otherwise be parsed as a cwebp flag.
// Relative paths are prefixed with "./" so they are unambiguously paths, never
// options. Absolute paths and paths already starting with "." are unchanged
// (SEC-011).
func safeArgPath(p string) string {
	if p == "" || filepath.IsAbs(p) || strings.HasPrefix(p, ".") {
		return p
	}
	return "./" + p
}

// convertImage converts a single image to WebP using cwebp.
//
// cwebp is an optional, system-installed dependency, so it must be resolved
// from PATH (an absolute path is not portable). Its availability is verified up
// front in ConvertDirectory via exec.LookPath, and the only variable arguments
// are image paths, which safeArgPath hardens against flag injection (SEC-011).
// The PATH-lookup sensitivity (Sonar S4036 / gosec G204) is therefore reviewed
// and accepted here.
func convertImage(srcPath, dstPath string, quality int) error {
	// #nosec G204 -- fixed external tool (cwebp); only path args vary, hardened by safeArgPath
	cmd := exec.Command("cwebp", "-q", strconv.Itoa(quality), safeArgPath(srcPath), "-o", safeArgPath(dstPath)) // NOSONAR S4036: cwebp is intentionally resolved from PATH
	// Suppress cwebp output unless error
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cwebp failed: %v, output: %s", err, string(output))
	}
	return nil
}

// variantPath returns the responsive-variant filename for a base WebP path and
// width, e.g. ("img/foo.webp", 480) → "img/foo-480.webp" (ASSET-004).
func variantPath(webpPath string, width int) string {
	return strings.TrimSuffix(webpPath, ".webp") + fmt.Sprintf("-%d.webp", width)
}

// imageWidth reads an image's pixel width without fully decoding it.
func imageWidth(path string) (int, bool) {
	f, err := os.Open(path) // #nosec G304 -- CLI tool reads user's output files
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, false
	}
	return cfg.Width, true
}

// generateResponsiveVariants emits one downsized WebP per configured width that is
// smaller than the original (no upscaling), derived from the original image via
// cwebp -resize (ASSET-004). Failures are non-fatal per variant.
func generateResponsiveVariants(srcPath, webpPath string, opts ConvertOptions) {
	origWidth, ok := imageWidth(srcPath)
	if !ok {
		return
	}
	for _, w := range opts.Sizes {
		if w <= 0 || w >= origWidth {
			continue // skip upscaling and non-positive widths
		}
		dst := variantPath(webpPath, w)
		if convErr := convertImageResized(srcPath, dst, opts.Quality, w); convErr != nil && !opts.Quiet {
			fmt.Printf("   ⚠️  Failed to make %dw variant of %s: %v\n", w, filepath.Base(srcPath), convErr)
		}
	}
}

// convertImageResized converts an image to WebP at a target width (height auto),
// hardened against argument injection like convertImage (SEC-011).
func convertImageResized(srcPath, dstPath string, quality, width int) error {
	// #nosec G204 -- fixed external tool (cwebp); only path/size args vary, paths hardened by safeArgPath
	cmd := exec.Command("cwebp", "-q", strconv.Itoa(quality), // NOSONAR S4036: cwebp is intentionally resolved from PATH
		"-resize", strconv.Itoa(width), "0",
		safeArgPath(srcPath), "-o", safeArgPath(dstPath))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cwebp resize failed: %v, output: %s", err, string(output))
	}
	return nil
}

// imgTagRe / imgSrcRe locate <img> tags and their WebP src for srcset emission.
var imgTagRe = regexp.MustCompile(`<img\b[^>]*>`)
var imgSrcRe = regexp.MustCompile(`\bsrc="([^"]+\.webp)"`)

// EmitSrcset adds srcset/sizes to <img> tags whose WebP source has responsive
// variants on disk (ASSET-004). Tags already carrying srcset are left untouched;
// the original src remains the fallback. Variant URLs are resolved relative to the
// HTML file (absolute-from-root when the src begins with "/").
func EmitSrcset(dir string, sizes []int, sizesAttr string) error {
	if len(sizes) == 0 {
		return nil
	}
	if sizesAttr == "" {
		sizesAttr = "100vw"
	}
	sorted := append([]int(nil), sizes...)
	sort.Ints(sorted)

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.ToLower(filepath.Ext(path)) != ".html" {
			return err
		}
		content, e := os.ReadFile(path) // #nosec G304,G122 -- CLI reads its own output; path from local Walk
		if e != nil {
			return e
		}
		out := imgTagRe.ReplaceAllStringFunc(string(content), func(tag string) string {
			return rewriteImgTag(tag, filepath.Dir(path), dir, sorted, sizesAttr)
		})
		if out == string(content) {
			return nil
		}
		return os.WriteFile(path, []byte(out), info.Mode()) // #nosec G306,G703,G122 -- CLI writes its own output; path from local Walk
	})
}

// rewriteImgTag injects srcset/sizes into a single <img> tag when variants exist.
func rewriteImgTag(tag, htmlDir, root string, sizes []int, sizesAttr string) string {
	if strings.Contains(tag, "srcset=") {
		return tag
	}
	m := imgSrcRe.FindStringSubmatch(tag)
	if m == nil {
		return tag
	}
	src := m[1]
	var parts []string
	for _, w := range sizes {
		variantURL := variantPath(src, w)
		var variantFile string
		if strings.HasPrefix(variantURL, "/") {
			variantFile = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(variantURL, "/")))
		} else {
			variantFile = filepath.Join(htmlDir, filepath.FromSlash(variantURL))
		}
		if _, err := os.Stat(variantFile); err == nil {
			parts = append(parts, fmt.Sprintf("%s %dw", variantURL, w))
		}
	}
	if len(parts) == 0 {
		return tag
	}
	inject := fmt.Sprintf(` srcset="%s" sizes="%s"`, strings.Join(parts, ", "), sizesAttr)
	return strings.TrimSuffix(tag, ">") + inject + ">"
}

// UpdateReferences updates image references in HTML/CSS files
func UpdateReferences(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		ext := filepath.Ext(path)
		if info.IsDir() || (ext != ".html" && ext != ".css") {
			return nil
		}

		content, err := os.ReadFile(path) // #nosec G304,G122 -- CLI tool reads user's output files
		if err != nil {
			return err
		}

		newContent := string(content)
		// Replace in quotes
		newContent = strings.ReplaceAll(newContent, ".jpg\"", ".webp\"")
		newContent = strings.ReplaceAll(newContent, ".jpeg\"", ".webp\"")
		newContent = strings.ReplaceAll(newContent, ".png\"", ".webp\"")
		newContent = strings.ReplaceAll(newContent, ".jpg'", ".webp'")
		newContent = strings.ReplaceAll(newContent, ".jpeg'", ".webp'")
		newContent = strings.ReplaceAll(newContent, ".png'", ".webp'")
		// CSS url()
		newContent = strings.ReplaceAll(newContent, ".jpg)", ".webp)")
		newContent = strings.ReplaceAll(newContent, ".jpeg)", ".webp)")
		newContent = strings.ReplaceAll(newContent, ".png)", ".webp)")
		// srcset with space
		newContent = strings.ReplaceAll(newContent, ".jpg ", ".webp ")
		newContent = strings.ReplaceAll(newContent, ".jpeg ", ".webp ")
		newContent = strings.ReplaceAll(newContent, ".png ", ".webp ")

		if newContent != string(content) {
			return os.WriteFile(path, []byte(newContent), info.Mode()) // #nosec G703,G122 -- CLI tool writes user's output files
		}
		return nil
	})
}
