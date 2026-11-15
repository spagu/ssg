// Every route of the worker, one line each. Paths are relative to the base
// path; the permission is in each operation, not here (ops.ts). Add a path to
// openapi.yaml with every route (test/openapi.test.ts keeps them equal).

import { changePublishOp } from "./change-publish";
import { readBody } from "./change-input";
import { contentListOp } from "./content-list";
import { contentNewPathOp } from "./content-new";
import { contentReadOp } from "./content-read";
import { healthOp } from "./health";
import { json } from "./http";
import { mediaListOp, mediaNewPathOp } from "./media-list";
import { mediaReferencesOp } from "./media-references";
import { mcpOp } from "./mcp";
import { meOp } from "./me";
import { moduleOp } from "./module";
import { revisionDiffOp } from "./revision-diff";
import { revisionReadOp } from "./revision-read";
import { revisionsListOp } from "./revisions";
import { siteInfoOp } from "./site-info";
import type { Route } from "./router";
import { summaryOp } from "./summary";
import { widgetsListOp } from "./widgets";

export const ROUTES: Route[] = [
  { method: "GET", path: "/api/module", op: moduleOp },
  { method: "GET", path: "/api/module/summary", op: summaryOp },
  { method: "GET", path: "/api/health", op: healthOp },
  { method: "GET", path: "/api/me", op: meOp },
  // What the panel needs before listing or creating (GO-173): sections, languages, media.
  { method: "GET", path: "/api/site", op: siteInfoOp },
  { method: "GET", path: "/api/content", op: contentListOp, input: ({ url }) => query(url, "q", "section", "cursor", "limit") },
  // Before /api/content/:path: "new-path" is no Markdown file, so the two never meet.
  { method: "GET", path: "/api/content/new-path", op: contentNewPathOp, input: ({ url }) => query(url, "section", "title") },
  { method: "GET", path: "/api/content/:path", op: contentReadOp, input: (_ctx, params) => ({ path: params.path }) },
  { method: "POST", path: "/api/changes", op: changePublishOp, input: (ctx) => readBody(ctx), respond: (out) => json(out, 201) },
  { method: "GET", path: "/api/revisions/:path", op: revisionsListOp, input: ({ url }, params) => ({ path: params.path, ...query(url, "cursor") }) },
  // Before /api/revisions/:path/:commit: "diff" is no commit id, so the two never meet.
  { method: "GET", path: "/api/revisions/:path/diff", op: revisionDiffOp, input: ({ url }, params) => ({ path: params.path, ...query(url, "from", "to", "cursor") }) },
  { method: "GET", path: "/api/revisions/:path/:commit", op: revisionReadOp, input: ({ url }, params) => ({ path: params.path, commit: params.commit, ...query(url, "cursor") }) },
  { method: "GET", path: "/api/media", op: mediaListOp, input: ({ url }) => query(url, "cursor", "limit") },
  { method: "GET", path: "/api/media/new-path", op: mediaNewPathOp, input: ({ url }) => query(url, "name", "type", "hash") },
  // What can be inserted (GO-174): components, shortcodes, built-in blocks.
  { method: "GET", path: "/api/widgets", op: widgetsListOp },
  { method: "GET", path: "/api/media/references", op: mediaReferencesOp, input: ({ url }) => query(url, "path", "cursor") },
  // The MCP endpoint (GO-176): JSON-RPC in, JSON-RPC out — mcpOp answers with its own Response.
  { method: "POST", path: "/mcp", op: mcpOp, respond: (res: Response) => res },
];

// query: the named search parameters (absent → null).
function query<K extends string>(url: URL, ...names: K[]): Record<K, string | null> {
  return Object.fromEntries(names.map((n) => [n, url.searchParams.get(n)])) as Record<K, string | null>;
}
