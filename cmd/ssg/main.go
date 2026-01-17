// Package main provides the entry point for the SSG (Static Site Generator) CLI tool.
// Usage: ssg <source> <template> <domain> [options]
// Example: ssg krowy.net.2026-01-13110345 simple krowy.net --zip --webp
// Example: ssg my-content my-template example.com --http --watch
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/generator"
)

func main() {
	// Parse arguments manually to support flags at end
	args := os.Args[1:]
	zipFlag := false
	webpFlag := false
	webpQuality := 60 // Default quality (1-100)
	watchFlag := false
	httpFlag := false
	httpPort := 8888
	contentDir := "content"
	templatesDir := "templates"
	outputDir := "output"

	// Filter out flags and collect positional args
	var positionalArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--zip" || arg == "-zip":
			zipFlag = true
		case arg == "--webp" || arg == "-webp":
			webpFlag = true
		case strings.HasPrefix(arg, "--webp-quality="):
			if q, err := strconv.Atoi(strings.TrimPrefix(arg, "--webp-quality=")); err == nil && q >= 1 && q <= 100 {
				webpQuality = q
			}
		case arg == "--webp-quality" && i+1 < len(args):
			i++
			if q, err := strconv.Atoi(args[i]); err == nil && q >= 1 && q <= 100 {
				webpQuality = q
			}
		case arg == "--watch" || arg == "-watch":
			watchFlag = true
		case arg == "--http" || arg == "-http":
			httpFlag = true
		case strings.HasPrefix(arg, "--port="):
			if port, err := strconv.Atoi(strings.TrimPrefix(arg, "--port=")); err == nil {
				httpPort = port
			}
		case arg == "--port" && i+1 < len(args):
			i++
			if port, err := strconv.Atoi(args[i]); err == nil {
				httpPort = port
			}
		case strings.HasPrefix(arg, "--content-dir="):
			contentDir = strings.TrimPrefix(arg, "--content-dir=")
		case strings.HasPrefix(arg, "--templates-dir="):
			templatesDir = strings.TrimPrefix(arg, "--templates-dir=")
		case strings.HasPrefix(arg, "--output-dir="):
			outputDir = strings.TrimPrefix(arg, "--output-dir=")
		case arg == "--content-dir" && i+1 < len(args):
			i++
			contentDir = args[i]
		case arg == "--templates-dir" && i+1 < len(args):
			i++
			templatesDir = args[i]
		case arg == "--output-dir" && i+1 < len(args):
			i++
			outputDir = args[i]
		case !strings.HasPrefix(arg, "-"):
			positionalArgs = append(positionalArgs, arg)
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
		ContentDir:   contentDir,
		TemplatesDir: templatesDir,
		OutputDir:    outputDir,
	}

	// Initial build
	if err := build(cfg, webpFlag, webpQuality, zipFlag); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if !watchFlag && !httpFlag {
			os.Exit(1)
		}
	}

	// Start HTTP server if requested
	if httpFlag {
		go startServer(cfg.OutputDir, httpPort)
	}

	// Watch mode
	if watchFlag {
		fmt.Println("👀 Watching for changes in content and templates...")
		lastBuild := time.Now()

		for {
			time.Sleep(1 * time.Second) // Poll every second

			if hasChanges([]string{cfg.ContentDir, cfg.TemplatesDir}, lastBuild) {
				fmt.Println("\n🔄 Changes detected! Rebuilding...")
				if err := build(cfg, webpFlag, webpQuality, zipFlag); err != nil {
					fmt.Fprintf(os.Stderr, "Error rebuilding: %v\n", err)
				}
				lastBuild = time.Now()
				fmt.Println("👀 Watching for changes...")
			}
		}
	} else if httpFlag {
		// If only HTTP server (no watch), block forever
		select {}
	}
}

func startServer(dir string, port int) {
	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("🌐 Starting HTTP server at http://localhost%s\n", addr)
	fmt.Printf("   Serving files from: %s/\n", dir)

	fs := http.FileServer(http.Dir(dir))
	http.Handle("/", fs)

	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Server error: %v\n", err)
	}
}

func build(cfg generator.Config, webpFlag bool, webpQuality int, zipFlag bool) error {
	gen, err := generator.New(cfg)
	if err != nil {
		return fmt.Errorf("initializing generator: %w", err)
	}

	if err := gen.Generate(); err != nil {
		return fmt.Errorf("generating site: %w", err)
	}

	fmt.Printf("✅ Site generated successfully to %s/\n", cfg.OutputDir)

	// Convert images to WebP if requested
	if webpFlag {
		fmt.Printf("🖼️  Converting images to WebP (quality: %d)...\n", webpQuality)
		if err := convertToWebP(cfg.OutputDir, webpQuality); err != nil {
			return fmt.Errorf("converting to WebP: %w", err)
		}
		fmt.Println("✅ Images converted to WebP")
	}

	// Create ZIP if requested
	if zipFlag {
		zipFileName := fmt.Sprintf("%s.zip", cfg.Domain)
		if err := createCloudflareZip(cfg.OutputDir, zipFileName); err != nil {
			return fmt.Errorf("creating ZIP: %w", err)
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
	return nil
}

func hasChanges(dirs []string, lastBuild time.Time) bool {
	changed := false
	for _, dir := range dirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Ignore errors during walk
			}
			if !info.IsDir() {
				if info.ModTime().After(lastBuild) {
					changed = true
					return io.EOF // Stop walking
				}
			}
			return nil
		})
		if changed {
			break
		}
	}
	return changed
}

// convertToWebP converts all JPG/PNG images to WebP format
func convertToWebP(outputDir string, quality int) error {
	// Check if cwebp is available
	if _, err := exec.LookPath("cwebp"); err != nil {
		return fmt.Errorf("cwebp not found. Install with: sudo apt install webp")
	}

	qualityStr := strconv.Itoa(quality)

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

		// Use specified quality
		cmd := exec.Command("cwebp", "-q", qualityStr, "-quiet", path, "-o", webpPath)
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
	defer func() { _ = zipFile.Close() }()

	zipWriter := zip.NewWriter(zipFile)
	defer func() { _ = zipWriter.Close() }()

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
		defer func() { _ = file.Close() }()

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
	fmt.Println("  source    - Content source folder name (inside content-dir)")
	fmt.Println("  template  - Template name (inside templates-dir)")
	fmt.Println("  domain    - Target domain for the generated site")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  --http                 - Start built-in HTTP server")
	fmt.Println("  --port=PORT            - HTTP server port (default: 8888)")
	fmt.Println("  --watch                - Watch for changes and rebuild automatically")
	fmt.Println("  --zip                  - Create ZIP file for Cloudflare Pages deployment")
	fmt.Println("  --webp                 - Convert images to WebP format (reduces size)")
	fmt.Println("  --webp-quality=N       - WebP compression quality 1-100 (default: 60)")
	fmt.Println("  --content-dir=PATH     - Path to content directory (default: content)")
	fmt.Println("  --templates-dir=PATH   - Path to templates directory (default: templates)")
	fmt.Println("  --output-dir=PATH      - Path to output directory (default: output)")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ssg my-site simple example.com --http --watch")
	fmt.Println("  ssg my-site simple example.com --webp --webp-quality=80")
}
