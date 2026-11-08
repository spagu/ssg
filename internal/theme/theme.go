// Package theme provides theme downloading and management
package theme

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
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

// Download downloads a theme from a URL (GitHub repo or direct archive) and
// extracts it into destDir. A GitHub/GitLab repo URL may pin a tag, branch or
// commit as "repo@ref"; without one the moving main branch is fetched.
// wantSHA256, when set, is the archive's expected SHA-256 (hex): a mismatch
// stops before anything is extracted (SEC-005). Without it the download is
// trusted as served, and a warning says so.
func Download(url, destDir, wantSHA256 string) error {
	// GO-028: the only extractor is ZIP; a tar archive would download fully
	// and then always fail with "not a valid zip file", so reject it up front.
	if strings.HasSuffix(url, ".tar.gz") || strings.HasSuffix(url, ".tgz") {
		return fmt.Errorf("unsupported theme archive format %q: only .zip archives are supported (use a .zip URL or a GitHub/GitLab repo URL)", url)
	}

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

	tmpPath, sum, err := downloadArchive(client, archiveURL)
	if err != nil {
		// GO-040: repos whose default branch is master 404 on the hardcoded
		// main-branch archive URL; retry once with master before giving up.
		fallbackURL := masterFallbackURL(archiveURL)
		if fallbackURL == "" {
			return err
		}
		fmt.Printf("   ⚠️  main branch failed (%v); retrying %s...\n", err, fallbackURL)
		if tmpPath, sum, err = downloadArchive(client, fallbackURL); err != nil {
			return err
		}
	}
	defer func() { _ = os.Remove(tmpPath) }()

	if err := verifySHA256(sum, wantSHA256); err != nil {
		return err
	}

	// Extract
	fmt.Printf("📦 Extracting theme to %s...\n", destDir)
	if err := extractZip(tmpPath, destDir); err != nil {
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

// verifySHA256 compares the archive's SHA-256 with the expected one (SEC-005).
// An empty expectation is allowed and warned about: the theme is then trusted
// as the server delivered it.
func verifySHA256(got, want string) error {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		fmt.Printf("   ⚠️  theme archive not verified (sha256 %s); pin it with online_theme_sha256\n", got)
		return nil
	}
	if got != want {
		return fmt.Errorf("theme archive checksum mismatch: expected sha256 %s, got %s; refusing to extract", want, got)
	}
	fmt.Println("   🔒 theme archive sha256 verified")
	return nil
}

// downloadArchive fetches archiveURL into a temporary file and returns its
// path and SHA-256 (hex); the caller removes the file when done. The temp
// file's Close error is checked because a failed write-side Close can mean
// truncated data (GO-040).
func downloadArchive(client *http.Client, archiveURL string) (string, string, error) {
	resp, err := client.Get(archiveURL) // #nosec G107 -- CLI tool downloads user-specified theme URL
	if err != nil {
		return "", "", fmt.Errorf("downloading theme: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to download theme: HTTP %d", resp.StatusCode)
	}

	// Save to temp file
	tmpFile, err := os.CreateTemp("", "theme-*.zip")
	if err != nil {
		return "", "", fmt.Errorf("creating temp file: %w", err)
	}

	// SEC-006: cap the total downloaded size; +1 lets us detect an overflow.
	// SEC-005: hash while writing, so the archive is read once.
	h := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmpFile, h), io.LimitReader(resp.Body, maxTotalSize+1))
	if cerr := tmpFile.Close(); err == nil {
		err = cerr
	}
	if err == nil && written > maxTotalSize {
		err = fmt.Errorf("theme archive exceeds %d bytes; refusing to extract", maxTotalSize)
	}
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", "", fmt.Errorf("saving theme archive: %w", err)
	}
	return tmpFile.Name(), hex.EncodeToString(h.Sum(nil)), nil
}

// masterFallbackURL returns the master-branch variant of a GitHub/GitLab
// main-branch archive URL, or "" when no branch fallback applies (GO-040).
func masterFallbackURL(archiveURL string) string {
	const (
		githubMain = "/archive/refs/heads/main.zip"
		gitlabMain = "/-/archive/main/archive.zip"
	)
	switch {
	case strings.HasSuffix(archiveURL, githubMain):
		return strings.TrimSuffix(archiveURL, githubMain) + "/archive/refs/heads/master.zip"
	case strings.HasSuffix(archiveURL, gitlabMain):
		return strings.TrimSuffix(archiveURL, gitlabMain) + "/-/archive/master/archive.zip"
	}
	return ""
}

// convertToArchiveURL converts a GitHub/GitLab repo URL to a ZIP archive URL.
// "repo@ref" pins a tag, branch or commit (SEC-005); a bare repo URL follows
// the main branch, with the master fallback of masterFallbackURL.
func convertToArchiveURL(url string) string {
	// Already a direct URL
	if strings.HasSuffix(url, ".zip") || strings.HasSuffix(url, ".tar.gz") {
		return url
	}
	isGitHub, isGitLab := strings.Contains(url, "github.com"), strings.Contains(url, "gitlab.com")
	if !isGitHub && !isGitLab {
		return url
	}
	repo, ref := splitRef(strings.TrimSuffix(url, "/"))
	repo = strings.TrimSuffix(repo, ".git")
	if isGitHub {
		if ref == "" {
			return repo + "/archive/refs/heads/main.zip"
		}
		return repo + "/archive/" + ref + ".zip"
	}
	if ref == "" {
		ref = "main"
	}
	return repo + "/-/archive/" + ref + "/archive.zip"
}

// splitRef splits "https://host/user/repo@ref" into the repo URL and the ref.
// Only the last path segment is looked at, so an "@" elsewhere is left alone.
func splitRef(url string) (repo, ref string) {
	slash := strings.LastIndex(url, "/")
	at := strings.LastIndex(url, "@")
	if at <= slash || at == len(url)-1 {
		return url, ""
	}
	return url[:at], url[at+1:]
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

// commonPrefix returns the top-level directory shared by every archive entry
// so it can be stripped from extracted paths (e.g. "repo-main/"). When the
// entries do not all live under one top-level directory — e.g. a hand-rolled
// ZIP with files at the root — nothing is stripped, so the theme layout is
// preserved instead of being silently flattened (GO-029).
func commonPrefix(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}
	// strings.Split always yields at least one element, so [0] is safe.
	prefix := strings.Split(files[0].Name, "/")[0] + "/"
	for _, f := range files {
		// The bare directory entry itself ("repo-main/") also matches.
		if !strings.HasPrefix(f.Name, prefix) {
			return ""
		}
	}
	return prefix
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
func extractZipEntry(f *zip.File, fpath string, remainingTotal int64) (written int64, err error) {
	if remainingTotal <= 0 {
		return 0, fmt.Errorf("archive exceeds total size limit of %d bytes", maxTotalSize)
	}

	// Extract file. Web content must be world-readable, so 0o644 is intentional.
	// #nosec G304,G302 -- Path is validated by the caller's traversal check; mode is a fixed safe 0o644 (SEC-010)
	outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	// GO-040: a failed Close on the write side can mean truncated data (e.g.
	// full disk); propagate it instead of discarding.
	defer func() {
		if cerr := outFile.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing extracted file %q: %w", fpath, cerr)
		}
	}()

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
func copyFile(src, dst string) (err error) {
	srcFile, err := os.Open(src) // #nosec G304 -- CLI tool copies user's theme files
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst) // #nosec G304 -- CLI tool copies user's theme files
	if err != nil {
		return err
	}
	// GO-040: propagate write-side Close errors (possible truncated data).
	defer func() {
		if cerr := dstFile.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing %q: %w", dst, cerr)
		}
	}()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
