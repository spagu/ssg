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
)

// Download downloads a theme from a URL (GitHub repo or direct archive)
func Download(url, destDir string) error {
	// Convert GitHub repo URL to archive URL
	archiveURL := convertToArchiveURL(url)

	fmt.Printf("📥 Downloading theme from %s...\n", archiveURL)

	// Download archive
	resp, err := http.Get(archiveURL) // #nosec G107 -- CLI tool downloads user-specified theme URL
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

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return fmt.Errorf("saving theme archive: %w", err)
	}

	// Extract
	fmt.Printf("📦 Extracting theme to %s...\n", destDir)
	if err := extractZip(tmpFile.Name(), destDir); err != nil {
		return fmt.Errorf("extracting theme: %w", err)
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

	// Create destination directory
	// #nosec G301 -- Web content directories need to be world-traversable
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	// Find common prefix (first directory in archive)
	var prefix string
	if len(r.File) > 0 {
		parts := strings.Split(r.File[0].Name, "/")
		if len(parts) > 0 {
			prefix = parts[0] + "/"
		}
	}

	for _, f := range r.File {
		// Strip common prefix
		name := strings.TrimPrefix(f.Name, prefix)
		if name == "" {
			continue
		}

		fpath := filepath.Join(dest, name)

		// Security check
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, f.Mode()); err != nil {
				return err
			}
			continue
		}

		// Create parent directories
		// #nosec G301 -- Web content directories need to be world-traversable
		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		// Extract file
		// #nosec G304 -- Path is validated above with security check
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			_ = outFile.Close()
			return err
		}

		// Limit extraction to 100MB per file to prevent zip bombs
		const maxFileSize = 100 * 1024 * 1024 // 100MB
		_, err = io.Copy(outFile, io.LimitReader(rc, maxFileSize))
		_ = rc.Close()
		_ = outFile.Close()

		if err != nil {
			return err
		}
	}

	return nil
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
