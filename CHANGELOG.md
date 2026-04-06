# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.7.10] - 2026-04-06

### Added
- ✨ **Rewrite `.md` links to final URLs** - opt-in via `rewrite_md_links: true` (closes #5)
  - Rewrites `href="AUTHENTICATION.md"` → `href="/authentication/"` based on actual slug
  - Handles relative prefixes `./file.md`, `../dir/file.md` — only base filename is matched
  - Priority: exact source filename > lowercase > slug-derived
  - Unknown `.md` links are left untouched
  - Disabled by default to avoid breaking sites serving raw `.md` files
- ✨ **Auto-derive slug from filename** - when no `slug:` in frontmatter, derived from filename
  - `AUTHENTICATION.md` without slug → slug `authentication` → `/authentication/`
- ✨ **`preserve_slug_case` option** - control URL casing for slugs derived from filenames
  - Default (`false`): lowercased — `API.md` → `/api/`
  - `preserve_slug_case: true` — original case kept — `API.md` → `/API/`

## [1.7.9] - 2026-04-06

### Added
- ✨ **Configurable pages and posts paths** - Override default `pages/` and `posts/` subdirectory names via config
  - `pages_path: "docs"` — read static pages from `content/{source}/docs/` instead of `pages/`
  - `posts_path: "articles"` — read posts from `content/{source}/articles/` instead of `posts/`
  - Default behaviour (`pages/` and `posts/`) is preserved when not set

## [1.7.8] - 2026-04-06

### Added
- ✨ **Template variables** - Define custom variables in `.ssg.yaml` available in all templates as `{{.Vars.key}}`
  - Flat and nested structures supported: `{{.Vars.gtm}}`, `{{.Vars.api.endpoint}}`
  - Values starting with `$` are resolved from OS environment variables at build time (e.g. `"$GTM_CODE"`)
  - All variables automatically exported as environment variables with `SSG_` prefix (e.g. `SSG_GTM`, `SSG_API_ENDPOINT`)
  - Available in every template context: index, page, post, category

## [1.7.7] - 2026-04-01

### Added
- ✨ **Skip minification for specific elements** - Use `<!-- htmlmin:ignore -->` comments (fixes #2)
  - Wrap content with `<!-- htmlmin:ignore -->...<!-- /htmlmin:ignore -->` to preserve whitespace
  - Perfect for Mermaid.js diagrams, code blocks, and pre-formatted content
  - Multiple ignore blocks supported in a single file

## [1.7.6] - 2026-04-01

### Fixed
- 🐛 **Pages directory now supports subdirectories** - Recursive scanning of `pages/` directory (fixes #1)
  - `content/pages/docs/intro.md` → `/docs/intro/`
  - `content/pages/docs/advanced/guide.md` → `/docs/advanced/guide/`
  - Works for both pages and posts (via category subdirectories)

## [1.7.4] - 2026-04-01

### Fixed
- 🐛 **Markdown parser fallback mode** - Content without `## Excerpt` or `## Content` markers is now properly parsed
  - Previously, markdown files without explicit section markers would have empty content
  - Now all content after frontmatter is treated as content when no markers are present

## [1.7.3] - 2026-03-31

### Added
- ✨ **Dynamic MDDB metadata fields with top-level access** - Custom metadata fields are flattened to template root
  - Use `{{.dupa}}` directly instead of `{{.Extra.dupa}}` or `{{.Page.Extra.dupa}}`
  - All standard Page fields also available at root: `{{.Title}}`, `{{.Content}}`, `{{.Slug}}`, etc.
  - Backward compatible: `{{.Page.Title}}` and `{{.Post.Title}}` still work
  - URL helpers at root level: `{{.URL}}`, `{{.CanonicalURL}}`, `{{.OutputPath}}`
- ✨ **Additional SEO fields from MDDB** - Now extracts: `description`, `keywords`, `lang`, `canonical`, `robots`, `featured_image`, `tags`, `category`, `layout`, `template`

## [1.7.2] - 2026-03-31

### Added
- 🔗 **Page output format** (`--page-format` / `page_format`) - Control how HTML files are generated
  - `directory` (default): `slug/index.html` - clean URLs with trailing slash
  - `flat`: `slug.html` - direct HTML files (e.g., `/docs/introduction.html`)
  - `both`: generates both formats for maximum compatibility
  - Works for both pages and posts
  - Config file option: `page_format: "flat"`

### Documentation
- 📖 Updated README.md with complete MDDB gRPC and watch mode documentation
- 📖 Updated man page with all MDDB options (protocol, watch, batch-size)
- 📖 Updated docs/INSTALL.md to require Go 1.26

## [1.7.1] - 2026-03-30

### Added
- 📎 **Co-located content assets** - Images and media files placed alongside Markdown content files are automatically copied to the corresponding output directory
  - Place `entry-image.png` next to `entry.md` and reference it with `![](entry-image.png)`
  - Supports: PNG, JPG, JPEG, GIF, SVG, WebP, ICO, BMP, TIFF, AVIF, MP4, WebM, OGG, MP3, WAV, PDF, ZIP
  - Works for both pages and posts
- 📖 **Man page** - Comprehensive `ssg.1` man page with full documentation of all options, configuration, and examples
  - Installed automatically via `make install`, DEB, and RPM packages

### Changed
- ⬆️ **Go dependencies updated** - All modules bumped to latest versions
  - goldmark v1.7.16 → v1.8.2
  - grpc v1.79.1 → v1.79.3
  - golang.org/x/net v0.48.0 → v0.52.0
  - golang.org/x/sys v0.39.0 → v0.42.0
  - golang.org/x/text v0.32.0 → v0.35.0
- 🐳 **Docker image updated**
  - Go builder: 1.25 → 1.26
  - Alpine runtime: 3.19 → 3.23
- 🔧 **GitHub Actions updated to latest versions**
  - codecov/codecov-action v4 → v5
  - docker/setup-qemu-action v3 → v4
  - docker/setup-buildx-action v3 → v4
  - docker/login-action v3 → v4
  - docker/metadata-action v5 → v6
  - docker/build-push-action v5 → v7
  - actions/upload-artifact v4 → v7
  - actions/download-artifact v4 → v8
  - github/codeql-action v3 → v4
- 📦 **Snap package updated** - base core22 → core24, platforms syntax
- 🔒 **Security** - Added gosec `#nosec` annotations for all G703/G122 false positives

## [1.7.0] - 2026-03-05

### Added
- ✨ **MDDB gRPC Support** - Optional gRPC connection alongside HTTP
  - CLI flag: `--mddb-protocol=grpc` (default: `http`)
  - YAML config: `mddb.protocol: "grpc"`
  - gRPC port: 11024 (HTTP: 11023)
  - Uses protobuf for faster serialization
  - Full gRPC API generated from MDDB proto file
- ✨ **MDDB Watch Mode** - Auto-rebuild on content changes
  - CLI flags: `--mddb-watch`, `--mddb-watch-interval=SEC`
  - YAML config: `mddb.watch: true`, `mddb.watch_interval: 30`
  - Polls collection checksum and rebuilds when content changes
  - Works with both HTTP and gRPC protocols

### Changed
- Refactored MDDB client to use interface pattern (supports HTTP and gRPC implementations)

## [1.6.2] - 2026-03-05

### Added
- ✨ **MDDB Batch Size** - Configurable batch size for pagination
  - CLI flag: `--mddb-batch-size=N` (default: 1000)
  - YAML config: `mddb.batch_size`
  - Removed hardcoded 1000 limit in `GetByType` - now fetches all documents with pagination

## [1.6.1] - 2026-03-05

### Fixed
- 🐛 **MDDB Client** - Aligned with actual MDDB API format
  - `contentMd` instead of `content`
  - `meta` (arrays) instead of `metadata`
  - `addedAt`/`updatedAt` (unix timestamps) instead of ISO dates
  - `X-Total-Count` header for pagination
  - `/v1/get` returns document directly (no wrapper)
  - `/v1/search` returns array directly
- 🐛 **Install Script** - Fixed download URL pattern for release assets

## [1.6.0] - 2026-03-05

### Added
- ✨ **MDDB Content Source** - Fetch markdown content from [MDDB](https://github.com/tradik/mddb) server
  - Single document fetch via `/v1/get` endpoint
  - Bulk fetch via `/v1/search` endpoint with pagination
  - CLI flags: `--mddb-url`, `--mddb-collection`, `--mddb-key`, `--mddb-lang`, `--mddb-timeout`
  - YAML config support:
    ```yaml
    mddb:
      enabled: true
      url: "http://localhost:8080"
      collection: "blog"
      lang: "en_US"
    ```
  - Automatic conversion of MDDB documents to pages/posts
  - Support for categories, media, and users collections

## [1.5.4] - 2026-02-04

### Added
- ✨ **Configurable shortcodes** - Define reusable content snippets in config
  - Use `{{shortcode_name}}` syntax in markdown content
  - Each shortcode requires a template file (no built-in HTML)
  - Template variables: `{{.Name}}`, `{{.Title}}`, `{{.Text}}`, `{{.URL}}`, `{{.Logo}}`, `{{.Legal}}`, `{{.Data}}`
  - Define in `.ssg.yaml`:
    ```yaml
    shortcodes:
      - name: "promo"
        template: "shortcodes/banner.html"
        title: "Special Offer"
        text: "Get 50% off!"
        url: "https://example.com"
    ```

## [1.5.3] - 2026-02-04

### Added
- ✨ **Relative links conversion** (`--relative-links` / `relative_links: true`)
  - Converts absolute URLs with site domain to relative links
  - Supports `href`, `src`, `action` attributes and `url()` in inline styles
  - Works with https, http, and protocol-relative URLs
  - Preserves external links to other domains

## [1.5.2] - 2026-02-03

### Fixed
- 🐛 **Pretty HTML now reliably removes ALL blank lines** - Refactored algorithm for better reliability
  - Uses line-by-line processing instead of regex for more predictable results
  - Handles CRLF and mixed line endings (Windows compatibility)
  - Added tests for CRLF and mixed line ending scenarios

## [1.5.1] - 2026-02-03

### Fixed
- 🐛 **Link field always takes priority** - If a post has `link` in frontmatter, it's used regardless of `post_url_format` setting
  - `post_url_format` is now a fallback when `link` is not present

## [1.5.0] - 2026-02-03

### Added
- ✨ **Configurable post URL format** (`--post-url-format` / `post_url_format`)
  - `date` (default): `/YYYY/MM/DD/slug/` - date-based URLs
  - `slug`: `/slug/` - SEO-friendly slug-only URLs
  - `link` field from frontmatter **always** takes priority
  - Config file option: `post_url_format: "slug"`

## [1.4.9] - 2026-01-29

### Fixed
- 🐛 **Pretty HTML now removes ALL blank lines** - Improved `--pretty-html` to fully clean HTML output
  - Previously only collapsed 3+ blank lines to 1 blank line
  - Now removes ALL empty/blank lines for truly clean HTML
  - Added comprehensive tests for config file parsing (`pretty_html: true`)

## [1.4.8] - 2026-01-29

### Changed
- 🔒 **Code quality improvements** - Refactored high-complexity functions and fixed all security scanner warnings
  - Reduced cyclomatic complexity in `main()`, `parseFlags()`, `Generate()`, `loadTemplates()`, `ParseMarkdownFile()`
  - Added documented `#nosec` comments for all 41 gosec false positives (CLI tool with trusted inputs)
  - All quality checks pass: golangci-lint, gosec, gocyclo (<15)

### Added
- 🛡️ **OpenSSF Scorecard badge** - Security posture visibility in README

## [1.4.7] - 2026-01-29

### Added
- ✨ **Pretty HTML output** (`--pretty-html`) - Clean up generated HTML without minification
  - Removes excessive blank lines (collapses to max 1 between elements)
  - Removes whitespace-only lines
  - Removes trailing whitespace from lines
  - Keeps readable formatting, not aggressive like minify
  - Also available as `--pretty` shorthand
  - Config file option: `pretty_html: true`

## [1.4.6] - 2026-01-23

### Fixed
- 🐛 **Homepage overwriting prevention** - Pages with `link` field pointing to root URL no longer overwrite the main index.html
  - Generator now skips pages that would generate to root path with a warning
  - Displays hint to change the `link` field or use a different slug
  - Fixes: imd.agency frontpage showing raw content instead of designed homepage template

## [1.4.5] - 2026-01-23

### Fixed
- 🐛 **WordPress metadata parsing** - Handle `width`/`height` as string or int
  - Added `FlexInt` type for flexible JSON unmarshaling
  - Fixes: `json: cannot unmarshal string into Go struct field .media.media_details.width of type int`

## [1.4.4] - 2026-01-18

### Changed
- 📝 **Complete README overhaul** - Hugo-style comprehensive documentation
  - Added detailed Overview section
  - "What Can You Build?" guide with use cases
  - Key Capabilities table
  - Development Workflow documentation
  - Asset Processing details
  - Reorganized Features into categories

## [1.4.3] - 2026-01-18

### Fixed
- 🔧 **Example workflow moved** - `example-deploy.yml` moved to `examples/workflows/`
  - No longer runs on every push to main
  - Users copy it to their own `.github/workflows/`

### Added
- 📁 **Examples directory** - `examples/workflows/` with complete workflow templates
- 📝 Examples README with usage instructions

## [1.4.2] - 2026-01-18

### Fixed
- 🐳 **Docker build optimization** - Only builds on full semver tags (v1.4.2), not major version alias (v1)
- 📄 **Jekyll compatibility** - Escaped Liquid syntax in README.md for GitHub Pages

### Changed
- 🔧 **Code quality** - Refactored main() to reduce cyclomatic complexity (25 → 18)
- 📝 Added LICENSE.md for better Go Report Card detection

## [1.4.1] - 2026-01-18

### Added
- ✅ **Test coverage** for new packages:
  - `engine`: 61.6% coverage
  - `config`: 79.2% coverage
  - `theme`: 26.1% coverage
- 📝 **SECURITY.md** - Security policy and best practices
- 👥 **CONTRIBUTORS.md** - Contribution guidelines
- 🎨 **Template examples** for all engines (pongo2, mustache, handlebars)

### Changed
- 🔄 Updated all dependencies to latest versions
- 📦 Updated GitHub Action with `engine` and `online-theme` inputs

## [1.4.0] - 2026-01-18

### Added
- 🔧 **Multiple template engines** - choose your preferred syntax:
  - `--engine=go` (default) - Go templates
  - `--engine=pongo2` - Jinja2/Django-like templates
  - `--engine=mustache` - Mustache templates
  - `--engine=handlebars` - Handlebars templates
- 🌍 **Online theme download** (`--online-theme=URL`):
  - Download Hugo themes from GitHub/GitLab
  - Support for direct ZIP URLs
  - Auto-extraction to templates directory

### Documentation
- Added comprehensive Template Engines section
- Template syntax comparison for all engines
- Examples for using online themes

## [1.3.4] - 2026-01-17

### Changed
- 📦 **WebP tools now installed automatically** in GitHub Action
  - No need to manually install `cwebp`
  - Works on Linux and macOS runners

## [1.3.3] - 2026-01-17

### Fixed
- 🐛 **Raw binaries now included in releases** - direct download works:
  - `curl -sL .../ssg-linux-amd64 -o ssg` ✅
  - `curl -sL .../ssg-darwin-arm64 -o ssg` ✅
  - `curl -sL .../ssg-windows-amd64.exe -o ssg.exe` ✅
- Fixed CI release job to include all artifact types (archives + raw binaries)

## [1.3.2] - 2026-01-17

### Fixed
- 🔧 **Simplified release asset naming** - removed version from filenames for easier downloads
  - Archives now named `ssg-linux-amd64.tar.gz` instead of `ssg-1.3.1-linux-amd64.tar.gz`
  - Raw binaries also available: `ssg-linux-amd64` (no extension)
- 🐛 Fixed GitHub Action download URL to match new asset naming
- ✅ Added HTTP status and content validation for binary downloads

## [1.3.1] - 2026-01-17

### Added
- 🐳 **Docker support** - minimal Alpine-based image (~15MB)
  - Multi-arch builds: `linux/amd64` and `linux/arm64`
  - Published to GitHub Container Registry: `ghcr.io/spagu/ssg`
  - Docker Compose configuration included
- 🔄 Docker CI workflow for automatic image builds

### Changed
- Reverted to `cwebp` for WebP conversion to support static builds and cross-compilation (removed CGO dependency)
- Changed license to BSD 3-Clause
- ⚡ **GitHub Action now downloads pre-built binary** instead of building from source (much faster!)
  - Added `version` input to specify SSG version
  - Added `minify` and `clean` inputs

### Documentation
- Added Docker installation and usage examples
- Updated GitHub Actions versioning documentation
- Updated License badge
- Added Code of Conduct

## [1.3.0] - 2026-01-17

### Added
- 🌐 **Built-in HTTP server** (`--http` flag) - no need for external Python/Node server
- 🔌 **Custom port** (`--port=PORT`) - default: 8888
- 👀 **Watch mode** (`--watch` flag) - auto-rebuild on file changes (with error recovery)
- 📄 **Config file support** (`--config`) - load settings from YAML, TOML, or JSON
  - Auto-detects `.ssg.yaml`, `.ssg.toml`, `.ssg.json`
  - All CLI flags available in config file
- 🖼️ **WebP conversion** (`--webp`) - requires `cwebp` installed
  - `--webp-quality=N` - compression level 1-100 (default: 60)
- 📝 `stripHTML` template function for clean meta descriptions
- 🧹 **Clean build** (`--clean`) - clean output directory before build
- 🔇 **Quiet mode** (`--quiet`, `-q`) - suppress output, only exit codes
- 🗺️ **Sitemap control** (`--sitemap-off`) - disable sitemap.xml generation
- 🤖 **Robots control** (`--robots-off`) - disable robots.txt generation
- 🗜️ **Minification options**:
  - `--minify-all` - minify HTML, CSS, and JS
  - `--minify-html` - minify only HTML
  - `--minify-css` - minify only CSS
  - `--minify-js` - minify only JS
- 🗂️ **Source maps** (`--sourcemap`) - include source maps in output
- ℹ️ **Version flag** (`--version`, `-v`) - show version info
- ❓ **Help flag** (`--help`, `-h`) - show usage help
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
- WebP conversion now uses native Go library (removed `cwebp` dependency)
- Config package for loading settings from files

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
