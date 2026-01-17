# SSG - Static Site Generator

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Go Report Card](https://goreportcard.com/badge/github.com/spagu/ssg)](https://goreportcard.com/report/github.com/spagu/ssg)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)
[![CI](https://github.com/spagu/ssg/actions/workflows/ci.yml/badge.svg)](https://github.com/spagu/ssg/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/spagu/ssg/branch/main/graph/badge.svg)](https://codecov.io/gh/spagu/ssg)
[![GitHub Action](https://img.shields.io/badge/GitHub_Action-Available-2088FF?logo=github-actions&logoColor=white)](action.yml)
[![GitHub issues](https://img.shields.io/github/issues/spagu/ssg)](https://github.com/spagu/ssg/issues)
[![GitHub stars](https://img.shields.io/github/stars/spagu/ssg)](https://github.com/spagu/ssg/stargazers)
[![GitHub Release](https://img.shields.io/github/v/release/spagu/ssg?style=flat&color=blue)](https://github.com/spagu/ssg/releases)
[![Docker](https://github.com/spagu/ssg/actions/workflows/docker.yml/badge.svg)](https://github.com/spagu/ssg/actions/workflows/docker.yml)
[![GitHub forks](https://img.shields.io/github/forks/spagu/ssg)](https://github.com/spagu/ssg/network)

🐄 **SSG** - A simple static site generator written in Go. Converts content from WordPress exports (Markdown format with YAML frontmatter) to static HTML, CSS, and JS files.

## 📋 Table of Contents

- [Features](#-features)
- [Requirements](#-requirements)
- [Installation](#-installation)
- [Usage](#-usage)
- [GitHub Actions](#-github-actions)
- [Project Structure](#-project-structure)
- [Templates](#-templates)
- [Styles/Colors](#-stylescolors)
- [Architecture](#-architecture)
- [Testing](#-testing)
- [Development](#-development)

## ✨ Features

- 🚀 Fast static site generation
- 📝 Markdown support with YAML frontmatter
- 🎨 Two templates: **simple** (dark theme) and **krowy** (green/natural theme)
- 📱 Responsive design
- ♿ WCAG 2.2 compliant
- 🔍 SEO-friendly URLs (clean addresses)
- 📁 Automatic media file copying
- 🏷️ Category support
- 📄 **Config file support** (YAML, TOML, JSON)
- 🌐 **Built-in HTTP server** (`--http` flag)
- 👀 **Watch mode** - auto-rebuild on file changes (`--watch` flag)
- 🖼️ **WebP conversion** (`--webp` flag)
- 🗄️ **Minification** - HTML, CSS, JS (`--minify-all` flag)
- 🧹 **Clean builds** (`--clean` flag)
- 📦 Cloudflare Pages deployment package (`--zip` flag)
- 🐳 **Docker support** - minimal Alpine image (~15MB)
- 🎬 **GitHub Actions integration** - Use as a step in CI/CD pipelines

## 📦 Requirements

- Go 1.25 or later
- Make (optional, for Makefile)
- `cwebp` (optional, for WebP conversion)

## 🚀 Installation

### Quick Install (Linux/macOS)

```bash
curl -sSL https://raw.githubusercontent.com/spagu/ssg/main/install.sh | bash
```

### Package Managers

| Platform | Command |
|----------|---------|
| **Homebrew** (macOS/Linux) | `brew install spagu/tap/ssg` |
| **Snap** (Ubuntu) | `snap install ssg` |
| **Debian/Ubuntu** | `wget https://github.com/spagu/ssg/releases/download/v1.3.0/ssg_1.3.0_amd64.deb && sudo dpkg -i ssg_1.3.0_amd64.deb` |
| **Fedora/RHEL** | `sudo dnf install https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-1.x86_64.rpm` |
| **FreeBSD** | `pkg install ssg` or from ports |
| **OpenBSD** | From ports: `/usr/ports/www/ssg` |

### Binary Downloads

Download pre-built binaries from [GitHub Releases](https://github.com/spagu/ssg/releases):

| Platform | AMD64 | ARM64 |
|----------|-------|-------|
| Linux | [ssg-linux-amd64.tar.gz](https://github.com/spagu/ssg/releases/latest) | [ssg-linux-arm64.tar.gz](https://github.com/spagu/ssg/releases/latest) |
| macOS | [ssg-darwin-amd64.tar.gz](https://github.com/spagu/ssg/releases/latest) | [ssg-darwin-arm64.tar.gz](https://github.com/spagu/ssg/releases/latest) |
| FreeBSD | [ssg-freebsd-amd64.tar.gz](https://github.com/spagu/ssg/releases/latest) | [ssg-freebsd-arm64.tar.gz](https://github.com/spagu/ssg/releases/latest) |
| Windows | [ssg-windows-amd64.zip](https://github.com/spagu/ssg/releases/latest) | [ssg-windows-arm64.zip](https://github.com/spagu/ssg/releases/latest) |

### From Source

```bash
git clone https://github.com/spagu/ssg.git
cd ssg
make build
sudo make install
```

### Docker

```bash
# Pull image from GitHub Container Registry
docker pull ghcr.io/spagu/ssg:latest

# Run SSG in container
docker run --rm -v $(pwd):/site ghcr.io/spagu/ssg:latest \
    my-content krowy example.com --webp

# Or use docker-compose
docker compose run --rm ssg my-content krowy example.com

# Development server with watch mode
docker compose up dev
```

📖 **Full installation guide:** [docs/INSTALL.md](docs/INSTALL.md)

## 💻 Usage

### Syntax

```bash
ssg <source> <template> <domain> [options]
```

### Arguments

| Argument | Description |
|----------|-------------|
| `source` | Source folder name (inside content-dir) |
| `template` | Template name (inside templates-dir) |
| `domain` | Target domain for the generated site |

### Configuration File

SSG supports configuration files in YAML, TOML, or JSON format. Auto-detects: `.ssg.yaml`, `.ssg.toml`, `.ssg.json`

```bash
# Use explicit config file
ssg --config .ssg.yaml

# Or just create .ssg.yaml and run ssg (auto-detected)
ssg
```

Example `.ssg.yaml`:

```yaml
source: "my-content"
template: "krowy"
domain: "example.com"

http: true
watch: true
clean: true
webp: true
webp_quality: 80
minify_all: true
```

See [.ssg.yaml.example](.ssg.yaml.example) for all options.

### Options

**Configuration:**

| Option | Description |
|--------|-------------|
| `--config=FILE` | Load config from YAML/TOML/JSON file |

**Server & Development:**

| Option | Description |
|--------|-------------|
| `--http` | Start built-in HTTP server (default port: 8888) |
| `--port=PORT` | HTTP server port (default: `8888`) |
| `--watch` | Watch for changes and rebuild automatically |
| `--clean` | Clean output directory before build |

**Output Control:**

| Option | Description |
|--------|-------------|
| `--sitemap-off` | Disable sitemap.xml generation |
| `--robots-off` | Disable robots.txt generation |
| `--minify-all` | Minify HTML, CSS, and JS |
| `--minify-html` | Minify HTML output |
| `--minify-css` | Minify CSS output |
| `--minify-js` | Minify JS output |
| `--sourcemap` | Include source maps in output |

**Image Processing (Native Go - no external tools needed):**

| Option | Description |
|--------|-------------|
| `--webp` | Convert images to WebP format (requires `cwebp`) |
| `--webp-quality=N` | WebP compression quality 1-100 (default: `60`) |

**Deployment:**

| Option | Description |
|--------|-------------|
| `--zip` | Create ZIP file for Cloudflare Pages |

**Paths:**

| Option | Description |
|--------|-------------|
| `--content-dir=PATH` | Content directory (default: `content`) |
| `--templates-dir=PATH` | Templates directory (default: `templates`) |
| `--output-dir=PATH` | Output directory (default: `output`) |

**Other:**

| Option | Description |
|--------|-------------|
| `--quiet`, `-q` | Suppress output (only exit codes) |
| `--version`, `-v` | Show version |
| `--help`, `-h` | Show help |

### Examples

```bash
# Development mode: HTTP server + auto-rebuild on changes
./build/ssg my-content krowy example.com --http --watch

# HTTP server on custom port
./build/ssg my-content krowy example.com --http --port=3000

# Generate site with krowy template
./build/ssg krowy.net.2026-01-13110345 krowy krowy.net

# Generate with simple template (dark theme)
./build/ssg krowy.net.2026-01-13110345 simple krowy.net

# Generate with WebP conversion and ZIP package
./build/ssg krowy.net.2026-01-13110345 krowy krowy.net --webp --zip

# Use custom directories
./build/ssg my-content my-template example.com \
  --content-dir=/data/content \
  --templates-dir=/data/templates \
  --output-dir=/var/www/html

# Or using Makefile
make generate        # krowy template
make generate-simple # simple template
make serve           # generate and run local server
make deploy          # generate with WebP + ZIP for Cloudflare Pages
```

### Output

Generated files will be in the `output/` folder:

```
output/
├── index.html          # Homepage
├── css/
│   └── style.css       # Stylesheet
├── js/
│   └── main.js         # JavaScript
├── media/              # Media files
├── {slug}/             # Pages and posts (SEO URLs)
│   └── index.html
├── category/
│   └── {category-slug}/
│       └── index.html
├── sitemap.xml         # Sitemap for search engines
├── robots.txt          # Robots file
├── _headers            # Cloudflare Pages headers
└── _redirects          # Cloudflare Pages redirects
```

## 🎬 GitHub Actions

Use SSG as a GitHub Action in your CI/CD pipeline:

### Versioning

| Reference | Description |
|-----------|-------------|
| `spagu/ssg@main` | Latest from main branch (development) |
| `spagu/ssg@v1` | Latest stable v1.x release |
| `spagu/ssg@v1.3.0` | Specific version |

> **Note:** Use `@main` until a stable release is published.

### Basic Usage

```yaml
- name: Generate static site
  uses: spagu/ssg@main  # or @v1 after release
  with:
    source: 'my-content'
    template: 'krowy'
    domain: 'example.com'
```

### Full Configuration

```yaml
- name: Generate static site
  id: ssg
  uses: spagu/ssg@v1
  with:
    source: 'my-content'           # Content folder (inside content/)
    template: 'krowy'              # Template: 'simple' or 'krowy'
    domain: 'example.com'          # Target domain
    content-dir: 'content'         # Optional: content directory path
    templates-dir: 'templates'     # Optional: templates directory path
    output-dir: 'output'           # Optional: output directory path
    webp: 'true'                   # Optional: convert images to WebP
    webp-quality: '80'             # Optional: WebP quality 1-100 (default: 60)
    zip: 'true'                    # Optional: create ZIP for deployment

- name: Show outputs
  run: |
    echo "Output path: ${{ steps.ssg.outputs.output-path }}"
    echo "ZIP file: ${{ steps.ssg.outputs.zip-file }}"
    echo "ZIP size: ${{ steps.ssg.outputs.zip-size }} bytes"
```

### Action Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `source` | Content source folder name | ✅ | - |
| `template` | Template name | ✅ | `simple` |
| `domain` | Target domain | ✅ | - |
| `content-dir` | Path to content directory | ❌ | `content` |
| `templates-dir` | Path to templates directory | ❌ | `templates` |
| `output-dir` | Path to output directory | ❌ | `output` |
| `webp` | Convert images to WebP | ❌ | `false` |
| `webp-quality` | WebP compression quality 1-100 | ❌ | `60` |
| `zip` | Create ZIP file | ❌ | `false` |

### Action Outputs

| Output | Description |
|--------|-------------|
| `output-path` | Path to generated site directory |
| `zip-file` | Path to ZIP file (if --zip used) |
| `zip-size` | Size of ZIP file in bytes |

### Deploy to Cloudflare Pages

```yaml
name: Deploy

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - name: Generate site
        id: ssg
        uses: spagu/ssg@v1
        with:
          source: 'my-content'
          template: 'krowy'
          domain: 'example.com'
          webp: 'true'

      - name: Deploy to Cloudflare
        uses: cloudflare/pages-action@v1
        with:
          apiToken: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          accountId: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
          projectName: 'my-site'
          directory: ${{ steps.ssg.outputs.output-path }}
```

## 📁 Project Structure

```
ssg/
├── cmd/
│   └── ssg/
│       └── main.go           # CLI entry point
├── internal/
│   ├── generator/
│   │   ├── generator.go      # Generator logic
│   │   ├── generator_test.go # Generator tests
│   │   └── templates.go      # Default HTML templates
│   ├── models/
│   │   └── content.go        # Data models
│   └── parser/
│       ├── markdown.go       # Markdown parser
│       └── markdown_test.go  # Parser tests
├── content/                  # Source data
│   └── {source}/
│       ├── metadata.json
│       ├── media/
│       ├── pages/
│       └── posts/
├── templates/                # Templates
│   ├── simple/
│   │   ├── css/
│   │   └── js/
│   └── krowy/
│       ├── css/
│       └── js/
├── output/                   # Generated site (gitignored)
├── go.mod
├── go.sum
├── Makefile
├── README.md
├── CHANGELOG.md
├── .gitignore
└── .dockerignore
```

## 🎨 Templates

### simple - Modern Dark Theme

Elegant dark theme with glassmorphism and gradients:
- Dark background: `#0f0f0f`
- Cards: `#222222`
- Accent: purple gradient `#6366f1` → `#a855f7`
- Hover animations and micro-interactions

### krowy - Green Farm Theme

Natural light theme inspired by krowy.net:
- Light background: `#f8faf5`
- Cards: `#ffffff`
- Accent: green `#2d7d32`
- Cow icon 🐄 in logo
- Nature and ecology focus

## 🎨 Styles/Colors

### Color Guidelines (WCAG 2.2 Compliant)

#### Simple Template (Dark)
```css
/* Background */
--color-bg-primary: #0f0f0f;
--color-bg-secondary: #1a1a1a;
--color-bg-card: #222222;

/* Text (minimum contrast 4.5:1) */
--color-text-primary: #ffffff;
--color-text-secondary: #b3b3b3;
--color-text-muted: #808080;

/* Accent */
--color-accent: #6366f1;
--gradient-primary: linear-gradient(135deg, #6366f1 0%, #8b5cf6 50%, #a855f7 100%);
```

#### Krowy Template (Light)
```css
/* Background */
--color-bg-primary: #f8faf5;
--color-bg-secondary: #ffffff;
--color-bg-card: #ffffff;

/* Text (minimum contrast 4.5:1) */
--color-text-primary: #1a2e1a;
--color-text-secondary: #3d5a3d;
--color-text-muted: #6b8a6b;

/* Accent */
--color-accent: #2d7d32;
--gradient-primary: linear-gradient(135deg, #2d7d32 0%, #43a047 50%, #66bb6a 100%);
```

Detailed style documentation: [docs/STYLES.md](docs/STYLES.md)

## 🏗️ Architecture

```mermaid
flowchart TB
    subgraph Input["📥 Input"]
        A[content/source] --> B[metadata.json]
        A --> C[pages/*.md]
        A --> D[posts/**/*.md]
        A --> E[media/*]
    end

    subgraph Processing["⚙️ Processing"]
        F[Parser] --> G[Models]
        G --> H[Generator]
        T[Templates] --> H
    end

    subgraph Output["📤 Output"]
        H --> I[output/]
        I --> J[index.html]
        I --> K[pages/]
        I --> L[posts/]
        I --> M[category/]
        I --> N[css/]
        I --> O[js/]
        I --> P[media/]
    end

    B --> F
    C --> F
    D --> F
    E --> P
```

## 🧪 Testing

```bash
# Run all tests
make test

# Tests with coverage
make test-coverage

# Open coverage report
open coverage.html
```

## 🛠️ Development

### Available Make Commands

```bash
make help           # Show all commands
make all            # deps + lint + test + build
make build          # Build binary
make test           # Run tests
make lint           # Check code
make run            # Build and run
make generate       # Generate site (krowy template)
make generate-simple # Generate site (simple template)
make serve          # Generate and serve locally
make deploy         # Generate with WebP + ZIP for Cloudflare Pages
make clean          # Clean artifacts
make install        # Install binary to /usr/local/bin
```

### Creating Your Own Template

1. Create a folder in `templates/your-template-name/`
2. Add files:
   - `css/style.css`
   - `js/main.js` (optional)
   - `index.html`, `page.html`, `post.html`, `category.html` (optional)
3. HTML templates are generated automatically if missing

## 📄 License

BSD 3-Clause License - see [LICENSE](LICENSE)

## 👥 Authors

- **spagu** - [GitHub](https://github.com/spagu)
