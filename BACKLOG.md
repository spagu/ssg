# Backlog

One row is one session: a ready-to-use prompt, its size (S/M/L) and the model.
State lives in issues and CHANGELOG.md, not here. A session without a specific
task takes the first row; once it is done, commit and delete the row.

| Prompt | Size | Model |
|---|---|---|
| `analytics_ids` from the config are ignored when `analytics: true` and no metadata.json is read (content_sources-only or mddb build): apply them in `loadContent` like the site identity (#440) | S | Sonnet |
| Home page `og:description` and JSON-LD `description` never carry the site description (`indexPageContext`); decide with the owner whether to add it (changes output of existing sites) | S | Sonnet |
| Config schema help text is wrong: `pretty_html` says "indent" (it only removes blank lines), `title`/`description` say "feeds" (feeds do not use them) — `internal/config/schema_docs_*.go`, the MCP pin test, designer-ui `config-schema.json` | S | Sonnet |
| `action.yml`: a `domain` input that maps to `--domain` (#439) | S | Sonnet |
| Watch mode: the positional `<domain>` is lost on config reload when the file has no `domain:` (`reloadWatchConfig` skips `validateRequiredFields`) | S | Sonnet |
