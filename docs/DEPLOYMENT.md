# Deployment guide

SSG can package a generated site, publish it directly, or run as a GitHub
Action. Deployment always happens after generation and enabled post-processing,
so providers receive the final output tree.

## Production build

A conservative production command is:

```bash
ssg my-blog simple example.com \
  --clean --minify-all --fingerprint --check-links=strict
```

Review `output/` locally before enabling native deployment. Deployment does not
replace validation of provider configuration, redirects, custom domains or DNS.

## Archives

Archives are written beside the project using the configured domain as their
base filename:

| Configuration | CLI | Output |
|---|---|---|
| `zip: true` | `--zip` | `<domain>.zip` |
| `targz: true` | `--targz` | `<domain>.tar.gz` |
| `tarxz: true` | `--tarxz` | `<domain>.tar.xz` |

Multiple archive formats can be enabled in one build. Archives contain the
output tree and can be uploaded manually to any static host.

## Native deployment model

```yaml
deploy: cloudflare
deploy_project: my-site
deploy_branch: main
deploy_target: ""
```

Equivalent CLI flags are `--deploy`, `--deploy-project`, `--deploy-branch` and
`--deploy-target`.

Secrets are read from environment variables. Do not put tokens, passwords or
private keys in `.ssg.yaml`, Markdown content, command history or a committed
workflow.

| Provider | `deploy` value | Project | Target | Credentials |
|---|---|---|---|---|
| Cloudflare Pages | `cloudflare` | Pages project name | — | `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID` |
| GitHub Pages | `github-pages` | — | Git remote; defaults to `origin` | `GITHUB_TOKEN` for HTTPS or normal Git/SSH credentials |
| Netlify | `netlify` | Site ID | — | `NETLIFY_AUTH_TOKEN` |
| Vercel | `vercel` | Project ID/name | — | `VERCEL_TOKEN`; optional `VERCEL_ORG_ID` |
| FTP | `ftp` | — | `ftp://[user@]host[:port]/path` | `FTP_USERNAME`, `FTP_PASSWORD` |
| SFTP | `sftp` | — | `sftp://[user@]host[:port]/path` | SSH environment described below |

Accepted aliases include `cloudflare-pages`, `github`, `gh-pages` and `ssh`,
but canonical names are recommended in durable configuration.

## Cloudflare Pages

Create the Pages project first and use an API token with permission to edit it:

```bash
export CLOUDFLARE_API_TOKEN=...
export CLOUDFLARE_ACCOUNT_ID=...

ssg my-blog simple example.com \
  --deploy=cloudflare \
  --deploy-project=my-site
```

`--deploy-branch` optionally selects the Pages branch. SSG uses Cloudflare's
Direct Upload API, hashes the output manifest and uploads the required files; it
does not require Wrangler.

### Generated `_headers` and `_redirects`

**Every build** writes `_headers` and `_redirects` into the output directory,
whether or not you deploy to Cloudflare Pages — they are inert anywhere else.
Pages applies them verbatim, so their content is the site's security and
caching policy, and worth knowing before a page appears to go stale.

Security headers, applied to `/*`:

| Header | Value |
|---|---|
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `X-XSS-Protection` | `1; mode=block` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | `geolocation=(), microphone=(), camera=()` |

Cache policy:

| Path | `Cache-Control` |
|---|---|
| `/css/*`, `/js/*`, `/images/*`, `/media/*` | `public, max-age=31536000, immutable` |
| `/*.html` and `/` | `public, max-age=3600` |

The one-year `immutable` on assets assumes their filenames change when their
contents do. That is exactly what `--fingerprint` and the image pipeline's
content-addressed names guarantee. **Without fingerprinting, a CSS or JS file
edited in place keeps its name and can be served from a browser cache for a
year** — enable `fingerprint: true` for any site that deploys more than once,
or ship your own `_headers`.

`_redirects` carries the `aliases:` declared in frontmatter as `301`s, plus a
header explaining that trailing-slash normalisation is Cloudflare's job.

#### Overriding them

The generated files are written **after** `static/` is copied, so a `_headers`
of your own placed in `static/` is overwritten rather than merged. To take
control of the policy, deploy with a post-build hook that rewrites the file, or
keep the generated defaults and layer per-path rules in the Cloudflare
dashboard.

#### A page still shows its old content

HTML is cached at the edge for an hour. After renaming a page — changing
`post_url_format`, adding a `permalinks` pattern — the **old** URL keeps
answering `200` from cache for up to that hour even though the new deployment
no longer contains it, and the new URL is live immediately. Check with
`curl -sI <url> | grep -i age` before concluding the deploy failed; purge the
cache in the Cloudflare dashboard if you cannot wait.

### This project's own documentation site

`ssg.tradik.com` is built from this repository by
[`.github/workflows/docs-site.yml`](../.github/workflows/docs-site.yml) and is
a working example of everything above:

- The site has **no content tree**. `docs-site.yaml` pulls `docs/` in through
  `content_sources`, so editing a guide is the whole publishing workflow.
- It is built with the `ssg` binary **from the commit being deployed**, not the
  released action, so the site doubles as an integration test of `main` on real
  content — and so it can use configuration keys newer than the last release.
- `shortcode_errors: strict` and `--check-links=strict` gate the deploy: an
  unrenderable shortcode or a dead internal link fails the run, and SSG deploys
  only after a successful generate, so a broken page never reaches the CDN.
- `cwebp` is installed in the runner because the hero image is encoded as WebP.

Setup is two repository secrets and nothing else:

| Secret / variable | Required | Purpose |
|---|---|---|
| `CLOUDFLARE_API_TOKEN` | yes | *Cloudflare Pages: Edit*. Attaching the custom domain additionally needs *Zone:DNS:Edit* on that zone. |
| `CLOUDFLARE_ACCOUNT_ID` | yes | Account that owns the project |
| `CLOUDFLARE_PAGES_PROJECT` | no | Pages project name (default `ssg-docs`) |
| `DOCS_DOMAIN` | no | Custom domain (default `ssg.tradik.com`) |

The workflow creates the Pages project and attaches the custom domain on its
first run, and leaves both alone afterwards — there is nothing to click in the
dashboard. Domain attachment is deliberately non-fatal: the deployment is
already live on `<project>.pages.dev` by then, and the usual failure is a token
without `Zone:DNS:Edit`, which is a permissions decision rather than a build
problem. The job summary says which of the two happened.

A pull request runs the same build with `--check-links=strict` and stops there:
nothing is created in Cloudflare and nothing is uploaded, but a dead link or an
unrenderable shortcode in a new post fails the check before review rather than
after the merge. Only a push to the default branch deploys.

`workflow_dispatch` only becomes available once the workflow file is on the
default branch, so the first manual run happens after a merge.

## GitHub Pages

```bash
export GITHUB_TOKEN=...

ssg my-blog simple example.com \
  --deploy=github-pages \
  --deploy-target=https://github.com/example/site.git \
  --deploy-branch=gh-pages
```

The target defaults to the current repository's `origin`, and the branch
defaults to `gh-pages`. SSG creates an isolated Git repository inside the output
directory, commits the generated tree and force-pushes a single commit.

> `github-pages` intentionally rewrites the target branch history. Use a branch
> dedicated to generated output, never a source or shared development branch.

For HTTPS remotes, `GITHUB_TOKEN` is passed as an HTTP authorization header and
is not embedded in the remote URL. SSH remotes use the normal Git/SSH setup.
The `git` executable must be available.

## Netlify

```bash
export NETLIFY_AUTH_TOKEN=...

ssg my-blog simple example.com \
  --deploy=netlify \
  --deploy-project=your-site-id
```

`NETLIFY_SITE_ID` may replace `--deploy-project`. SSG declares a digest manifest
through Netlify's deploy API and uploads only files Netlify reports missing.
The Netlify CLI is not required.

## Vercel

```bash
export VERCEL_TOKEN=...
export VERCEL_ORG_ID=...

ssg my-blog simple example.com \
  --deploy=vercel \
  --deploy-project=my-project
```

`VERCEL_PROJECT_ID` may replace `--deploy-project`; `VERCEL_ORG_ID` selects the
team scope and is optional for an unscoped account. SSG uploads content-addressed
files and creates a production deployment. The Vercel CLI is not required.

## FTP

```bash
export FTP_USERNAME=deploy
export FTP_PASSWORD=...

ssg my-blog simple example.com \
  --deploy=ftp \
  --deploy-target=ftp://ftp.example.com/public_html
```

The username may be included in the target URL; otherwise `FTP_USERNAME` is
used, falling back to `anonymous`. The default port is 21. SSG creates remote
directories where possible and uploads every regular output file.

FTP does not encrypt credentials or content. Prefer SFTP when the host supports
it.

## SFTP

Password authentication:

```bash
export SSH_USERNAME=deploy
export SSH_PASSWORD=...

ssg my-blog simple example.com \
  --deploy=sftp \
  --deploy-target=sftp://server.example.com/var/www/site
```

Key authentication:

```bash
export SSH_KEY_FILE="$HOME/.ssh/id_ed25519"
export SSH_KEY_PASSPHRASE=...

ssg my-blog simple example.com \
  --deploy=sftp \
  --deploy-target=sftp://deploy@server.example.com/var/www/site
```

SFTP variables:

| Variable | Meaning |
|---|---|
| `SSH_USERNAME` | Used when the URL has no username |
| `SSH_PASSWORD` | Password authentication; takes priority over a key |
| `SSH_KEY_FILE` | Private key; defaults to `~/.ssh/id_rsa` |
| `SSH_KEY_PASSPHRASE` | Optional encrypted-key passphrase |
| `SSH_KNOWN_HOSTS` | Host database; defaults to `~/.ssh/known_hosts` |

The default port is 22. Host keys are always verified; unknown hosts are
rejected. Add the expected key out of band, for example after independently
checking its fingerprint:

```bash
ssh-keyscan server.example.com >> ~/.ssh/known_hosts
```

## GitHub Action

Use the stable major reference to receive compatible v1 updates:

```yaml
name: Build site

on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v6
      - name: Build with SSG
        id: ssg
        uses: spagu/ssg@v1
        with:
          source: my-blog
          template: simple
          domain: example.com
          clean: "true"
          minify: "true"
```

Pin a full released tag instead of `@v1` when reproducibility is more important
than automatic compatible updates. Use `@main` only to test unreleased changes.

**Pin the binary too.** The action's `version` input defaults to `latest`, so
every deploy silently picks up the newest ssg release the moment it ships —
including behaviour changes. For production sites, pin it:

```yaml
- uses: spagu/ssg@v1
  with:
    version: v1.8.5   # exact ssg release used for the build
```

Since v1.8.5 the action logs the resolved version on every run (a `::notice::`
when `latest` was used) and exposes it as the `version` output, so unpinned
builds are at least traceable.

### Action inputs

The action intentionally exposes a stable subset of the complete CLI:

| Input | Required | Default | Meaning |
|---|---:|---|---|
| `source` | yes | — | Content source name |
| `template` | yes | `simple` | Theme name |
| `domain` | yes | — | Canonical host |
| `version` | no | `latest` | Binary release to download |
| `content-dir` | no | `content` | Content root |
| `templates-dir` | no | `templates` | Theme root |
| `output-dir` | no | `output` | Generated destination |
| `webp` | no | `false` | Enable WebP conversion |
| `webp-quality` | no | `60` | WebP quality |
| `zip` | no | `false` | Create ZIP archive |
| `minify` | no | `false` | Enable all minification |
| `clean` | no | `false` | Clean before build |
| `engine` | no | `go` | Template engine |
| `online-theme` | no | empty | Theme download URL |
| `deploy` | no | empty | Native provider |
| `deploy-project` | no | empty | Provider project/site |
| `deploy-branch` | no | empty | Provider branch |
| `deploy-target` | no | empty | Git/FTP/SFTP target |

Inputs are passed through environment variables and validated before being
added to the command argument array.

### Action outputs

| Output | Meaning |
|---|---|
| `output-path` | Generated site directory |
| `zip-file` | ZIP path when `zip` is enabled |
| `zip-size` | ZIP size in bytes |
| `version` | Resolved ssg version used for the build (GO-052) |

Example artifact upload:

```yaml
- uses: actions/upload-pages-artifact@v4
  with:
    path: ${{ steps.ssg.outputs.output-path }}
```

### Native deployment from Actions

```yaml
- uses: spagu/ssg@v1
  with:
    source: my-blog
    template: simple
    domain: example.com
    clean: "true"
    minify: "true"
    deploy: cloudflare
    deploy-project: my-site
  env:
    CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
    CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
```

Provider credentials must be repository or environment secrets referenced in
`env`, not action inputs. A complete example is available at
[examples/workflows/cloudflare-pages.yml](../examples/workflows/cloudflare-pages.yml).

## Deployment checklist

- Run a clean production build.
- Run `--check-links=strict`.
- Inspect generated canonical URLs and redirects.
- Confirm the provider project/site ID.
- Store credentials only in the runtime environment or CI secret store.
- Use a least-privilege token.
- For GitHub Pages, confirm the target is a disposable generated branch.
- For SFTP, verify and pin the server host key.
- Test custom domains, HTTPS and cache headers after publishing.

