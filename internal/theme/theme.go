// Package theme provides theme downloading and management
package theme

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	downloadTimeout = 30 * time.Second
	maxRedirects    = 5
)

// Download and extraction limits guard against slow/oversized downloads
// (SEC-008) and decompression bombs (SEC-006). They are package variables (not
// consts) so tests can lower them to exercise the caps without huge fixtures.
var (
	maxTotalSize int64 = 500 * 1024 * 1024 // cumulative downloaded/extracted bytes
	maxFileSize  int64 = 100 * 1024 * 1024 // per archive entry
	maxEntries         = 10000             // archive entry count
)

// Download downloads a theme from a URL (GitHub repo or direct archive)
func Download(url, destDir string) error {
	// Convert GitHub repo URL to archive URL
	archiveURL := convertToArchiveURL(url)

	fmt.Printf("📥 Downloading theme from %s...\n", archiveURL)

	// SEC-008: use a bounded client (timeout + redirect cap) instead of
	// http.DefaultClient, which has no timeout and follows redirects freely.
	client := &http.Client{
		Timeout: downloadTimeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}
	resp, err := client.Get(archiveURL) // #nosec G107 -- CLI tool downloads user-specified theme URL
	if err != nil {
		return fmt.Errorf("downloading theme: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download theme: HTTP %d", resp.StatusCode)
	}

	// Save to temp file
	tmpFile, err := os.CreateTemp("", "theme-*.zip")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	// SEC-006: cap the total downloaded size; +1 lets us detect an overflow.
	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxTotalSize+1))
	if err != nil {
		return fmt.Errorf("saving theme archive: %w", err)
	}
	if written > maxTotalSize {
		return fmt.Errorf("theme archive exceeds %d bytes; refusing to extract", maxTotalSize)
	}

	// Extract
	fmt.Printf("📦 Extracting theme to %s...\n", destDir)
	if err := extractZip(tmpFile.Name(), destDir); err != nil {
		return fmt.Errorf("extracting theme: %w", err)
	}

	// A downloaded Hugo theme uses layouts/ + static/ + assets/ which the SSG
	// generator does not understand natively; best-effort convert that structure
	// into the flat SSG layout so the theme is at least usable (GO-010).
	if _, err := os.Stat(filepath.Join(destDir, "layouts")); err == nil {
		if err := ConvertHugoTheme(destDir, destDir); err != nil {
			fmt.Printf("   ⚠️  Hugo theme conversion failed: %v\n", err)
		}
	}

	fmt.Println("✅ Theme downloaded successfully")
	return nil
}

// convertToArchiveURL converts a GitHub repo URL to a ZIP archive URL
func convertToArchiveURL(url string) string {
	// Already a direct URL
	if strings.HasSuffix(url, ".zip") || strings.HasSuffix(url, ".tar.gz") {
		return url
	}

	// GitHub repo URL: https://github.com/user/repo
	if strings.Contains(url, "github.com") {
		// Remove trailing slash
		url = strings.TrimSuffix(url, "/")
		// Remove .git suffix
		url = strings.TrimSuffix(url, ".git")
		// Convert to archive URL (main branch)
		return url + "/archive/refs/heads/main.zip"
	}

	// GitLab repo URL
	if strings.Contains(url, "gitlab.com") {
		url = strings.TrimSuffix(url, "/")
		url = strings.TrimSuffix(url, ".git")
		return url + "/-/archive/main/archive.zip"
	}

	return url
}

// extractZip extracts a ZIP archive to destination directory
func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	// SEC-006: reject archives with an implausible number of entries.
	if len(r.File) > maxEntries {
		return fmt.Errorf("archive has too many entries (%d > %d)", len(r.File), maxEntries)
	}

	// Create destination directory
	// #nosec G301 -- Web content directories need to be world-traversable
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	prefix := commonPrefix(r.File)

	var written int64 // SEC-006: cumulative extracted bytes across all entries
	for _, f := range r.File {
		n, err := extractOneEntry(f, dest, prefix, maxTotalSize-written)
		if err != nil {
			return err
		}
		written += n
	}

	return nil
}

// commonPrefix returns the top-level directory shared by the archive so it can
// be stripped from extracted paths (e.g. "repo-main/").
func commonPrefix(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}
	// strings.Split always yields at least one element, so [0] is safe.
	return strings.Split(files[0].Name, "/")[0] + "/"
}

// extractOneEntry resolves, validates and writes a single archive entry.
// Directories create a fixed-mode dir and return 0 bytes; the returned byte
// count feeds the caller's cumulative size cap (SEC-006).
func extractOneEntry(f *zip.File, dest, prefix string, remainingTotal int64) (int64, error) {
	// Strip common prefix
	name := strings.TrimPrefix(f.Name, prefix)
	if name == "" {
		return 0, nil
	}

	fpath := filepath.Join(dest, name)

	// Security check: reject path traversal outside dest.
	if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
		return 0, fmt.Errorf("invalid file path: %s", fpath)
	}

	if f.FileInfo().IsDir() {
		// SEC-010: fixed safe mode, never trust the archive's f.Mode().
		// #nosec G301 -- Web content directories need to be world-traversable
		return 0, os.MkdirAll(fpath, 0o755)
	}

	// Create parent directories
	// #nosec G301 -- Web content directories need to be world-traversable
	if err := os.MkdirAll(filepath.Dir(fpath), 0o755); err != nil {
		return 0, err
	}

	return extractZipEntry(f, fpath, remainingTotal)
}

// extractZipEntry writes a single archive entry to fpath with a fixed 0o644 mode
// (SEC-010) and enforces the per-file and remaining-total size caps (SEC-006).
// remainingTotal is the extraction budget left before the cumulative limit.
func extractZipEntry(f *zip.File, fpath string, remainingTotal int64) (int64, error) {
	if remainingTotal <= 0 {
		return 0, fmt.Errorf("archive exceeds total size limit of %d bytes", maxTotalSize)
	}

	// Extract file. Web content must be world-readable, so 0o644 is intentional.
	// #nosec G304,G302 -- Path is validated by the caller's traversal check; mode is a fixed safe 0o644 (SEC-010)
	outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	defer func() { _ = outFile.Close() }()

	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer func() { _ = rc.Close() }()

	// Cap this entry by both the per-file limit and the remaining total budget;
	// +1 lets us detect an entry that blows past the cap (zip bomb).
	limit := maxFileSize
	if remainingTotal < limit {
		limit = remainingTotal
	}
	n, err := io.Copy(outFile, io.LimitReader(rc, limit+1))
	if err != nil {
		return n, err
	}
	if n > limit {
		return n, fmt.Errorf("archive entry %q exceeds size limit (possible zip bomb)", f.Name)
	}
	return n, nil
}

// ConvertHugoTheme converts a Hugo theme structure to SSG format
func ConvertHugoTheme(themeDir, outputDir string) error {
	fmt.Printf("🔄 Converting Hugo theme to SSG format...\n")

	// Hugo theme structure:
	// - layouts/ -> templates/
	// - static/ -> css/, js/, images/
	// - assets/ -> css/, js/

	// Create output directory
	// #nosec G301 -- Web content directories need to be world-traversable
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	// Copy layouts as templates
	layoutsDir := filepath.Join(themeDir, "layouts")
	if _, err := os.Stat(layoutsDir); err == nil {
		if err := copyDir(layoutsDir, outputDir); err != nil {
			return fmt.Errorf("copying layouts: %w", err)
		}
	}

	// Copy static assets
	staticDir := filepath.Join(themeDir, "static")
	if _, err := os.Stat(staticDir); err == nil {
		if err := copyDir(staticDir, outputDir); err != nil {
			return fmt.Errorf("copying static: %w", err)
		}
	}

	// Copy assets
	assetsDir := filepath.Join(themeDir, "assets")
	if _, err := os.Stat(assetsDir); err == nil {
		if err := copyDir(assetsDir, outputDir); err != nil {
			return fmt.Errorf("copying assets: %w", err)
		}
	}

	fmt.Println("✅ Hugo theme converted")
	return nil
}

// copyDir copies a directory recursively
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath)
	})
}

// copyFile copies a single file
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src) // #nosec G304 -- CLI tool copies user's theme files
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst) // #nosec G304 -- CLI tool copies user's theme files
	if err != nil {
		return err
	}
	defer func() { _ = dstFile.Close() }()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
