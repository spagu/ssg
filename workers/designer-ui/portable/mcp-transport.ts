// The Streamable HTTP binding of MCP for a stateless, tool-only server:
// one endpoint, one JSON-RPC message per POST, one JSON answer (never an SSE
// stream, never a session). Checked against the specification 2026-07-28
// (current) and 2025-11-25 (the last initialize-based revision) on
// 2026-11-15 — the worker's README lists the rules and links.
//
//   not POST                         → 405, allow: POST
//   Origin present, not allowed      → 403 (DNS rebinding; MUST in both revisions)
//   Content-Type not JSON            → 415;  Accept without JSON → 406
//   larger than maxBytes             → 413, read no further
//   not JSON / not UTF-8             → 400  -32700
//   a batch, a bad envelope or id    → 400  -32600
//   a notification                   → 202, no body
//   modern: version not served       → 400  -32022 {supported, requested}
//           headers ≠ body           → 400  -32020 (MCP-Protocol-Version, Mcp-Method, Mcp-Name)
//           no clientCapabilities    → 400  -32602
//           unknown method           → 404  -32601
//   legacy: unknown MCP-Protocol-Version header → 400 -32022
//   anything unexpected              → 500  -32603, without its text
//
// Error bodies never carry a stack or the request's data beyond a tool name.
// Portable: imports only mcp-methods.ts; designer-ui copies both.

import {
  CODE, LEGACY_VERSIONS, META_CAPABILITIES, META_VERSION, MODERN_VERSIONS, RpcFailure, SUPPORTED_VERSIONS,
  answer, isObject, unsupportedVersion,
} from "./mcp-methods";
import type { Json, McpHost, RpcRequest } from "./mcp-methods";

export type { Call, Json, McpHost, McpTool } from "./mcp-methods";
export { CODE, LEGACY_VERSIONS, MODERN_VERSIONS, SUPPORTED_VERSIONS } from "./mcp-methods";

export const DEFAULT_MAX_BYTES = 1024 * 1024;
// utf8: a decoder that refuses malformed bytes instead of replacing them.
const utf8 = () => new TextDecoder("utf-8", { fatal: true, ignoreBOM: false });

// serveMcp answers one POST to the MCP endpoint. The caller has already
// authenticated the request; host.call checks each operation's permission.
export async function serveMcp<Op>(request: Request, host: McpHost<Op>): Promise<Response> {
  try {
    return await serve(request, host);
  } catch (err) {
    return fail(err instanceof RpcFailure ? err : new RpcFailure(CODE.internal, "internal error", 500));
  }
}

async function serve<Op>(request: Request, host: McpHost<Op>): Promise<Response> {
  const refuse = (status: number, message: string, headers: Record<string, string> = {}) =>
    fail(new RpcFailure(CODE.invalidRequest, message, status), headers);
  if (request.method !== "POST") return refuse(405, "the MCP endpoint accepts POST", { allow: "POST" });
  const origin = request.headers.get("origin");
  if (origin !== null && origin !== new URL(request.url).origin && !host.origins?.includes(origin)) return refuse(403, "origin not allowed");
  if (!/^application\/json\b/i.test(request.headers.get("content-type") ?? "")) return refuse(415, "send application/json");
  if (!acceptsJson(request.headers.get("accept"))) return refuse(406, "the answer is application/json");
  const max = host.maxBytes ?? DEFAULT_MAX_BYTES;
  const raw = await readLimited(request, max);
  if (raw === null) return refuse(413, `the request is larger than ${max} bytes`);
  let msg: unknown;
  try {
    msg = JSON.parse(raw);
  } catch {
    throw new RpcFailure(CODE.parse, "parse error", 400);
  }
  const req = envelope(msg, request.headers);
  if (!req) return new Response(null, { status: 202 }); // a notification: accepted, nothing to say
  const headers = versionHeader(req, request.headers);
  try {
    return reply(200, { jsonrpc: "2.0", id: req.id, result: await answer(host, req) }, headers);
  } catch (err) {
    if (err instanceof RpcFailure) return fail(withId(err, req.id), headers);
    throw err;
  }
}

// envelope checks the JSON-RPC message and the era rules; null for a
// notification (which this server only acknowledges).
function envelope(msg: unknown, headers: Headers): RpcRequest | null {
  if (Array.isArray(msg)) throw new RpcFailure(CODE.invalidRequest, "batches are not supported", 400);
  if (!isObject(msg)) throw new RpcFailure(CODE.invalidRequest, "a JSON-RPC message is an object", 400);
  const hasId = Object.hasOwn(msg, "id");
  const id = msg.id;
  const goodId = typeof id === "string" || Number.isSafeInteger(id);
  if (hasId && !goodId) throw new RpcFailure(CODE.invalidRequest, "id must be a string or an integer", 400);
  const own = goodId ? (id as string | number) : undefined;
  if (msg.jsonrpc !== "2.0" || typeof msg.method !== "string") {
    throw withId(new RpcFailure(CODE.invalidRequest, 'a request needs jsonrpc "2.0" and a method', 400), own);
  }
  const params = msg.params ?? {};
  if (!isObject(params)) throw withId(new RpcFailure(CODE.invalidParams, "params must be an object", 400), own);
  const header = headers.get("mcp-protocol-version");
  if (!hasId) {
    if (header !== null && !SUPPORTED_VERSIONS.includes(header)) throw unsupportedVersion(header);
    return null;
  }
  const meta = isObject(params._meta) ? params._meta : {};
  const declared = meta[META_VERSION];
  const req: RpcRequest = { id: own!, method: msg.method, params, modern: typeof declared === "string" && declared !== "" };
  try {
    if (req.modern) modernRules(req, declared as string, meta, headers);
    else if (header !== null && !LEGACY_VERSIONS.includes(header)) {
      if (MODERN_VERSIONS.includes(header)) throw new RpcFailure(CODE.invalidParams, `_meta must carry ${META_VERSION}`, 400);
      throw unsupportedVersion(header);
    }
  } catch (err) {
    throw withId(err as RpcFailure, req.id);
  }
  return req;
}

// modernRules: the version is one this server speaks, and the headers an
// intermediary may route on say what the body says.
function modernRules(req: RpcRequest, declared: string, meta: Json, headers: Headers): void {
  if (!MODERN_VERSIONS.includes(declared)) throw unsupportedVersion(declared);
  const mismatch = (what: string) => new RpcFailure(CODE.headerMismatch, `header mismatch: ${what}`, 400);
  if (headers.get("mcp-protocol-version") !== declared) throw mismatch("MCP-Protocol-Version must equal the request's protocol version");
  if (headers.get("mcp-method") !== req.method) throw mismatch("Mcp-Method must equal the method");
  if (req.method === "tools/call" && decodeHeader(headers.get("mcp-name")) !== req.params.name) throw mismatch("Mcp-Name must equal params.name");
  if (!isObject(meta[META_CAPABILITIES])) throw new RpcFailure(CODE.invalidParams, `_meta must carry ${META_CAPABILITIES}`, 400);
}

// decodeHeader: a header value, the =?base64?…?= form decoded (null when
// absent or not decodable — which can never equal a name in the body).
function decodeHeader(v: string | null): string | null {
  const m = v === null ? null : /^=\?base64\?(.*)\?=$/.exec(v);
  if (!m) return v;
  try {
    return utf8().decode(Uint8Array.from(atob(m[1]), (c) => c.charCodeAt(0)));
  } catch {
    return null;
  }
}

// acceptsJson: no Accept header, or one that admits application/json.
function acceptsJson(accept: string | null): boolean {
  if (accept === null) return true;
  return accept.split(",").some((t) => ["application/json", "application/*", "*/*"].includes(t.split(";")[0].trim().toLowerCase()));
}

// readLimited: the body as UTF-8 text, or null as soon as it passes max
// bytes (the declared length is checked first, the stream as it arrives).
// Exported for the workers' REST bodies too (SR-14): a body sent without
// Content-Length (chunked) is never held whole before the limit is known.
export async function readLimited(request: Request, max: number): Promise<string | null> {
  if (Number(request.headers.get("content-length")) > max) return null;
  if (!request.body) return "";
  const parts: Uint8Array[] = [];
  let size = 0;
  const reader = request.body.getReader();
  for (let r = await reader.read(); !r.done; r = await reader.read()) {
    size += r.value.byteLength;
    if (size > max) {
      await reader.cancel();
      return null;
    }
    parts.push(r.value);
  }
  const all = new Uint8Array(size);
  let at = 0;
  for (const p of parts) {
    all.set(p, at);
    at += p.byteLength;
  }
  try {
    return utf8().decode(all);
  } catch {
    return ""; // not UTF-8: JSON.parse refuses it as a parse error
  }
}

const versionHeader = (req: RpcRequest, headers: Headers): Record<string, string> => {
  const v = req.modern ? (req.params._meta as Json)[META_VERSION] as string : headers.get("mcp-protocol-version");
  return v ? { "mcp-protocol-version": v } : {};
};

function withId(err: RpcFailure, id: string | number | undefined): RpcFailure {
  err.id = id;
  return err;
}

function fail(err: RpcFailure, headers: Record<string, string> = {}): Response {
  const error = { code: err.code, message: err.message, ...(err.data === undefined ? {} : { data: err.data }) };
  return reply(err.status, { jsonrpc: "2.0", ...(err.id === undefined ? {} : { id: err.id }), error }, headers);
}

const reply = (status: number, body: Json, headers: Record<string, string> = {}): Response =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json", ...headers } });
