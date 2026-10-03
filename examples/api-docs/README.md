# API docs from code

A small JavaScript library (`lib/`) documented by its own comments, and one
hand-written guide that links into the generated reference.

```bash
ssg --config examples/api-docs/ssg.yaml        # from the repository root
open examples/api-docs/output/api/textkit/index.html
```

The build publishes `/api/textkit/` (the package and its README), one page per
module, one per class, and `api.json`. `check_links: strict` and
`check_api: warn` run on every build. See [docs/API_DOCS.md](../../docs/API_DOCS.md).
