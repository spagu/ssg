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

// Version is set by build flags
var Version = "dev"

func main() {
	// Parse arguments manually to support flags at end
	args := os.Args[1:]

	// Flags with defaults
	zipFlag := false
	webpFlag := false
	webpQuality := 60
	watchFlag := false
	httpFlag := false
	httpPort := 8888
	contentDir := "content"
	templatesDir := "templates"
	outputDir := "output"

	// New flags
	sitemapOff := false
	robotsOff := false
	minifyAll := false
	minifyHTML := false
	minifyCSS := false
	minifyJS := false
	sourceMap := false
	cleanFlag := false
	quietFlag := false

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
		// New flags
		case arg == "--sitemap-off":
			sitemapOff = true
		case arg == "--robots-off":
			robotsOff = true
		case arg == "--minify-all":
			minifyAll = true
		case arg == "--minify-html":
			minifyHTML = true
		case arg == "--minify-css":
			minifyCSS = true
		case arg == "--minify-js":
			minifyJS = true
		case arg == "--sourcemap":
			sourceMap = true
		case arg == "--clean":
			cleanFlag = true
		case arg == "--quiet" || arg == "-q":
			quietFlag = true
		case arg == "--version" || arg == "-v":
			fmt.Printf("ssg version %s\n", Version)
			os.Exit(0)
		case arg == "--help" || arg == "-h":
			printUsage()
			os.Exit(0)
		case !strings.HasPrefix(arg, "-"):
			positionalArgs = append(positionalArgs, arg)
		}
	}

	if len(positionalArgs) < 3 {
		printUsage()
		os.Exit(1)
	}

	// Apply minify-all
	if minifyAll {
		minifyHTML = true
		minifyCSS = true
		minifyJS = true
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
		SitemapOff:   sitemapOff,
		RobotsOff:    robotsOff,
		MinifyHTML:   minifyHTML,
		MinifyCSS:    minifyCSS,
		MinifyJS:     minifyJS,
		SourceMap:    sourceMap,
		Clean:        cleanFlag,
		Quiet:        quietFlag,
	}

	// Initial build
	if err := build(cfg, webpFlag, webpQuality, zipFlag); err != nil {
		if !quietFlag {
			fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		}
		if !watchFlag && !httpFlag {
			os.Exit(1)
		}
	} else if !quietFlag {
		fmt.Printf("✅ Site generated successfully to %s/\n", cfg.OutputDir)
	}

	// Start HTTP server if requested
	if httpFlag {
		go startServer(cfg.OutputDir, httpPort, quietFlag)
	}

	// Watch mode
	if watchFlag {
		if !quietFlag {
			fmt.Println("👀 Watching for changes in content and templates...")
		}
		lastBuild := time.Now()

		for {
			time.Sleep(1 * time.Second)

			if hasChanges([]string{cfg.ContentDir, cfg.TemplatesDir}, lastBuild) {
				if !quietFlag {
					fmt.Println("\n🔄 Changes detected! Rebuilding...")
				}
				if err := build(cfg, webpFlag, webpQuality, zipFlag); err != nil {
					// In watch mode, show error but don't exit
					if !quietFlag {
						fmt.Fprintf(os.Stderr, "❌ Build error: %v\n", err)
						fmt.Println("⚠️  Fix the issue and save to retry...")
					}
				} else if !quietFlag {
					fmt.Printf("✅ Rebuilt successfully\n")
				}
				lastBuild = time.Now()
				if !quietFlag {
					fmt.Println("👀 Watching for changes...")
				}
			}
		}
	} else if httpFlag {
		select {}
	}
}

func startServer(dir string, port int, quiet bool) {
	addr := fmt.Sprintf(":%d", port)
	if !quiet {
		fmt.Printf("🌐 Starting HTTP server at http://localhost%s\n", addr)
		fmt.Printf("   Serving files from: %s/\n", dir)
	}

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

	// Convert images to WebP if requested
	if webpFlag {
		if !cfg.Quiet {
			fmt.Printf("🖼️  Converting images to WebP (quality: %d)...\n", webpQuality)
		}
		if err := convertToWebP(cfg.OutputDir, webpQuality); err != nil {
			return fmt.Errorf("converting to WebP: %w", err)
		}
		if !cfg.Quiet {
			fmt.Println("✅ Images converted to WebP")
		}
	}

	// Create ZIP if requested
	if zipFlag {
		zipFileName := fmt.Sprintf("%s.zip", cfg.Domain)
		if err := createCloudflareZip(cfg.OutputDir, zipFileName); err != nil {
			return fmt.Errorf("creating ZIP: %w", err)
		}

		if !cfg.Quiet {
			if info, err := os.Stat(zipFileName); err == nil {
				sizeMB := float64(info.Size()) / (1024 * 1024)
				fmt.Printf("📦 Created deployment package: %s (%.1f MB)\n", zipFileName, sizeMB)
				if sizeMB > 25 {
					fmt.Printf("⚠️  Warning: File exceeds Cloudflare Pages 25MB limit!\n")
				}
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
				return nil
			}
			if !info.IsDir() {
				if info.ModTime().After(lastBuild) {
					changed = true
					return io.EOF
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
	if _, err := exec.LookPath("cwebp"); err != nil {
		return fmt.Errorf("cwebp not found. Install with: sudo apt install webp")
	}

	qualityStr := strconv.Itoa(quality)
	var convertedCount int
	var savedBytes int64

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

		originalSize := info.Size()
		webpPath := strings.TrimSuffix(path, ext) + ".webp"

		cmd := exec.Command("cwebp", "-q", qualityStr, "-quiet", path, "-o", webpPath)
		if err := cmd.Run(); err != nil {
			fmt.Printf("   ⚠️  Failed to convert %s: %v\n", filepath.Base(path), err)
			return nil
		}

		if newInfo, err := os.Stat(webpPath); err == nil {
			savedBytes += originalSize - newInfo.Size()
		}

		if err := os.Remove(path); err != nil {
			fmt.Printf("   ⚠️  Failed to remove original %s: %v\n", filepath.Base(path), err)
		}

		convertedCount++
		return nil
	})

	if err != nil {
		return err
	}

	err = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
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
		newContent = strings.ReplaceAll(newContent, ".jpg\"", ".webp\"")
		newContent = strings.ReplaceAll(newContent, ".jpeg\"", ".webp\"")
		newContent = strings.ReplaceAll(newContent, ".png\"", ".webp\"")
		newContent = strings.ReplaceAll(newContent, ".jpg'", ".webp'")
		newContent = strings.ReplaceAll(newContent, ".jpeg'", ".webp'")
		newContent = strings.ReplaceAll(newContent, ".png'", ".webp'")
		newContent = strings.ReplaceAll(newContent, ".jpg)", ".webp)")
		newContent = strings.ReplaceAll(newContent, ".jpeg)", ".webp)")
		newContent = strings.ReplaceAll(newContent, ".png)", ".webp)")
		newContent = strings.ReplaceAll(newContent, ".jpg ", ".webp ")
		newContent = strings.ReplaceAll(newContent, ".jpeg ", ".webp ")
		newContent = strings.ReplaceAll(newContent, ".png ", ".webp ")

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

func createCloudflareZip(sourceDir, zipFileName string) error {
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return fmt.Errorf("creating zip file: %w", err)
	}
	defer func() { _ = zipFile.Close() }()

	zipWriter := zip.NewWriter(zipFile)
	defer func() { _ = zipWriter.Close() }()

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == sourceDir {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("getting relative path: %w", err)
		}

		relPath = strings.ReplaceAll(relPath, string(os.PathSeparator), "/")

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

		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("creating zip entry: %w", err)
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}
		defer func() { _ = file.Close() }()

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
	fmt.Println("Server & Development:")
	fmt.Println("  --http                 - Start built-in HTTP server")
	fmt.Println("  --port=PORT            - HTTP server port (default: 8888)")
	fmt.Println("  --watch                - Watch for changes and rebuild automatically")
	fmt.Println("  --clean                - Clean output directory before build")
	fmt.Println("")
	fmt.Println("Output Control:")
	fmt.Println("  --sitemap-off          - Disable sitemap.xml generation")
	fmt.Println("  --robots-off           - Disable robots.txt generation")
	fmt.Println("  --minify-all           - Minify HTML, CSS, and JS")
	fmt.Println("  --minify-html          - Minify HTML output")
	fmt.Println("  --minify-css           - Minify CSS output")
	fmt.Println("  --minify-js            - Minify JS output")
	fmt.Println("  --sourcemap            - Include source maps in output")
	fmt.Println("")
	fmt.Println("Image Processing:")
	fmt.Println("  --webp                 - Convert images to WebP format")
	fmt.Println("  --webp-quality=N       - WebP compression quality 1-100 (default: 60)")
	fmt.Println("")
	fmt.Println("Deployment:")
	fmt.Println("  --zip                  - Create ZIP file for Cloudflare Pages")
	fmt.Println("")
	fmt.Println("Paths:")
	fmt.Println("  --content-dir=PATH     - Content directory (default: content)")
	fmt.Println("  --templates-dir=PATH   - Templates directory (default: templates)")
	fmt.Println("  --output-dir=PATH      - Output directory (default: output)")
	fmt.Println("")
	fmt.Println("Other:")
	fmt.Println("  --quiet, -q            - Suppress output (only exit codes)")
	fmt.Println("  --version, -v          - Show version")
	fmt.Println("  --help, -h             - Show this help")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ssg my-site simple example.com --http --watch")
	fmt.Println("  ssg my-site krowy example.com --clean --minify-all --zip")
	fmt.Println("  ssg my-site simple example.com --quiet")
}
