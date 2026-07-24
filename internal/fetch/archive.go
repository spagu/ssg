package fetch

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

// Archive extraction limits (decompression-bomb guard, SEC-006). Variables, not
// consts, so tests can lower them without huge fixtures.
var (
	maxArchiveBytes int64 = 200 * 1024 * 1024
	maxArchiveFile  int64 = 100 * 1024 * 1024
	maxArchiveFiles       = 10000
)

// Archive downloads a worker source — a GitHub/GitLab repo URL or a direct .zip
// — with auth, and extracts it into destDir. A repo archive's single top-level
// wrapper directory (repo-branch/) is stripped so destDir holds the project
// itself. Mirrors the theme downloader's hardening (bounded client, size caps,
// safe extraction) with authentication added for private sources (GO-076).
//
// NOTE: internal/theme has a sibling download+extract; unifying the two onto
// this package is a documented DRY follow-up, deferred to avoid touching the
// working theme path in this change.
func Archive(rawURL string, auth Auth, destDir string, opts Options) error {
	if strings.HasSuffix(rawURL, ".tar.gz") || strings.HasSuffix(rawURL, ".tgz") {
		return fmt.Errorf("unsupported worker archive %q: use a .zip URL or a GitHub/GitLab repo URL", rawURL)
	}
	archiveURL := toArchiveURL(rawURL)
	tmp, err := downloadArchive(archiveURL, auth, opts)
	if err != nil {
		if fb := masterFallback(archiveURL); fb != "" {
			tmp, err = downloadArchive(fb, auth, opts)
		}
		if err != nil {
			return err
		}
	}
	defer func() { _ = os.Remove(tmp) }()
	return extractAtomic(tmp, destDir)
}

// extractAtomic extracts src into a sibling staging directory and renames it
// into destDir only after the whole archive extracted cleanly. A mid-extraction
// failure (a tripped size cap, a bad entry) therefore never leaves a
// half-populated destDir — which resolveWorkerDir would otherwise reuse on the
// next build as if it were a complete, vendored worker (GO-081).
func extractAtomic(src, destDir string) error {
	parent := filepath.Dir(destDir)
	if parent == "" {
		parent = "."
	}
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".ssg-worker-*")
	if err != nil {
		return err
	}
	if err := extractZip(src, staging); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}
	if err := os.RemoveAll(destDir); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}
	if err := os.Rename(staging, destDir); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("installing worker into %s: %w", destDir, err)
	}
	return nil
}

// downloadArchive fetches archiveURL (authed, bounded, size-capped) to a temp
// zip and returns its path; the caller removes it. A transient failure is
// retried per opts, matching Bytes.
func downloadArchive(archiveURL string, auth Auth, opts Options) (string, error) {
	attempts := opts.Retries + 1
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		path, retriable, err := downloadOnce(archiveURL, auth, opts.timeout())
		if err == nil {
			return path, nil
		}
		lastErr = err
		if !retriable || attempt == attempts {
			break
		}
		time.Sleep(opts.retryDelay())
	}
	if attempts > 1 {
		return "", fmt.Errorf("after %d attempts: %w", attempts, lastErr)
	}
	return "", lastErr
}

// downloadOnce is a single download attempt. retriable is true for a transport
// error, an HTTP 429/5xx, or a mid-stream read error; false for a 4xx or an
// over-cap archive.
func downloadOnce(archiveURL string, auth Auth, timeout time.Duration) (path string, retriable bool, err error) {
	req, err := http.NewRequest(http.MethodGet, archiveURL, nil)
	if err != nil {
		return "", false, fmt.Errorf("invalid url %q: %w", archiveURL, err)
	}
	if err := auth.apply(req); err != nil {
		return "", false, err
	}
	resp, err := client(auth, timeout).Do(req) // #nosec G107 -- url from the user's own worker config
	if err != nil {
		return "", true, fmt.Errorf("downloading %s: %w", safeURL(archiveURL), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", isRetriableStatus(resp.StatusCode), fmt.Errorf("downloading %s: HTTP %d", safeURL(archiveURL), resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "worker-*.zip")
	if err != nil {
		return "", false, err
	}
	written, copyErr := io.Copy(tmp, io.LimitReader(resp.Body, maxArchiveBytes+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmp.Name())
		return "", true, fmt.Errorf("downloading %s: %w", safeURL(archiveURL), copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp.Name())
		return "", false, closeErr
	}
	if written > maxArchiveBytes {
		_ = os.Remove(tmp.Name())
		return "", false, fmt.Errorf("archive exceeds %d bytes; refusing to extract", maxArchiveBytes)
	}
	return tmp.Name(), false, nil
}

// toArchiveURL turns a GitHub/GitLab repo URL into a main-branch zip URL; a
// direct .zip passes through.
func toArchiveURL(url string) string {
	if strings.HasSuffix(url, ".zip") {
		return url
	}
	url = strings.TrimSuffix(strings.TrimSuffix(url, "/"), ".git")
	switch {
	case strings.Contains(url, "github.com"):
		return url + "/archive/refs/heads/main.zip"
	case strings.Contains(url, "gitlab.com"):
		return url + "/-/archive/main/archive.zip"
	}
	return url
}

// masterFallback returns the master-branch variant of a main-branch archive URL,
// or "" when none applies (repos whose default branch is still master).
func masterFallback(archiveURL string) string {
	const gh = "/archive/refs/heads/main.zip"
	const gl = "/-/archive/main/archive.zip"
	switch {
	case strings.HasSuffix(archiveURL, gh):
		return strings.TrimSuffix(archiveURL, gh) + "/archive/refs/heads/master.zip"
	case strings.HasSuffix(archiveURL, gl):
		return strings.TrimSuffix(archiveURL, gl) + "/-/archive/master/archive.zip"
	}
	return ""
}

// extractZip safely extracts src into destDir: path-escape guarded, size- and
// count-capped. A single top-level wrapper directory (a repo archive's
// repo-branch/) is stripped so destDir holds the project root.
func extractZip(src, destDir string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer func() { _ = zr.Close() }()

	if len(zr.File) > maxArchiveFiles {
		return fmt.Errorf("archive has %d entries, exceeding %d", len(zr.File), maxArchiveFiles)
	}
	strip := singleTopDir(zr.File)
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return err
	}
	cleanDest := filepath.Clean(destDir)
	var total int64
	for _, f := range zr.File {
		name := f.Name
		if strip != "" {
			name = strings.TrimPrefix(name, strip)
		}
		if name == "" {
			continue
		}
		target := filepath.Join(cleanDest, name) // #nosec G305 -- checked below
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("archive entry %q escapes the destination", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}
		// Compare as uint64 (the header's own type): converting a crafted
		// oversize value to int64 could wrap negative and pass the cap.
		// #nosec G115 -- maxArchiveFile is our own positive constant, not attacker input
		if f.UncompressedSize64 > uint64(maxArchiveFile) {
			return fmt.Errorf("archive entry %q exceeds %d bytes", f.Name, maxArchiveFile)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		n, err := writeZipEntry(f, target, maxArchiveBytes-total)
		if err != nil {
			return err
		}
		total += n
	}
	return nil
}

// writeZipEntry copies one archive entry to target, capped at remaining bytes.
func writeZipEntry(f *zip.File, target string, remaining int64) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer func() { _ = rc.Close() }()
	// #nosec G304 -- target is validated to stay within destDir above
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, io.LimitReader(rc, remaining+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > remaining {
		err = fmt.Errorf("archive total exceeds %d bytes", maxArchiveBytes)
	}
	return n, err
}

// singleTopDir returns the sole top-level directory prefix ("repo-main/") shared
// by every entry, or "" when entries do not share one. Repo archives wrap the
// project in exactly such a directory; a hand-made worker zip usually does not.
func singleTopDir(files []*zip.File) string {
	var top string
	for _, f := range files {
		i := strings.IndexByte(f.Name, '/')
		if i < 0 {
			return "" // a top-level file means no single wrapper dir
		}
		dir := f.Name[:i+1]
		// A ".." or "." segment is never a legitimate wrapper; leaving it in
		// place lets the extract loop's escape guard reject it (SEC).
		if dir == "../" || dir == "./" {
			return ""
		}
		if top == "" {
			top = dir
		} else if dir != top {
			return ""
		}
	}
	return top
}
