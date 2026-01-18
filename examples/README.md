# SSG Workflow Examples

This directory contains example GitHub Actions workflows for deploying sites built with SSG.

## Available Examples

### [`cloudflare-pages.yml`](workflows/cloudflare-pages.yml)

Deploy your static site to Cloudflare Pages.

**Prerequisites:**
- Cloudflare account
- Cloudflare Pages project created
- GitHub secrets configured:
  - `CLOUDFLARE_API_TOKEN` - Your Cloudflare API token
  - `CLOUDFLARE_ACCOUNT_ID` - Your Cloudflare account ID

**Usage:**
1. Copy the workflow to your repository's `.github/workflows/` directory
2. Update the configuration values:
   - `source` - Your content folder name
   - `template` - Template to use (`simple` or `krowy`)
   - `domain` - Your target domain
   - `projectName` - Your Cloudflare Pages project name

## Using SSG Action

All examples use the SSG GitHub Action:

```yaml
- uses: spagu/ssg@v1  # Use latest stable v1.x
  with:
    source: 'my-content'
    template: 'krowy'
    domain: 'example.com'
```

### Available Inputs

| Input | Description | Default |
|-------|-------------|---------|
| `source` | Content folder name | *required* |
| `template` | Template name | `simple` |
| `domain` | Target domain | *required* |
| `webp` | Convert images to WebP | `false` |
| `webp-quality` | WebP quality (1-100) | `60` |
| `zip` | Create ZIP for deployment | `false` |
| `minify` | Minify HTML/CSS/JS | `false` |
| `clean` | Clean output before build | `false` |
| `engine` | Template engine | `go` |
| `online-theme` | Download theme from URL | - |

### Available Outputs

| Output | Description |
|--------|-------------|
| `output-path` | Path to generated site |
| `zip-file` | Path to ZIP file (if created) |
| `zip-size` | Size of ZIP in bytes |
