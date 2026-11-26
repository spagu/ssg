# What a build reads

This page lists what a one-shot build (`ssg --config <file>`, no `--http`, no
`--watch`) takes from outside its source tree: the network, the environment,
configuration files and programs on `PATH`. It is the reference for running
ssg in a sandbox, a container without network, or a reproducible pipeline.

Everything here describes ssg 1.9.16. Subcommands that exist to talk to a
server (`ssg mcp`, `ssg edit`, `ssg migrate`, `ssg self-update-check`,
`ssg mddb …`) and the `--http` server are outside its scope, except where a
variable is listed for completeness.

## Network

**A one-shot build makes no network request unless one of the settings below
asks for one.** There is no telemetry and no update check: the version check
runs only as `ssg self-update-check`.

| Setting | Request made | When |
|---|---|---|
| `online_theme` | Downloads the theme archive | Every build with the key set |
| `include:` with a `url:` entry | Fetches that YAML file | While the config loads |
| `workers[].source` holding a URL | Downloads the worker archive | Only while the worker's directory is missing or empty |
| `external_sources` with an HTTP, SQL or CMS source | Queries the API or database | Per source, subject to its cache; `--offline` serves from the cache only |
| `mddb.enabled: true` | Loads content from the MDDB server, and answers the `related` helper's MDDB queries | Every build |
| `tts.enabled: true` | Sends article text to `tts.api_url`; fetches `tts.jingle_url` once | For audio not already in the cache |
| `ai:` with an `[ai …]` shortcode in content | Calls the model's `url` | For answers not already in the cache |
| `notify: true` with `notifications:` (or `--notify`) | Calls each destination | After the build, for new posts |
| `deploy:` / `--deploy=…` | Uploads the output | After the build |
| `hooks:` | Whatever the hook's program does | ssg does not restrict it |

Notes:

- Links, images, analytics snippets and fonts that a page references are
  written into the HTML as addresses. The build does not fetch them.
  `check_links` validates internal links against the output tree and makes no
  request.
- `og_cards`, `image_formats`, `--webp`, `scss` and `api_docs` work on local
  files only.
- Proxy variables are not applied uniformly. Requests for `online_theme`,
  remote `include:` files, worker archives, `tts`, `ai` and MDDB go through
  Go's default HTTP transport, which reads `HTTP_PROXY`, `HTTPS_PROXY` and
  `NO_PROXY`. Requests for `external_sources` and `notifications` use a
  hardened transport that connects directly and ignores them.

To confirm a given config builds without network, run it in a network
namespace with no interfaces, for example on Linux:

```bash
unshare -rn ssg --config site.yaml
```

## Environment variables

ssg has no global `SSG_*` settings: no variable switches a feature on or
changes a default. The environment is read only in the cases below.

### Read by a build

| Variable | Effect |
|---|---|
| `SOURCE_DATE_EPOCH` | Unix seconds. Pins the build timestamp (`.BuildTime` and what derives from it), in UTC. A value that is not a number is ignored. See [TEMPLATES.md](TEMPLATES.md). |
| Any name referenced as `$NAME` or `${NAME}` in `domain:` | Supplies the domain. An unset or empty variable stops the build. Not read when `--domain` is given. |
| Any name referenced as `$NAME` in `variables:` | Replaces that value. An unset or empty variable leaves the literal `$NAME` in place. |
| Any name referenced as `$NAME` in a secret field | Supplies the secret: `auth` of a remote `include:` or worker source, `external_sources` credentials, `tts.api_key`, `ai.models.*.key`, `notifications` headers. Read only when that feature is configured. |
| `CLOUDFLARE_ACCOUNT_ID`, `CLOUDFLARE_API_TOKEN`, `NETLIFY_SITE_ID`, `NETLIFY_AUTH_TOKEN`, `VERCEL_TOKEN`, `VERCEL_PROJECT_ID`, `VERCEL_ORG_ID`, `GITHUB_TOKEN`, `SSH_KEY_FILE`, `SSH_KEY_PASSPHRASE`, `SSH_KNOWN_HOSTS` | Deployment credentials, read by the matching `deploy:` target only. A build without `deploy:` does not read them. |
| `PATH` | Used to find the optional programs listed under [Programs](#programs). |
| `TMPDIR` | Where the archive of an `online_theme` or a remote worker source is downloaded before it is unpacked. Not used otherwise: image conversion writes its temporary files next to the output file. |
| `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` | Apply to some network requests; see [Network](#network). |
| `SNAP` | Only changes the wording of the error printed when a hook's program is missing. |

Not read by a build:

- **`TZ`.** Dates are converted only when `timezone` (or a per-language
  timezone) is configured, and the IANA zone database is compiled into the
  binary, so the system's zone files are not needed either.
- **`LANG`, `LC_*`.** Language, number and date formatting come from the
  configuration (`default_language`, the page's language), never from the
  locale.
- **`HOME`.** The cache lives in `.ssg-cache/` under the working directory
  unless the config names another directory. `HOME` is read only by SFTP
  deployment (for SSH keys and `known_hosts`) and by `--http` with automatic
  TLS (for the certificate cache).
- **`NO_COLOR`, `TERM`, `CI`.** Output is the same in a terminal and in a pipe.

### Set by a build

- Every entry of `variables:` is exported into the build's own process
  environment under an `SSG_` prefix, so hooks can read it.
- A hook's program receives the whole environment of the ssg process plus
  `SSG_OUTPUT_DIR`, `SSG_PHASE` and, for page hooks, `SSG_PAGE_PATH`. A caller
  that wants a hook to see less must start ssg with less.

### Read by other commands

| Variable | Read by |
|---|---|
| `SSG_MCP_TOKEN` | `ssg mcp --listen` |
| `SSG_EDIT_TOKEN` | `ssg edit` |
| `SSG_WPEXPORTER` | `ssg migrate` (path of the export engine) |
| `HOMEBREW_PREFIX` | `ssg self-update-check` (to name the install method) |
| `$NAME` references in `server_auth` and `endpoints` | `--http` |

## Configuration sources

A build is configured by exactly these, lowest priority first:

1. Built-in defaults.
2. Files named by `include:` in the config file, local or remote.
3. The config file: the one given to `--config`, or without the flag the first
   of `.ssg.yaml`, `.ssg.yml`, `.ssg.toml`, `.ssg.json`, `ssg.yaml`, `ssg.yml`,
   `ssg.toml`, `ssg.json` in the working directory.
4. `.ssg.designer.json` beside the config file, when the file exists (YAML
   configs only; it cannot use `include:`).
5. Command-line flags.

Details of each are in
[CONFIGURATION.md](CONFIGURATION.md#loading-configuration).

**A theme directory carries no configuration.** ssg takes templates,
partials and static assets from it, and reads none of its files as
configuration: no file inside a theme can set a config key, enable a network
source or declare a hook. There is no per-user or
system-wide config file either. To see the configuration a build will use,
with every source merged, run `ssg config view --effective`.

Two things a theme does control, both through the templates it ships: which
template helpers a page calls (the `related` helper queries MDDB only when
`mddb.enabled` is already true in the config), and what markup the pages
contain.

## Programs

A build starts another program only in these cases. Each is found through
`PATH` and run without a shell.

| Program | Started when | If it is missing |
|---|---|---|
| `cwebp` | `--webp` / `webp: true` | The build fails with a message naming the tool |
| `cwebp`, `avifenc` | `image_formats` containing `webp` or `avif` | That format is left out of the `<picture>` |
| `sass` (or the path in `sass_binary`) | `scss: true` | Compilation is skipped with a warning |
| `git` | `lastmod_from_git: true` | Pages git cannot date get a warning, printed once |
| Hook programs | `hooks:` | The build stops before rendering |
| `npx` (running `wrangler`) or `git` | `deploy:` / `--deploy=…` for the targets that use them | The deployment fails |

## Files written outside the output directory

- `.ssg-cache/` in the working directory (or the configured cache directory):
  image, audio, AI and external-source caches, and the incremental build
  state.
- The theme directory, when `online_theme` downloads into it.
- `workers/<name>/`, when a worker with a remote `source` is fetched.
