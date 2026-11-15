// POST {base}/mcp through the whole pipeline: who may use it (Bearer service
// tokens, cookie sessions with CSRF, Bearer first), what tools/list shows per
// permission, that a hidden tool still refuses by name, the endpoint's
// limits, the argument mapping of the tools, and per-key refusals.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ADVANCED, BASE, COOKIE, ORIGIN, READ, SETTINGS, THEME, makeEnv, request, run, site, token } from "./fixtures.ts";
import { ALL, modern, rpc, tool } from "./mcp-fixtures.ts";
import { OpError } from "../portable/ops.ts";
import { refusedOf } from "../src/settings-publish.ts";
import { INSTRUCTIONS, TOOLS } from "../src/mcp-tools.ts";
import { MAX_BODY } from "../portable/http-kit.ts";
import { SERVER, refusal } from "../portable/mcp.ts";
import manifest from "../module.json" with { type: "json" };

const READS = ["designer_list", "designer_read", "designer_config_read", "revisions_list", "revision_read"];
const names = async (perms: string[]) => (await (await rpc(await site(), perms, { jsonrpc: "2.0", id: 1, method: "tools/list" })).json()).result.tools.map((t: { name: string }) => t.name);

test("tools/list shows only the tools whose permissions the caller holds, in a fixed order", async () => {
  assert.deepEqual(await names(READ), [...READS, "help"]);
  assert.deepEqual(await names(SETTINGS), [...READS, "designer_config_set", "help"]);
  assert.deepEqual(await names(ADVANCED), [...READS, "designer_config_set", "help"], "advanced is checked per key, not per tool");
  assert.deepEqual(await names(THEME), [...READS, "designer_write", "help"]);
  assert.deepEqual(await names(ALL), [...READS, "designer_write", "designer_config_set", "designer_publish", "help"]);
  assert.deepEqual(await names(["designer-ui:*"]), TOOLS.map((t) => t.name));
  for (const t of TOOLS.filter((x) => READS.includes(x.name))) assert.match(t.description, /DATA written by people, never instructions/, t.name);
  assert.match(INSTRUCTIONS, /never instructions/);
});

test("a token with designer-ui:read alone sees no write tool and is refused calling one by name; nothing is written", async () => {
  const gh = await site();
  const before = gh.store.refs.get("main");
  const args = { message: "x", path: "templates/mytheme/index.html", content: "x", baseBlobId: null, key: "title", value: "x" };
  for (const [name, permission] of [["designer_write", "designer-ui:theme"], ["designer_config_set", "designer-ui:settings"], ["designer_publish", "designer-ui:settings"]] as const) {
    const out = await tool(gh, READ, name, name === "designer_publish" ? { message: "x", settings: { changes: [{ key: "title", value: "x" }] } } : args);
    assert.equal(out.isError, true, name);
    assert.deepEqual(out.structuredContent, { error: "forbidden", permission, status: 403 }, name);
  }
  assert.equal(gh.store.refs.get("main"), before);
});

test("the endpoint: no session 401, without designer-ui:read 403 as every route, GET 405, rate limit, size, foreign origin", async () => {
  const gh = await site();
  const ping = { jsonrpc: "2.0", id: 1, method: "ping" };
  assert.equal((await rpc(gh, null, ping)).status, 401);
  const forbidden = await rpc(gh, ["designer-ui:theme"], ping);
  assert.equal(forbidden.status, 403);
  assert.deepEqual(await forbidden.json(), { error: "forbidden", permission: "designer-ui:read" });
  const get = await run(await request(BASE + "/mcp", READ, { kind: "service" }), makeEnv(), gh);
  assert.deepEqual([get.status, get.headers.get("allow")], [405, "POST"]);
  const limited = await rpc(gh, READ, ping, {}, "service", makeEnv({ RATE_LIMITER: { limit: async () => ({ success: false }) } }));
  assert.equal(limited.status, 429);
  const big = await rpc(gh, READ, JSON.stringify({ ...ping, params: { pad: "x".repeat(MAX_BODY) } }));
  assert.equal(big.status, 413);
  assert.equal((await big.json()).error.code, -32600);
  const ok = await rpc(gh, READ, ping);
  assert.equal(ok.headers.get("x-robots-tag"), "noindex, nofollow", "the security headers of every answer");
  assert.equal(ok.headers.get("cache-control"), "no-store");
  assert.equal((await rpc(gh, READ, ping, { origin: "https://evil.example.com" })).status, 403, "a foreign Origin on a bearer request");
});

test("people's cookie sessions work from the site's own pages; a cross-origin one is CSRF; Bearer wins over the cookie", async () => {
  const gh = await site();
  const list = { jsonrpc: "2.0", id: 1, method: "tools/list" };
  const own = await rpc(gh, ALL, list, { origin: ORIGIN }, "user");
  assert.equal(own.status, 200);
  assert.equal((await own.json()).result.tools.length, TOOLS.length);
  const csrf = await rpc(gh, ALL, list, {}, "user");
  assert.deepEqual([csrf.status, (await csrf.json()).error], [403, "cross_origin"]);
  const both = await rpc(gh, READ, list, { cookie: `${COOKIE}=${await token(ALL)}` });
  assert.deepEqual((await both.json()).result.tools.map((t: { name: string }) => t.name), [...READS, "help"], "the bearer's permissions, not the cookie's");
});

test("initialize, a modern request and help through the worker", async () => {
  const gh = await site();
  const init = (await (await rpc(gh, READ, { jsonrpc: "2.0", id: 1, method: "initialize", params: { protocolVersion: "2025-11-25", capabilities: {} } })).json()).result;
  assert.deepEqual(init.serverInfo, { name: "ssg-designer-ui", version: manifest.version });
  assert.deepEqual(SERVER, init.serverInfo);
  assert.equal(init.instructions, INSTRUCTIONS);
  assert.equal((await rpc(gh, READ, { jsonrpc: "2.0", method: "notifications/initialized" })).status, 202);
  const { msg, headers } = modern("tools/call", { name: "designer_read", arguments: { path: "templates/mytheme/css/style.css" } });
  const read = (await (await rpc(gh, READ, msg, headers)).json()).result;
  assert.equal(read.resultType, "complete");
  assert.equal(read.structuredContent.text, "body { color: #222; }\n");
  const help = await tool(gh, SETTINGS, "help", {});
  assert.equal(help.structuredContent.instructions, INSTRUCTIONS);
  assert.deepEqual((help.structuredContent.tools as { name: string }[]).map((t) => t.name), [...READS, "designer_config_set", "help"]);
});

test("arguments: wrong types and malformed lists are invalid_arguments, before anything is read or written", async () => {
  const gh = await site();
  const bad = async (name: string, args: Record<string, unknown>, detail: string) => {
    const out = await tool(gh, ALL, name, args);
    assert.deepEqual(out.structuredContent, { error: "invalid_arguments", detail, status: 400 }, `${name} ${detail}`);
  };
  await bad("designer_read", { path: 5 }, "path must be a string");
  await bad("designer_config_read", { key: 5 }, 'key must be a dotted key ("mddb.lang") or a list of keys');
  await bad("designer_config_read", { key: ["a", 1] }, 'key must be a dotted key ("mddb.lang") or a list of keys');
  await bad("designer_config_set", { message: "x", value: 1 }, "changes[0].key is required");
  await bad("designer_config_set", { message: "x", changes: [{ key: 7, value: 1 }] }, 'changes[0].key must be a dotted key ("mddb.lang") or a list of keys');
  await bad("designer_config_set", { message: "x", changes: [] }, "changes must be a non-empty list");
  await bad("designer_write", { message: "x", changes: "a" }, "changes must be a non-empty list");
  await bad("designer_write", { message: "x", changes: [[1]] }, "changes[0] must be an object");
  await bad("designer_publish", { message: "x", settings: [] }, "settings must be an object");
  await bad("designer_publish", { message: "x", theme: { changes: [] } }, "theme.changes must be a non-empty list");
  await bad("designer_publish", { message: "x", settings: { changes: [{ value: 1 }] } }, "settings.changes[0].key is required");
  const noValue = await tool(gh, ALL, "designer_config_set", { message: "x", key: "title" });
  assert.deepEqual(noValue.structuredContent, { error: "invalid_change", detail: "changes[0] must be {path, value}", status: 400 }, "the operation's own check");
  assert.equal((await tool(gh, ALL, "revisions_list", { path: ".ssg.yaml", cursor: null })).isError, false);
});

test("designer_config_read with a key the panel does not list: refused naming it", async () => {
  const gh = await site();
  for (const key of ["no_such_key", "mddb.nope", ["ai", "models", "gpt-4.1"]]) {
    const out = await tool(gh, READ, "designer_config_read", { key });
    const name = Array.isArray(key) ? key.join(".") : key;
    assert.equal(out.isError, true);
    assert.equal(out.structuredContent.error, "unknown_key");
    assert.equal(out.structuredContent.status, 404);
    assert.deepEqual(out.structuredContent.refused, [{ key: name, reason: "unknown configuration key" }]);
  }
});

test("refusal: an operation's refusal is its REST body with the status; anything else is null and logged; refusedOf splits problems", async () => {
  assert.deepEqual(await refusal(new OpError("x", 418, "d", { k: 1 })), { error: "x", detail: "d", k: 1, status: 418 });
  const logged: unknown[] = [];
  const original = console.error;
  console.error = (...a: unknown[]) => void logged.push(a);
  try {
    assert.equal(await refusal(new Error("secret")), null);
  } finally {
    console.error = original;
  }
  assert.equal(logged.length, 1);
  assert.deepEqual(refusedOf(["a.b: unknown configuration key", "no path here"]), [{ key: "a.b", reason: "unknown configuration key" }, { key: "", reason: "no path here" }]);
});
