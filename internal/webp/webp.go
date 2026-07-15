// Package webp provides WebP image conversion using the cwebp command-line tool.
package webp

import (
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder for image.DecodeConfig
	_ "image/png"  // register PNG decoder for image.DecodeConfig
	"io"
	"os"
	"os/exec"
	"path"
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
	// KeepOriginal emits the .webp NEXT TO the original instead of replacing
	// it, so themes with hardcoded .png/.jpg references (favicons, logos,
	// og:image) keep working while <img> src references are still rewritten
	// to .webp (GO-052). Default false preserves the historical
	// replace-in-place behaviour.
	KeepOriginal bool
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
			webpPath := webpTargetPath(path)
			// If WebP already exists, skip conversion (unless Force reconvert);
			// in replace mode the leftover original is deleted, in keep mode it stays.
			if !opts.Force {
				if _, statErr := os.Stat(webpPath); statErr == nil {
					if !opts.KeepOriginal {
						_ = os.Remove(path) // #nosec G122 -- CLI tool operates on user's output files
					}
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

		originalSize := info.Size()
		webpPath := webpTargetPath(path)

		if !opts.Quiet {
			fmt.Printf("   🖼️  Converting %d/%d: %s\n", i+1, total, filepath.Base(path))
		}

		if convErr := convertImage(path, webpPath, opts.Quality); convErr != nil {
			if !opts.Quiet {
				fmt.Printf("   ⚠️  Failed to convert %s: %v\n", filepath.Base(path), convErr)
			}
			continue
		}

		// Get new size; this also confirms the .webp actually exists before the
		// original is deleted below (GO-016).
		newInfo, statErr := os.Stat(webpPath)
		if statErr != nil {
			continue // output missing despite reported success — keep the original
		}
		savedBytes += originalSize - newInfo.Size()

		// Responsive variants: derive smaller widths from the original before it is
		// removed, so quality is best and no upscaling occurs (ASSET-004).
		if len(opts.Sizes) > 0 {
			generateResponsiveVariants(path, webpPath, opts)
		}

		// Replace mode removes the original; keep mode leaves it next to the
		// .webp so hardcoded extension references stay valid (GO-052).
		if !opts.KeepOriginal {
			if rmErr := os.Remove(path); rmErr != nil && !opts.Quiet {
				fmt.Printf("   ⚠️  Failed to remove original %s: %v\n", filepath.Base(path), rmErr)
			}
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

// webpTargetPath maps an image path to its .webp sibling by stripping the
// original extension by length; a case-sensitive TrimSuffix on a lowercased
// extension would turn "Photo.JPG" into "Photo.JPG.webp" (GO-016).
func webpTargetPath(imgPath string) string {
	return imgPath[:len(imgPath)-len(filepath.Ext(imgPath))] + ".webp"
}

// variantPath returns the responsive-variant filename for a base WebP path and
// width, e.g. ("img/foo.webp", 480) → "img/foo-480.webp" (ASSET-004).
func variantPath(webpPath string, width int) string {
	return strings.TrimSuffix(webpPath, ".webp") + fmt.Sprintf("-%d.webp", width)
}

// imageWidth reads an image's pixel width without fully decoding it. Formats
// unknown to the stdlib (i.e. converted .webp originals) fall back to a
// container-header parse (GO-032).
func imageWidth(path string) (int, bool) {
	f, err := os.Open(path) // #nosec G304 -- CLI tool reads user's output files
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()
	if cfg, _, decErr := image.DecodeConfig(f); decErr == nil {
		return cfg.Width, true
	}
	// Not a stdlib-decodable format — rewind and retry as a WebP container.
	// A failed Seek simply makes webpWidth report false.
	_, _ = f.Seek(0, io.SeekStart)
	return webpWidth(f)
}

// webpWidth parses a WebP (RIFF) container header for the pixel width without a
// decoder dependency — the stdlib image package has no WebP support and
// golang.org/x/image/webp is intentionally not added to go.mod (GO-032).
// Lossy (VP8), lossless (VP8L) and extended (VP8X) layouts are supported.
func webpWidth(r io.Reader) (int, bool) {
	buf := make([]byte, 30)
	n, err := io.ReadAtLeast(r, buf, 25)
	if err != nil {
		return 0, false
	}
	if string(buf[0:4]) != "RIFF" || string(buf[8:12]) != "WEBP" {
		return 0, false
	}
	switch string(buf[12:16]) {
	case "VP8X": // extended: 24-bit little-endian canvas width minus one at payload offset 4
		if n < 27 {
			return 0, false
		}
		return (int(buf[24]) | int(buf[25])<<8 | int(buf[26])<<16) + 1, true
	case "VP8 ": // lossy: sync code 0x9D012A, then 14-bit little-endian width
		if n < 28 || buf[23] != 0x9D || buf[24] != 0x01 || buf[25] != 0x2A {
			return 0, false
		}
		return (int(buf[26]) | int(buf[27])<<8) & 0x3FFF, true
	case "VP8L": // lossless: signature 0x2F, then 14-bit width minus one
		if buf[20] != 0x2F {
			return 0, false
		}
		bits := uint32(buf[21]) | uint32(buf[22])<<8 | uint32(buf[23])<<16 | uint32(buf[24])<<24
		return int(bits&0x3FFF) + 1, true
	}
	return 0, false
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
// imgSrcRe requires start-of-tag or a separator before "src" so prefixed
// attributes like lazy-load data-src are never matched (GO-038).
var imgTagRe = regexp.MustCompile(`<img\b[^>]*>`)
var imgSrcRe = regexp.MustCompile(`(?:^|[\s"'>])src\s*=\s*"([^"]+\.webp)"`)

// srcsetContext carries per-walk state for srcset emission: the output root,
// configured widths, the sizes attribute and memoized file-existence/width
// lookups so each variant is stat'ed and each original decoded at most once per
// build, however many pages reference it (PERF-011).
type srcsetContext struct {
	root      string
	sizes     []int
	sizesAttr string
	exists    func(string) bool
	width     func(string) (int, bool)
}

// newExistsCache returns a memoized file-existence check (PERF-011).
func newExistsCache() func(string) bool {
	cache := map[string]bool{}
	return func(file string) bool {
		if hit, ok := cache[file]; ok {
			return hit
		}
		_, err := os.Stat(file)
		cache[file] = err == nil
		return cache[file]
	}
}

// newWidthCache returns a memoized image-width reader (GO-032, PERF-011).
func newWidthCache() func(string) (int, bool) {
	type dims struct {
		width int
		ok    bool
	}
	cache := map[string]dims{}
	return func(file string) (int, bool) {
		if hit, ok := cache[file]; ok {
			return hit.width, hit.ok
		}
		w, ok := imageWidth(file)
		cache[file] = dims{width: w, ok: ok}
		return w, ok
	}
}

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
	ctx := &srcsetContext{
		root:      dir,
		sizes:     sorted,
		sizesAttr: sizesAttr,
		exists:    newExistsCache(),
		width:     newWidthCache(),
	}

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.ToLower(filepath.Ext(path)) != ".html" {
			return err
		}
		content, e := os.ReadFile(path) // #nosec G304,G122 -- CLI reads its own output; path from local Walk
		if e != nil {
			return e
		}
		out := imgTagRe.ReplaceAllStringFunc(string(content), func(tag string) string {
			return ctx.rewriteImgTag(tag, filepath.Dir(path))
		})
		if out == string(content) {
			return nil
		}
		return os.WriteFile(path, []byte(out), info.Mode()) // #nosec G306,G703,G122 -- CLI writes its own output; path from local Walk
	})
}

// resolveFile maps a page-relative or root-absolute URL to a file path.
func (c *srcsetContext) resolveFile(url, htmlDir string) string {
	if strings.HasPrefix(url, "/") {
		return filepath.Join(c.root, filepath.FromSlash(strings.TrimPrefix(url, "/")))
	}
	return filepath.Join(htmlDir, filepath.FromSlash(url))
}

// rewriteImgTag injects srcset/sizes into a single <img> tag when variants exist.
func (c *srcsetContext) rewriteImgTag(tag, htmlDir string) string {
	if strings.Contains(tag, "srcset=") {
		return tag
	}
	m := imgSrcRe.FindStringSubmatch(tag)
	if m == nil {
		return tag
	}
	src := m[1]
	var parts []string
	largest := 0
	for _, w := range c.sizes { // ascending, so largest tracks the widest variant
		variantURL := variantPath(src, w)
		if c.exists(c.resolveFile(variantURL, htmlDir)) {
			parts = append(parts, fmt.Sprintf("%s %dw", variantURL, w))
			largest = w
		}
	}
	if len(parts) == 0 {
		return tag
	}
	// Append the full-size original with its real pixel width: with w
	// descriptors browsers ignore the src attribute, so without this entry
	// desktops would upscale the largest downsized variant (GO-032). When the
	// width cannot be determined (e.g. an unparsable header), the original is
	// skipped rather than guessed.
	if w, ok := c.width(c.resolveFile(src, htmlDir)); ok && w > largest {
		parts = append(parts, fmt.Sprintf("%s %dw", src, w))
	}
	inject := fmt.Sprintf(` srcset="%s" sizes="%s"`, strings.Join(parts, ", "), c.sizesAttr)
	// Keep self-closing tags valid: insert before "/>" instead of before ">" (GO-038).
	if strings.HasSuffix(tag, "/>") {
		return strings.TrimRight(strings.TrimSuffix(tag, "/>"), " ") + inject + " />"
	}
	return strings.TrimSuffix(tag, ">") + inject + ">"
}

// imageRefAttrRe captures src/srcset/href attribute values (double- or single-
// quoted). The leading separator class scopes rewriting to real attributes, so
// prose text and prefixed attributes like data-src are never touched (GO-017).
var imageRefAttrRe = regexp.MustCompile(`(?i)(?:^|[\s"'>])(?:src|srcset|href)\s*=\s*("[^"]*"|'[^']*')`)

// cssURLRe captures url(...) references in stylesheets and inline <style> blocks (GO-017).
var cssURLRe = regexp.MustCompile(`(?i)url\(\s*("[^"]*"|'[^']*'|[^'")]+)\)`)

// collectWebpSet walks dir and records every existing .webp file by cleaned
// path, so reference rewriting can verify a conversion actually succeeded (GO-017).
func collectWebpSet(dir string) (map[string]bool, error) {
	set := map[string]bool{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.EqualFold(filepath.Ext(path), ".webp") {
			set[filepath.Clean(path)] = true
		}
		return nil
	})
	return set, err
}

// isRemoteURL reports whether ref points at another host (http://, https:// or
// protocol-relative //) and therefore must never be rewritten (GO-017).
func isRemoteURL(ref string) bool {
	lower := strings.ToLower(ref)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(ref, "//")
}

// rewriteLocalImageURL maps a single local .jpg/.jpeg/.png reference — matched
// case-insensitively so Photo.JPG is rewritten too (GO-016) — to .webp, but only
// when the converted file exists on disk (GO-017). baseDir resolves relative
// references (the referencing file's directory); root resolves "/"-absolute ones.
func rewriteLocalImageURL(ref, baseDir, root string, webpSet map[string]bool) string {
	if ref == "" || isRemoteURL(ref) {
		return ref
	}
	ext := strings.ToLower(path.Ext(ref))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return ref
	}
	webpRef := ref[:len(ref)-len(ext)] + ".webp"
	var file string
	if strings.HasPrefix(webpRef, "/") {
		file = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(webpRef, "/")))
	} else {
		file = filepath.Join(baseDir, filepath.FromSlash(webpRef))
	}
	if !webpSet[filepath.Clean(file)] {
		return ref // conversion missing or failed — keep the working reference
	}
	return webpRef
}

// rewriteRefList rewrites each URL of a (possibly comma-separated, srcset-style)
// attribute value, preserving width/density descriptors and spacing.
func rewriteRefList(value, baseDir, root string, webpSet map[string]bool) string {
	parts := strings.Split(value, ",")
	for i, part := range parts {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		if newURL := rewriteLocalImageURL(fields[0], baseDir, root, webpSet); newURL != fields[0] {
			parts[i] = strings.Replace(part, fields[0], newURL, 1)
		}
	}
	return strings.Join(parts, ",")
}

// rewriteImageRefs rewrites image references inside src/srcset/href attributes
// and CSS url(...) only; everything else — prose, remote URLs, scripts — is left
// untouched (GO-017).
func rewriteImageRefs(content, baseDir, root string, webpSet map[string]bool) string {
	out := imageRefAttrRe.ReplaceAllStringFunc(content, func(m string) string {
		quoted := imageRefAttrRe.FindStringSubmatch(m)[1]
		inner := quoted[1 : len(quoted)-1]
		rewritten := rewriteRefList(inner, baseDir, root, webpSet)
		if rewritten == inner {
			return m
		}
		return strings.Replace(m, quoted, quoted[:1]+rewritten+quoted[len(quoted)-1:], 1)
	})
	return cssURLRe.ReplaceAllStringFunc(out, func(m string) string {
		raw := cssURLRe.FindStringSubmatch(m)[1]
		quote, inner := "", raw
		if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') {
			quote, inner = string(raw[0]), raw[1:len(raw)-1]
		}
		inner = strings.TrimSpace(inner)
		rewritten := rewriteLocalImageURL(inner, baseDir, root, webpSet)
		if rewritten == inner {
			return m
		}
		return "url(" + quote + rewritten + quote + ")"
	})
}

// UpdateReferences updates image references in HTML/CSS files. Only references
// whose converted .webp target exists are rewritten, remote URLs and prose are
// left alone (GO-017), and extensions are matched case-insensitively (GO-016).
func UpdateReferences(dir string) error {
	webpSet, err := collectWebpSet(dir)
	if err != nil {
		return err
	}

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Lowercased so .HTML/.CSS files are processed too (GO-017).
		ext := strings.ToLower(filepath.Ext(path))
		if info.IsDir() || (ext != ".html" && ext != ".css") {
			return nil
		}

		content, err := os.ReadFile(path) // #nosec G304,G122 -- CLI tool reads user's output files
		if err != nil {
			return err
		}

		newContent := rewriteImageRefs(string(content), filepath.Dir(path), dir, webpSet)
		if newContent != string(content) {
			return os.WriteFile(path, []byte(newContent), info.Mode()) // #nosec G703,G122 -- CLI tool writes user's output files
		}
		return nil
	})
}
