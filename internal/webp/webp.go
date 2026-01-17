// Package webp provides WebP image conversion using the cwebp command-line tool.
package webp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ConvertOptions holds WebP conversion options
type ConvertOptions struct {
	Quality int  // 1-100, default 60
	Quiet   bool // Suppress output
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

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			return nil
		}

		originalSize := info.Size()
		webpPath := strings.TrimSuffix(path, ext) + ".webp"

		if convErr := convertImage(path, webpPath, opts.Quality); convErr != nil {
			if !opts.Quiet {
				fmt.Printf("   ⚠️  Failed to convert %s: %v\n", filepath.Base(path), convErr)
			}
			return nil // Continue with other files
		}

		// Get new size
		if newInfo, statErr := os.Stat(webpPath); statErr == nil {
			savedBytes += originalSize - newInfo.Size()
		}

		// Remove original
		if rmErr := os.Remove(path); rmErr != nil && !opts.Quiet {
			fmt.Printf("   ⚠️  Failed to remove original %s: %v\n", filepath.Base(path), rmErr)
		}

		converted++
		return nil
	})

	return converted, savedBytes, err
}

// convertImage converts a single image to WebP using cwebp
func convertImage(srcPath, dstPath string, quality int) error {
	cmd := exec.Command("cwebp", "-q", strconv.Itoa(quality), srcPath, "-o", dstPath)
	// Suppress cwebp output unless error
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cwebp failed: %v, output: %s", err, string(output))
	}
	return nil
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

		content, err := os.ReadFile(path)
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
			return os.WriteFile(path, []byte(newContent), info.Mode())
		}
		return nil
	})
}
