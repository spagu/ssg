// Package main provides the entry point for the SSG (Static Site Generator) CLI tool.
// Usage: ssg <source> <template> <domain> [--zip] [--webp]
// Example: ssg krowy.net.2026-01-13110345 simple krowy.net --zip --webp
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/generator"
)

func main() {
	// Parse arguments manually to support flags at end
	args := os.Args[1:]
	zipFlag := false
	webpFlag := false

	// Filter out flags and collect positional args
	var positionalArgs []string
	for _, arg := range args {
		switch arg {
		case "--zip", "-zip":
			zipFlag = true
		case "--webp", "-webp":
			webpFlag = true
		default:
			if !strings.HasPrefix(arg, "-") {
				positionalArgs = append(positionalArgs, arg)
			}
		}
	}

	if len(positionalArgs) < 3 {
		printUsage()
		os.Exit(1)
	}

	source := positionalArgs[0]
	template := positionalArgs[1]
	domain := positionalArgs[2]

	cfg := generator.Config{
		Source:       source,
		Template:     template,
		Domain:       domain,
		ContentDir:   "content",
		TemplatesDir: "templates",
		OutputDir:    "output",
	}

	gen, err := generator.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing generator: %v\n", err)
		os.Exit(1)
	}

	if err := gen.Generate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating site: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Site generated successfully to %s/\n", cfg.OutputDir)

	// Convert images to WebP if requested
	if webpFlag {
		fmt.Println("🖼️  Converting images to WebP...")
		if err := convertToWebP(cfg.OutputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error converting to WebP: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Images converted to WebP")
	}

	// Create ZIP if requested
	if zipFlag {
		zipFileName := fmt.Sprintf("%s.zip", domain)
		if err := createCloudflareZip(cfg.OutputDir, zipFileName); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating ZIP: %v\n", err)
			os.Exit(1)
		}

		// Get file size
		if info, err := os.Stat(zipFileName); err == nil {
			sizeMB := float64(info.Size()) / (1024 * 1024)
			fmt.Printf("📦 Created deployment package: %s (%.1f MB)\n", zipFileName, sizeMB)
			if sizeMB > 25 {
				fmt.Printf("⚠️  Warning: File exceeds Cloudflare Pages 25MB limit!\n")
			}
		}
	}
}

// convertToWebP converts all JPG/PNG images to WebP format
func convertToWebP(outputDir string) error {
	// Check if cwebp is available
	if _, err := exec.LookPath("cwebp"); err != nil {
		return fmt.Errorf("cwebp not found. Install with: sudo apt install webp")
	}

	var convertedCount int
	var savedBytes int64

	// Find all images
	err := filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			return nil
		}

		// Get original size
		originalSize := info.Size()

		// Convert to WebP
		webpPath := strings.TrimSuffix(path, ext) + ".webp"

		// Use quality 60 for smaller size (CF Pages 25MB limit)
		cmd := exec.Command("cwebp", "-q", "60", "-quiet", path, "-o", webpPath)
		if err := cmd.Run(); err != nil {
			fmt.Printf("   ⚠️  Failed to convert %s: %v\n", filepath.Base(path), err)
			return nil // Continue with other files
		}

		// Get new size
		if newInfo, err := os.Stat(webpPath); err == nil {
			savedBytes += originalSize - newInfo.Size()
		}

		// Remove original file
		if err := os.Remove(path); err != nil {
			fmt.Printf("   ⚠️  Failed to remove original %s: %v\n", filepath.Base(path), err)
		}

		convertedCount++
		return nil
	})

	if err != nil {
		return err
	}

	// Update HTML and CSS files to use .webp extensions
	err = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		ext := filepath.Ext(path)
		if info.IsDir() || (ext != ".html" && ext != ".css") {
			return nil
		}

		// Read file
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Replace image extensions
		newContent := string(content)
		newContent = strings.ReplaceAll(newContent, ".jpg\"", ".webp\"")
		newContent = strings.ReplaceAll(newContent, ".jpeg\"", ".webp\"")
		newContent = strings.ReplaceAll(newContent, ".png\"", ".webp\"")
		newContent = strings.ReplaceAll(newContent, ".jpg'", ".webp'")
		newContent = strings.ReplaceAll(newContent, ".jpeg'", ".webp'")
		newContent = strings.ReplaceAll(newContent, ".png'", ".webp'")
		// Handle CSS url() syntax with parentheses
		newContent = strings.ReplaceAll(newContent, ".jpg)", ".webp)")
		newContent = strings.ReplaceAll(newContent, ".jpeg)", ".webp)")
		newContent = strings.ReplaceAll(newContent, ".png)", ".webp)")
		// Handle srcset entries with space and width descriptor (e.g., .jpg 300w)
		newContent = strings.ReplaceAll(newContent, ".jpg ", ".webp ")
		newContent = strings.ReplaceAll(newContent, ".jpeg ", ".webp ")
		newContent = strings.ReplaceAll(newContent, ".png ", ".webp ")

		// Write back
		if err := os.WriteFile(path, []byte(newContent), info.Mode()); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	savedMB := float64(savedBytes) / (1024 * 1024)
	fmt.Printf("   📊 Converted %d images, saved %.1f MB\n", convertedCount, savedMB)

	return nil
}

// createCloudflareZip creates a ZIP file suitable for Cloudflare Pages deployment
func createCloudflareZip(sourceDir, zipFileName string) error {
	// Create ZIP file
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return fmt.Errorf("creating zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Walk through the output directory
	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if path == sourceDir {
			return nil
		}

		// Get relative path (relative to output dir)
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("getting relative path: %w", err)
		}

		// Use forward slashes for ZIP compatibility
		relPath = strings.ReplaceAll(relPath, string(os.PathSeparator), "/")

		// Create header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("creating file header: %w", err)
		}
		header.Name = relPath

		if info.IsDir() {
			header.Name += "/"
			_, err = zipWriter.CreateHeader(header)
			return err
		}

		// Set compression method
		header.Method = zip.Deflate

		// Create writer for file
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("creating zip entry: %w", err)
		}

		// Open source file
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}
		defer file.Close()

		// Copy content
		_, err = io.Copy(writer, file)
		return err
	})

	if err != nil {
		return fmt.Errorf("walking directory: %w", err)
	}

	return nil
}

func printUsage() {
	fmt.Println("SSG - Static Site Generator")
	fmt.Println("")
	fmt.Println("Usage: ssg <source> <template> <domain> [options]")
	fmt.Println("")
	fmt.Println("Arguments:")
	fmt.Println("  source    - Content source folder name (inside content/)")
	fmt.Println("  template  - Template name (inside templates/)")
	fmt.Println("  domain    - Target domain for the generated site")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  --zip     - Create ZIP file for Cloudflare Pages deployment")
	fmt.Println("  --webp    - Convert images to WebP format (reduces size significantly)")
	fmt.Println("")
	fmt.Println("Example:")
	fmt.Println("  ssg krowy.net.2026-01-13110345 simple krowy.net")
	fmt.Println("  ssg krowy.net.2026-01-13110345 krowy krowy.net --zip")
	fmt.Println("  ssg krowy.net.2026-01-13110345 krowy krowy.net --webp --zip")
}
