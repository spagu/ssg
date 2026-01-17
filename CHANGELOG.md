# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.3.0] - 2026-01-17

### Added
- 🌐 **Built-in HTTP server** (`--http` flag) - no need for external Python/Node server
- 🔌 **Custom port** (`--port=PORT`) - default: 8888
- 👀 **Watch mode** (`--watch` flag) - auto-rebuild on file changes in content/templates
- 🎚️ **WebP quality control** (`--webp-quality=N`) - compression level 1-100 (default: 60)
- 📝 `stripHTML` template function for clean meta descriptions
- 📦 **Multi-platform packages**:
  - Debian/Ubuntu: `.deb` packages (amd64, arm64)
  - Fedora/RHEL: `.rpm` packages (x86_64, aarch64)
  - Ubuntu Snap: `snap` package
  - macOS Homebrew: `brew install spagu/tap/ssg`
  - FreeBSD/OpenBSD: Port Makefiles
- 🔧 Quick install script (`install.sh`)
- 📖 Comprehensive installation documentation (`docs/INSTALL.md`)

### Changed
- Refactored build logic into reusable function for watch mode

### Fixed
- Page title overlapping with fixed navigation header
- Text width constrained by `max-width: 65ch` now fills container properly

## [1.2.0] - 2026-01-16

### Added
- 🎬 **GitHub Actions support** - Use SSG as a step in GitHub Actions workflows
- 📋 `action.yml` - Composite action definition with full input/output configuration
- 🔄 CI/CD workflows:
  - `ci.yml` - Test, lint, build, and release pipeline
  - `test-action.yml` - Tests for the GitHub Action itself
  - `example-deploy.yml` - Example Cloudflare Pages deployment workflow
- 📦 Automatic artifact uploads for all platforms
- 🏷️ Automatic release creation from version tags (v*)
- 🧪 Test content for CI validation
- 📂 **Custom directory paths**:
  - `--content-dir=PATH` - specify custom content directory
  - `--templates-dir=PATH` - specify custom templates directory  
  - `--output-dir=PATH` - specify custom output directory
- 😈 **FreeBSD support** - builds for FreeBSD amd64 and arm64
- 🗓️ **Flexible date parsing** - supports multiple formats:
  - RFC3339: `2025-01-01T12:00:00Z`
  - Datetime: `2025-01-01T12:00:00`
  - Date only: `2025-01-01`
  - And more formats

### Changed
- Improved cross-platform build matrix (8 targets now)
- All platforms now include arm64 builds:
  - Linux: amd64, arm64
  - FreeBSD: amd64, arm64
  - macOS: amd64, arm64
  - Windows: amd64, arm64
- Enhanced output path configuration via action inputs

### Fixed
- Date parsing now handles simple `YYYY-MM-DD` format correctly
- Fixed "same file" error in GitHub Action when testing locally with `uses: ./`
- Code cleanup: Fixed unhandled error returns (golangci-lint errcheck)

### Documentation
- Updated README with GitHub Actions usage examples
- Added workflow examples for Cloudflare Pages deployment
- Added CLI options documentation
- Added status badges for Code Quality, Coverage, and Project Stats

## [1.1.0] - 2026-01-13

### Added
- 🖼️ WebP image conversion (`--webp` flag) - reduces image sizes by ~70%
- 📦 ZIP deployment package (`--zip` flag) for Cloudflare Pages
- ☁️ Cloudflare Pages support with `_headers` and `_redirects` files
- 📊 Markdown table support (GFM extension)
- 🔗 Automatic media path fixing (relative to absolute)
- 🗺️ Sitemap.xml generation
- 🤖 robots.txt generation
- 🔐 SEO meta tags (Open Graph, Twitter Card, Schema.org JSON-LD)

### Changed
- Improved image path handling in HTML and CSS files
- Better srcset handling for responsive images

### Fixed
- Fixed relative media paths in href attributes
- Fixed srcset image extensions when using --webp

## [1.0.0] - 2026-01-13

### Added
- 🚀 Initial release of SSG (Static Site Generator)
- 📝 Markdown parser with YAML frontmatter support
- 🎨 Two templates: **simple** (dark) and **krowy** (green/farm theme)
- 📄 Page generation with SEO-friendly URLs
- 📝 Post generation with category support
- 📁 Category listing pages
- 🖼️ Media file copying
- 📱 Responsive design for both templates
- ♿ WCAG 2.2 color contrast compliance
- 🧪 Unit tests for parser and generator
- 📖 Comprehensive documentation
- 🔧 Makefile with colored output and help

### Templates
- **simple**: Modern dark theme with glassmorphism, purple gradient accents, micro-animations
- **krowy**: Light green farm theme inspired by krowy.net, natural colors, cow emoji logo

### Technical
- Go 1.25+ required
- Single binary output
- Dependencies: gopkg.in/yaml.v3, github.com/yuin/goldmark
- Cross-platform build support (Linux, macOS, Windows)
