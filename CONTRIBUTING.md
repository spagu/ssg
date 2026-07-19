# Contributing to SSG

Thank you for helping improve SSG. This guide covers the local development and
review workflow. By participating, you agree to follow the
[Code of Conduct](CODE_OF_CONDUCT.md).

## Before starting

- Search existing issues and pull requests before duplicating work.
- Keep changes focused; separate unrelated refactors from fixes or features.
- For a behaviour change, update tests and the canonical documentation in the
  same pull request.
- Never commit credentials, private content, generated archives or local build
  output.
- Security vulnerabilities must be reported through [SECURITY.md](SECURITY.md),
  not a public issue.

## Requirements

- Go 1.26.5 or newer
- Make for the documented shortcuts
- Git
- Optional `cwebp` for WebP integration paths
- Optional Dart Sass for SCSS integration paths
- Optional `golangci-lint`, `gosec` and `govulncheck` for extended checks

Go 1.26.5 is the minimum because earlier 1.26 standard libraries contain
security issues relevant to this project. The exact module requirement is in
[go.mod](go.mod).

## Set up the repository

```bash
git clone https://github.com/spagu/ssg.git
cd ssg
go mod download
make build
```

The binary is written to `build/ssg`.

## Repository map

```text
cmd/ssg/            CLI, flag parsing, server, archives and orchestration
internal/config/    YAML/TOML/JSON configuration
internal/parser/    Markdown and frontmatter parser
internal/models/    Content and URL models
internal/generator/ Site generation, templates and output transforms
internal/engine/    Go, Pongo2, Mustache and Handlebars adapters
internal/images/    Build-time image processing and cache
internal/webp/      Directory WebP conversion
internal/theme/     Theme download and conversion
internal/mddb/      HTTP/gRPC remote content client
internal/deploy/    Native deployment providers
templates/          Built-in themes
content/            Test/example content
docs/               User-facing reference documentation
examples/           Workflow examples
packaging/          OS/package-manager definitions
```

## Development commands

| Command | Purpose |
|---|---|
| `make help` | List Make targets |
| `make deps` | Download modules |
| `make tidy` | Run `go mod tidy` |
| `make build` | Build the local binary |
| `make test` | Run race-enabled tests with coverage data |
| `make test-coverage` | Generate `coverage.html` |
| `make lint` | Run golangci-lint or fall back to `go vet` |
| `make security` | Run gosec and govulncheck when installed |
| `make all` | Dependencies, lint, tests and build |
| `make version-check` | Check packaging version consistency |
| `make test-action` | Exercise the local Action build path |
| `make clean` | Remove generated build/test artifacts |

`make test` runs:

```bash
go test -v -race -coverprofile=coverage.out ./...
```

For a quick targeted iteration, run the affected package directly:

```bash
go test ./internal/parser
go test ./internal/generator -run TestName
```

Run the full suite before handing off a code change.

## Try a local site

```bash
make build
./build/ssg test-content simple example.com \
  --clean --output-dir=/tmp/ssg-preview
```

For an interactive preview:

```bash
./build/ssg test-content simple example.com --http --watch
```

Do not treat generated `output/` files as source. Fix the generator, theme or
content fixture instead.

## Change guidelines

### Behaviour and tests

- Add a regression test that fails without the fix.
- Prefer package-level unit tests for edge cases and generator integration tests
  for end-to-end behaviour.
- Cover invalid input and failure paths, not only success.
- Keep filesystem tests isolated with `t.TempDir()`.
- Avoid tests that require the network or real deployment credentials.
- Skip optional-tool integration paths clearly when the tool is unavailable.

### Security

- Treat content, frontmatter, downloaded themes, MDDB documents, paths and CLI
  values as untrusted at their boundary.
- Keep generated paths confined to the selected output directory.
- Pass process arguments as an argv slice; do not construct shell commands from
  user-controlled strings.
- Never weaken SSH host verification or place tokens in URLs/log output.
- Bound remote response bodies and use timeouts.
- Preserve the project's `#nosec` rationale when a flagged operation is
  intentional; do not add blanket suppressions.

### Documentation

Documentation is part of the public interface:

- [README.md](README.md) is the concise entry point.
- [docs/CONTENT.md](docs/CONTENT.md) owns content semantics.
- [docs/CONFIGURATION.md](docs/CONFIGURATION.md) owns configuration behaviour.
- [docs/TEMPLATES.md](docs/TEMPLATES.md) owns theme contexts and engines.
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) owns providers and CI.
- [.ssg.yaml.example](.ssg.yaml.example) is the exhaustive copyable YAML file.
- [CHANGELOG.md](CHANGELOG.md) owns release history.

When adding or renaming configuration, update `Config`, defaults, parsing,
`.ssg.yaml.example`, CLI help, tests and the appropriate guide together.
Examples must be safe to copy and must state optional external dependencies.

### Style

- Format Go changes with `gofmt`.
- Keep functions focused and errors descriptive.
- Follow existing package boundaries and naming.
- Add comments for exported identifiers and non-obvious security decisions.
- Avoid drive-by formatting or generated-file churn in focused changes.

## Versioned changes

The single source of the release version is `VERSION`. Packaging manifests are
kept in sync by:

```bash
make version-sync
make version-check
```

Do not create tags or publish packages as part of an ordinary contribution.
Maintainers handle releases. User-visible changes should be recorded in
`CHANGELOG.md` under the appropriate release section.

### Republishing the Homebrew tap (maintainers)

Tagging runs `.github/workflows/homebrew.yml`, which regenerates
`spagu/homebrew-tap/ssg.rb` from the release's `checksums.sha256`. If that
fails, fix it and re-run **that workflow alone**:

> Actions → **Homebrew Tap** → *Run workflow* → version, e.g. `1.8.7`

It is idempotent and needs no new tag. **Do not re-run the release workflow to
fix the tap**: it rebuilds the binaries, so the release assets get new SHA-256
sums and anyone who already downloaded or pinned the old ones is broken.

The push uses the `HOMEBREW_TAP_TOKEN` secret with
`AUTHORIZATION: basic base64(x-access-token:<PAT>)`. GitHub's git-over-HTTPS
endpoint rejects `bearer` with a PAT — and because a malformed header also
blocks otherwise-anonymous clones of the public tap, that failure looks
exactly like an expired token.

## Pull request checklist

- [ ] The change is focused and its behaviour is explained.
- [ ] Tests cover the change and failure cases.
- [ ] `make test` passes.
- [ ] `make lint` passes.
- [ ] Relevant documentation and examples are updated.
- [ ] Configuration/default/help text remain consistent.
- [ ] `make version-check` passes when packaging/version files changed.
- [ ] No secrets or unrelated generated files are included.

Existing contributors are listed in [CONTRIBUTORS.md](CONTRIBUTORS.md).

