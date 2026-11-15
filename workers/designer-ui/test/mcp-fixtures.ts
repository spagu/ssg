// MCP calls through the whole pipeline (fixtures.ts), beside the REST calls
// they must equal: a service token as Bearer by default, as a script or an
// assistant sends it.
import { BASE, makeEnv, request, run } from "./fixtures.ts";
import type { FakeGitHub } from "./fake-github.ts";

export const ALL = ["designer-ui:read", "designer-ui:settings", "designer-ui:advanced", "designer-ui:theme"];
export const MODERN = "2026-07-28";
type Kind = "user" | "service";
export type ToolResult = { isError: boolean; structuredContent: Record<string, any>; content: { type: string; text: string }[] };

// rpc posts one JSON-RPC message (an object, or raw text) to {base}/mcp.
export async function rpc(gh: FakeGitHub | undefined, perms: string[] | null, msg: unknown, headers: Record<string, string> = {}, kind: Kind = "service", env = makeEnv()): Promise<Response> {
  const r = await request(BASE + "/mcp", perms, {
    method: "POST", kind, body: typeof msg === "string" ? msg : JSON.stringify(msg),
    headers: { "content-type": "application/json", accept: "application/json, text/event-stream", ...headers },
  });
  return run(r, env, gh);
}

// modern builds a 2026-07-28 request and the headers that must mirror it.
export function modern(method: string, params: Record<string, unknown> = {}, id: number | string = 1) {
  const msg = { jsonrpc: "2.0", id, method, params: { ...params, _meta: { "io.modelcontextprotocol/protocolVersion": MODERN, "io.modelcontextprotocol/clientCapabilities": {} } } };
  const headers: Record<string, string> = { "mcp-protocol-version": MODERN, "mcp-method": method };
  if (typeof params.name === "string") headers["mcp-name"] = params.name;
  return { msg, headers };
}

// tool calls a tool (initialize-era shape) and returns its result.
export async function tool(gh: FakeGitHub, perms: string[], name: string, args: Record<string, unknown>): Promise<ToolResult> {
  const res = await rpc(gh, perms, { jsonrpc: "2.0", id: 7, method: "tools/call", params: { name, arguments: args } });
  const body = await res.json();
  if (!body.result) throw new Error(`no result: ${JSON.stringify(body)}`);
  return body.result as ToolResult;
}

// rest: the REST call (a service token too), its status and body.
export async function rest(gh: FakeGitHub, perms: string[], path: string, body?: unknown) {
  const init = body === undefined ? {} : { method: "POST", body: JSON.stringify(body), headers: { "content-type": "application/json" } };
  const res = await run(await request(BASE + path, perms, { kind: "service", ...init }), makeEnv(), gh);
  return { status: res.status, body: await res.json() };
}

// refusalOf: a REST refusal as the MCP tool reports it — the body and the status.
export const refusalOf = (r: { status: number; body: Record<string, unknown> }) => ({ ...r.body, status: r.status });

// tree: every path → blob id on main, to compare two repositories' effects.
export const tree = (gh: FakeGitHub) => Object.fromEntries([...gh.store.flatAt(gh.store.refs.get("main")!)].map(([p, e]) => [p, e.sha]));

// published drops what differs between two equal publications (the commit id carries the time).
export const published = (out: Record<string, unknown>) => ({ ...out, commitId: undefined });
