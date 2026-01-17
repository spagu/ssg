// Package webp provides native Go WebP image conversion
package webp

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/chai2010/webp"
)

// ConvertOptions holds WebP conversion options
type ConvertOptions struct {
	Quality int  // 1-100, default 60
	Quiet   bool // Suppress output
}

// ConvertDirectory converts all JPG/PNG images in a directory to WebP
func ConvertDirectory(dir string, opts ConvertOptions) (converted int, savedBytes int64, err error) {
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

		if convErr := convertImage(path, webpPath, ext, opts.Quality); convErr != nil {
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

// convertImage converts a single image to WebP
func convertImage(srcPath, dstPath, ext string, quality int) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer file.Close()

	var img image.Image

	switch ext {
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(file)
	case ".png":
		img, err = png.Decode(file)
	default:
		return fmt.Errorf("unsupported format: %s", ext)
	}

	if err != nil {
		return fmt.Errorf("decoding image: %w", err)
	}

	outFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("creating output: %w", err)
	}
	defer outFile.Close()

	// Encode to WebP with specified quality
	if err := webp.Encode(outFile, img, &webp.Options{Quality: float32(quality)}); err != nil {
		return fmt.Errorf("encoding webp: %w", err)
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

		return os.WriteFile(path, []byte(newContent), info.Mode())
	})
}
