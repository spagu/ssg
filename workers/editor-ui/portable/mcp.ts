// POST {base}/mcp — the MCP endpoint: the same operations as the REST routes,
// for an AI assistant or a script (README of the round, decision 11: one
// implementation, two ways in). It sits behind the whole pipeline (app.ts):
// the rate limit, the `auth` gate (a service token as Bearer, a person's
// cookie — the verifier decides, Bearer first), CSRF for a cookie session.
//
// The endpoint itself needs the module's read permission — nothing on it is
// usable without reading. Each tool is a thin wrapper of an operation (the
// template's src/mcp-tools.ts) run through perform(), so a tool call is
// refused exactly as its REST route would refuse it; tools/list shows only
// the tools whose permissions the caller holds. Shared by the panel workers
// (GO-185); the transport is mcp-transport.ts.

import { serveMcp } from "./mcp-transport";
import type { Json, McpHost, McpTool } from "./mcp-transport";
import { RepoError } from "./repo-provider";
import manifest from "../module.json" with { type: "json" };
import { activePlan } from "./plan";
import { MODULE_ID } from "../src/http";
import { INSTRUCTIONS, TOOLS, permissionsOf } from "../src/mcp-tools";
import { OpError, errorResponse, perform } from "./ops";
import type { Op, OpContext } from "./ops";
import { PERM } from "../src/permissions";
import { can } from "../src/session-verifier";

export const SERVER = { name: `ssg-${MODULE_ID}`, version: manifest.version };

// hostFor: the transport's view of this request's caller.
export function hostFor(ctx: OpContext, tools: readonly McpTool<Op>[] = TOOLS): McpHost<Op> {
  return {
    server: SERVER,
    instructions: INSTRUCTIONS,
    tools,
    visible: (tool) => permissionsOf(tool).every((p) => can(ctx.caller.perms, p)),
    call: (op, input) => perform(op, ctx, input),
    refusal,
    maxBytes: activePlan(ctx.env).maxBody,
  };
}

// refusal: an operation's refusal is the REST route's error body, its HTTP
// status beside it — the same answer by either way in. Anything else is
// logged by errorResponse and stays out of the answer.
export async function refusal(err: unknown): Promise<Json | null> {
  const res = errorResponse(err, "POST /mcp");
  if (!(err instanceof OpError) && !(err instanceof RepoError)) return null;
  return { ...((await res.json()) as Json), status: res.status };
}

export const mcpOp: Op<unknown, Response> = {
  name: "mcp",
  permission: PERM.read,
  run: (ctx) => serveMcp(ctx.request, hostFor(ctx)),
};
