// Every route of the worker, one line each. Paths are relative to the base
// path; the permission is in each operation, not here (ops.ts). Add a path to
// openapi.yaml with every route (test/module.test.ts keeps them equal).

import { readLimited } from "../portable/mcp-transport";
import { healthOp } from "./health";
import { mcpOp } from "../portable/mcp";
import { meOp } from "../portable/me";
import { moduleOp } from "../portable/module";
import { OpError } from "../portable/ops";
import { activePlan } from "../portable/plan";
import type { OpContext } from "../portable/ops";
import { previewOp, previewResponse } from "./preview";
import { publishOp } from "./publish";
import { revisionReadOp, revisionsOp } from "./revisions";
import type { Route } from "../portable/router";
import { settingsPublishOp } from "./settings-publish";
import { settingsReadOp } from "./settings-read";
import { summaryOp } from "./summary";
import { themeListOp, themeReadOp } from "./theme-ops";
import { themePublishOp } from "./theme-publish";

// body: the request's JSON object (400 for anything else; 413 past the
// plan's body limit, portable/plan.ts BODY_LIMIT, the MCP endpoint's too). The size is checked first: a declared length,
// then the bytes as they arrive (SR-14) — a chunked body is never held whole
// before its size is known.
async function body(ctx: OpContext): Promise<Record<string, unknown>> {
  const maxBody = activePlan(ctx.env).maxBody;
  const raw = await readLimited(ctx.request, maxBody);
  if (raw === null) throw new OpError("too_large", 413, `the request body is larger than ${maxBody} bytes`);
  let v: unknown;
  try {
    v = JSON.parse(raw);
  } catch {
    throw new OpError("invalid_json", 400, "the body must be a JSON object");
  }
  if (typeof v !== "object" || v === null || Array.isArray(v)) throw new OpError("invalid_json", 400, "the body must be a JSON object");
  return v as Record<string, unknown>;
}

const query = (ctx: OpContext, name: string): string | null => ctx.url.searchParams.get(name);

export const ROUTES: Route[] = [
  { method: "GET", path: "/api/module", op: moduleOp },
  { method: "GET", path: "/api/module/summary", op: summaryOp },
  { method: "GET", path: "/api/health", op: healthOp },
  { method: "GET", path: "/api/me", op: meOp },
  { method: "GET", path: "/api/settings", op: settingsReadOp },
  { method: "POST", path: "/api/settings/publish", op: settingsPublishOp, input: body },
  { method: "GET", path: "/api/theme", op: themeListOp },
  { method: "GET", path: "/api/theme/files/:path", op: themeReadOp, input: (_ctx, p) => ({ path: p.path }) },
  { method: "POST", path: "/api/theme/publish", op: themePublishOp, input: body },
  { method: "POST", path: "/api/publish", op: publishOp, input: body },
  { method: "GET", path: "/api/revisions", op: revisionsOp, input: (ctx) => ({ path: query(ctx, "path"), cursor: query(ctx, "cursor") }) },
  { method: "GET", path: "/api/revisions/:commitId", op: revisionReadOp, input: (ctx, p) => ({ commitId: p.commitId, path: query(ctx, "path") }) },
  // The look preview (GO-188): a page, not JSON; its headers are preview-entry.ts.
  { method: "GET", path: "/api/preview", op: previewOp, input: (ctx) => ctx.url.searchParams, respond: previewResponse },
  // The MCP endpoint (GO-181): JSON-RPC in, JSON-RPC out — mcpOp answers with its own Response.
  { method: "POST", path: "/mcp", op: mcpOp, respond: (res: Response) => res },
];
