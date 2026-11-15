// portable/mcp-transport.ts + mcp-methods.ts on their own, with a fake host:
// every HTTP and JSON-RPC rule of the stateless binding, and that no error
// body carries a stack or the text of an unexpected error.
import { test } from "node:test";
import assert from "node:assert/strict";
import { CODE, DEFAULT_MAX_BYTES, LEGACY_VERSIONS, SUPPORTED_VERSIONS, serveMcp } from "../portable/mcp-transport.ts";
import type { McpHost } from "../portable/mcp-transport.ts";

const URL_ = "https://example.com/editor/mcp";
const MODERN = "2026-07-28";
const META = { "io.modelcontextprotocol/protocolVersion": MODERN, "io.modelcontextprotocol/clientCapabilities": {} };

const host = (over: Partial<McpHost<string>> = {}): McpHost<string> => ({
  server: { name: "fake", version: "1.0.0" },
  instructions: "be kind",
  tools: [
    { name: "echo", description: "echo", inputSchema: { type: "object" }, op: "echo", input: (a) => ({ got: a }), annotations: { readOnlyHint: true } },
    { name: "bare", description: "bare", inputSchema: { type: "object" }, op: "scalar" },
    { name: "hidden", description: "hidden", inputSchema: { type: "object" }, op: "refused" },
    { name: "boom", description: "boom", inputSchema: { type: "object" }, op: "boom" },
  ],
  visible: (t) => t.name !== "hidden",
  call: async (op, input) => {
    if (op === "refused") throw new Error("refused");
    if (op === "boom") throw new Error("secret detail at /src/x.ts:1");
    return op === "scalar" ? 42 : input;
  },
  refusal: async (err) => ((err as Error).message === "refused" ? { error: "forbidden", status: 403 } : null),
  maxBytes: 1000,
  ...over,
});

type Init = { method?: string; headers?: Record<string, string>; body?: BodyInit | null };
const send = (msg: unknown, init: Init = {}, h = host()) =>
  serveMcp(new Request(URL_, {
    method: init.method ?? "POST",
    headers: { "content-type": "application/json", ...init.headers },
    body: init.body !== undefined ? init.body : typeof msg === "string" ? msg : JSON.stringify(msg),
  }), h);

async function rpcError(res: Response, status: number, code: number) {
  assert.equal(res.status, status);
  assert.equal(res.headers.get("content-type"), "application/json");
  const text = await res.text();
  assert.doesNotMatch(text, /\bat \/|\.ts:\d|secret/, "no stack, no error text");
  const body = JSON.parse(text);
  assert.equal(body.jsonrpc, "2.0");
  assert.equal(body.error.code, code, text);
  return body;
}

const req = (method: string, params?: unknown, id: unknown = 1) => ({ jsonrpc: "2.0", id, method, ...(params === undefined ? {} : { params }) });
const modernHeaders = (method: string, name?: string) => ({ "mcp-protocol-version": MODERN, "mcp-method": method, ...(name ? { "mcp-name": name } : {}) });

test("HTTP: POST only, Origin, Content-Type, Accept, size - each with a JSON-RPC body without id", async () => {
  const get = await serveMcp(new Request(URL_), host());
  assert.equal(get.headers.get("allow"), "POST");
  assert.equal("id" in (await rpcError(get, 405, CODE.invalidRequest)), false);
  await rpcError(await send(req("ping"), { headers: { origin: "https://evil.example.com" } }), 403, CODE.invalidRequest);
  assert.equal((await send(req("ping"), { headers: { origin: "https://example.com" } })).status, 200, "the endpoint's own origin");
  assert.equal((await send(req("ping"), { headers: { origin: "https://app.example.com" } }, host({ origins: ["https://app.example.com"] }))).status, 200);
  await rpcError(await send(req("ping"), { headers: { "content-type": "text/plain" } }), 415, CODE.invalidRequest);
  await rpcError(await serveMcp(new Request(URL_, { method: "POST", body: new TextEncoder().encode("{}") }), host()), 415, CODE.invalidRequest);
  await rpcError(await send(req("ping"), { headers: { accept: "text/html" } }), 406, CODE.invalidRequest);
  for (const accept of ["application/json, text/event-stream", "*/*", "application/*;q=0.9", "text/event-stream, application/json; q=1"]) {
    assert.equal((await send(req("ping"), { headers: { accept } })).status, 200, accept);
  }
  await rpcError(await send(req("ping"), { headers: { "content-length": "1001" } }), 413, CODE.invalidRequest);
  await rpcError(await send(JSON.stringify(req("ping", { pad: "x".repeat(1100) }))), 413, CODE.invalidRequest);
  const chunks = new ReadableStream({ start(c) { c.enqueue(new TextEncoder().encode(JSON.stringify(req("ping")))); c.close(); } });
  assert.equal((await serveMcp(new Request(URL_, { method: "POST", headers: { "content-type": "application/json" }, body: chunks, duplex: "half" } as RequestInit), host())).status, 200, "a streamed body");
  assert.equal(DEFAULT_MAX_BYTES, 1024 * 1024);
  assert.equal((await send(req("ping"), {}, host({ maxBytes: undefined }))).status, 200);
});

test("JSON-RPC envelope: parse errors, batches, ids, jsonrpc, method, params", async () => {
  await rpcError(await send("{not json"), 400, CODE.parse);
  await rpcError(await send(null, { body: null }), 400, CODE.parse);
  await rpcError(await send(null, { body: new Uint8Array([0x7b, 0xff, 0x7d]) }), 400, CODE.parse);
  await rpcError(await send([req("ping")]), 400, CODE.invalidRequest);
  await rpcError(await send(7), 400, CODE.invalidRequest);
  for (const id of [null, 1.5, true, {}]) assert.equal("id" in (await rpcError(await send(req("ping", undefined, id)), 400, CODE.invalidRequest)), false);
  assert.equal((await rpcError(await send({ jsonrpc: "1.0", id: 3, method: "ping" }), 400, CODE.invalidRequest)).id, 3);
  assert.equal((await rpcError(await send({ jsonrpc: "2.0", id: "a", method: 5 }), 400, CODE.invalidRequest)).id, "a");
  assert.equal((await rpcError(await send(req("ping", [1])), 400, CODE.invalidParams)).id, 1);
  const ok = await (await send(req("ping", undefined, "str-id"))).json();
  assert.deepEqual(ok, { jsonrpc: "2.0", id: "str-id", result: {} });
});

test("notifications: 202 with no body; an unknown protocol version header is still refused", async () => {
  const n = await send({ jsonrpc: "2.0", method: "notifications/initialized" });
  assert.equal(n.status, 202);
  assert.equal(await n.text(), "");
  assert.equal((await send({ jsonrpc: "2.0", method: "x" }, { headers: { "mcp-protocol-version": LEGACY_VERSIONS[0] } })).status, 202);
  const bad = await rpcError(await send({ jsonrpc: "2.0", method: "x" }, { headers: { "mcp-protocol-version": "1999-01-01" } }), 400, CODE.unsupportedVersion);
  assert.deepEqual(bad.error.data, { supported: SUPPORTED_VERSIONS, requested: "1999-01-01" });
});

test("the initialize era: negotiation, ping, tools/list, server/discover, unknown method 200", async () => {
  const init = async (protocolVersion?: unknown) => (await (await send(req("initialize", { protocolVersion, capabilities: {} }))).json()).result;
  assert.deepEqual(await init("2025-06-18"), {
    protocolVersion: "2025-06-18", capabilities: { tools: { listChanged: false } }, serverInfo: { name: "fake", version: "1.0.0" }, instructions: "be kind",
  });
  assert.equal((await init("2024-11-05")).protocolVersion, "2025-11-25", "unsupported -> the newest of the era");
  assert.equal((await init(5)).protocolVersion, "2025-11-25");
  assert.equal((await (await send(req("initialize"))).json()).result.protocolVersion, "2025-11-25");
  const list = await (await send(req("tools/list"), { headers: { "mcp-protocol-version": "2025-06-18" } })).json();
  assert.deepEqual(list.result.tools.map((t: { name: string }) => t.name), ["echo", "bare", "boom"], "hidden is not listed");
  assert.deepEqual(list.result.tools[0].annotations, { readOnlyHint: true });
  assert.equal("annotations" in list.result.tools[1], false);
  assert.equal("ttlMs" in list.result, false);
  await rpcError(await send(req("tools/list", { cursor: "x" })), 200, CODE.invalidParams);
  const discover = (await (await send(req("server/discover"))).json()).result;
  assert.deepEqual(discover.supportedVersions, SUPPORTED_VERSIONS);
  assert.equal(discover.cacheScope, "private");
  await rpcError(await send(req("resources/list")), 200, CODE.methodNotFound);
  const headerOut = await send(req("ping"), { headers: { "mcp-protocol-version": "2025-03-26" } });
  assert.equal(headerOut.headers.get("mcp-protocol-version"), "2025-03-26");
  assert.equal((await send(req("ping"))).headers.get("mcp-protocol-version"), null);
  await rpcError(await send(req("ping"), { headers: { "mcp-protocol-version": "1999-01-01" } }), 400, CODE.unsupportedVersion);
  await rpcError(await send(req("ping"), { headers: { "mcp-protocol-version": MODERN } }), 400, CODE.invalidParams);
});

test("tools/call: results, tool errors, protocol errors, an unexpected error without its text", async () => {
  const call = (params: unknown) => send(req("tools/call", params));
  const echo = (await (await call({ name: "echo", arguments: { a: 1 } })).json()).result;
  assert.deepEqual(echo, { content: [{ type: "text", text: '{"got":{"a":1}}' }], structuredContent: { got: { a: 1 } }, isError: false });
  assert.deepEqual((await (await call({ name: "echo" })).json()).result.structuredContent, { got: {} }, "no arguments = {}");
  assert.deepEqual((await (await call({ name: "bare" })).json()).result.structuredContent, { result: 42 }, "a scalar is wrapped");
  const hidden = (await (await call({ name: "hidden" })).json()).result;
  assert.deepEqual(hidden, { content: [{ type: "text", text: '{"error":"forbidden","status":403}' }], structuredContent: { error: "forbidden", status: 403 }, isError: true });
  await rpcError(await call({ name: "boom" }), 200, CODE.internal);
  await rpcError(await call({}), 200, CODE.invalidParams);
  const unknown = await rpcError(await call({ name: "n".repeat(500) }), 200, CODE.invalidParams);
  assert.equal(unknown.error.message, "unknown tool: " + "n".repeat(128));
  await rpcError(await call({ name: "echo", arguments: [1] }), 200, CODE.invalidParams);
  const broken = host({ refusal: async () => { throw new Error("secret"); } });
  assert.equal("id" in (await rpcError(await send(req("tools/call", { name: "boom" }), {}, broken), 500, CODE.internal)), false);
});

test("the 2026-07-28 shape: mirrored headers, capabilities, versions, resultType and serverInfo, 404 for an unknown method", async () => {
  const m = (method: string, params: Record<string, unknown> = {}, meta: Record<string, unknown> = META) => req(method, { ...params, _meta: meta });
  const list = await send(m("tools/list"), { headers: modernHeaders("tools/list") });
  assert.equal(list.headers.get("mcp-protocol-version"), MODERN);
  const out = (await list.json()).result;
  assert.equal(out.resultType, "complete");
  assert.deepEqual(out._meta, { "io.modelcontextprotocol/serverInfo": { name: "fake", version: "1.0.0" } });
  assert.deepEqual([out.ttlMs, out.cacheScope], [60000, "private"]);
  const call = (await (await send(m("tools/call", { name: "echo", arguments: {} }), { headers: modernHeaders("tools/call", "echo") })).json()).result;
  assert.equal(call.resultType, "complete");
  assert.equal(call.isError, false);
  const b64 = "=?base64?" + Buffer.from("echo").toString("base64") + "?=";
  assert.equal((await send(m("tools/call", { name: "echo" }), { headers: modernHeaders("tools/call", b64) })).status, 200, "Mcp-Name in the base64 form");
  await rpcError(await send(m("tools/call", { name: "echo" }), { headers: modernHeaders("tools/call", "=?base64?%%%?=") }), 400, CODE.headerMismatch);
  await rpcError(await send(m("tools/call", { name: "echo" }), { headers: modernHeaders("tools/call", "bare") }), 400, CODE.headerMismatch);
  await rpcError(await send(m("tools/call", { name: "echo" }), { headers: modernHeaders("tools/call") }), 400, CODE.headerMismatch);
  await rpcError(await send(m("ping"), { headers: modernHeaders("tools/list") }), 400, CODE.headerMismatch);
  await rpcError(await send(m("ping"), { headers: { ...modernHeaders("ping"), "mcp-protocol-version": "2025-11-25" } }), 400, CODE.headerMismatch);
  await rpcError(await send(m("ping")), 400, CODE.headerMismatch);
  const v = await rpcError(await send(m("ping", {}, { ...META, "io.modelcontextprotocol/protocolVersion": "2030-01-01" }), { headers: modernHeaders("ping") }), 400, CODE.unsupportedVersion);
  assert.equal(v.id, 1);
  assert.equal(v.error.data.requested, "2030-01-01");
  await rpcError(await send(m("ping", {}, { "io.modelcontextprotocol/protocolVersion": MODERN }), { headers: modernHeaders("ping") }), 400, CODE.invalidParams);
  await rpcError(await send(m("resources/list"), { headers: modernHeaders("resources/list") }), 404, CODE.methodNotFound);
  const discover = (await (await send(m("server/discover"), { headers: modernHeaders("server/discover") })).json()).result;
  assert.equal(discover.resultType, "complete");
  assert.deepEqual(discover.capabilities, { tools: { listChanged: false } });
  const hedge = (await (await send(m("initialize", { protocolVersion: "2025-11-25" }), { headers: modernHeaders("initialize") })).json()).result;
  assert.equal(hedge.protocolVersion, "2025-11-25", "an initialize with modern _meta is answered too");
});
