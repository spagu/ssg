// repo-github-http.ts: our error codes, rate limits, no redirect ever followed, the token never in any output.
import { test } from "node:test";
import assert from "node:assert/strict";
import { setup, rejects, AUTHOR, config } from "./repo-fixtures";
import { API, TOKEN } from "./fake-github";
import { githubHttp, toError, StaleRef } from "../portable/repo-github-http";
import { githubProvider } from "../portable/repo-github";
import { RepoError } from "../portable/repo-provider";

const res = (status: number, body: unknown, headers: Record<string, string> = {}) => new Response(JSON.stringify(body), { status, headers });
const NOW = Date.UTC(2026, 10, 14, 12, 0, 0);
const map = async (r: Response, method: "GET" | "PATCH" = "GET") => {
  const e = await toError(r, { method, path: "/x" }, NOW);
  return e instanceof RepoError ? [e.code, e.retryAt ?? null] : [e.name, null];
};

test("provider statuses map to our codes; the provider body never becomes our message", async () => {
  const cases: [Response, "GET" | "PATCH", unknown[]][] = [
    [res(401, { message: "Bad credentials" }), "GET", ["token_rejected", null]],
    [res(403, { message: "Resource not accessible by personal access token" }), "GET", ["token_rejected", null]],
    [res(404, { message: "Not Found" }), "GET", ["not_found", null]],
    [res(409, { message: "Git Repository is empty." }), "GET", ["not_found", null]],
    [res(409, { message: "Conflict" }), "GET", ["provider_error", null]],
    [res(413, {}), "GET", ["too_large", null]],
    [res(422, { message: "content is too large" }), "GET", ["too_large", null]],
    [res(422, { message: "Protected branch update failed for refs/heads/main." }), "PATCH", ["branch_protected", null]],
    [res(422, { message: "Repository rule violations found\n\nChanges must be made through a pull request." }), "PATCH", ["branch_protected", null]],
    [res(409, { message: "Required status check \"ci\" is expected." }), "PATCH", ["branch_protected", null]],
    [res(422, { message: "Update is not a fast forward" }), "PATCH", ["StaleRef", null]],
    [res(422, { message: "Update is not a fast forward" }), "GET", ["provider_error", null]],
    [res(422, { message: "Validation Failed" }), "GET", ["provider_error", null]],
    [res(500, "x"), "GET", ["provider_error", null]],
    [new Response("<html>", { status: 502 }), "GET", ["provider_error", null]],
    [new Response(null, { status: 302, headers: { location: "https://evil.example.com/" } }), "GET", ["provider_error", null]],
    // rate limits: reset header, retry-after, secondary message without headers (one minute), 429
    [res(403, { message: "API rate limit exceeded" }, { "x-ratelimit-remaining": "0", "x-ratelimit-reset": String(NOW / 1000 + 600) }), "GET", ["rate_limited", new Date(NOW + 600_000).toISOString()]],
    [res(429, {}, { "retry-after": "30", "x-ratelimit-reset": "1" }), "GET", ["rate_limited", new Date(NOW + 30_000).toISOString()]],
    [res(403, { message: "You have exceeded a secondary rate limit." }), "GET", ["rate_limited", new Date(NOW + 60_000).toISOString()]],
    [res(403, {}, { "retry-after": "soon" }), "GET", ["rate_limited", new Date(NOW + 60_000).toISOString()]],
    [res(429, {}, { "x-ratelimit-reset": "0" }), "GET", ["rate_limited", new Date(NOW + 60_000).toISOString()]],
  ];
  for (const [r, method, want] of cases) assert.deepEqual(await map(r, method), want, `${r.status} ${method}`);
  assert.equal(new StaleRef().message, "stale_ref");
});

test("network failures and timeouts become provider_error without the thrown text", async () => {
  const http = githubHttp(config(), { fetch: async () => { throw new Error(`connect failed Authorization: Bearer ${TOKEN}`); } });
  const e = await rejects(http.json({ path: "" }));
  assert.equal(e.code, "provider_error");
  assert.ok(!e.message.includes(TOKEN));
  const slow = githubHttp(config(), { timeoutMs: 5, fetch: (_u, init) => new Promise((_r, reject) => init.signal!.addEventListener("abort", () => reject(init.signal!.reason))) });
  assert.equal((await rejects(slow.json({ path: "" }))).code, "provider_error");
  const notJson = githubHttp(config(), { fetch: async () => new Response("<html>", { status: 200 }) });
  assert.equal((await rejects(notJson.json({ path: "" }))).code, "provider_error");
});

test("subrequest budget: refused before sending, counted per provider", async () => {
  const { gh, provider } = await setup({ maxSubrequests: 2 });
  await provider.head("main");
  assert.equal(provider.used(), 1);
  const e = await rejects(provider.read("main", "content/posts/hello.md"));
  assert.equal(e.code, "provider_error");
  assert.equal(e.detail, "subrequest budget exhausted");
  assert.equal(gh.requests.length, 2);
});

test("redirects are never followed: a fetch that would follow one never reaches the other host", async () => {
  const { gh } = await setup();
  const hosts: string[] = [];
  // A fetch that behaves like the platform's: it follows unless told redirect: "manual".
  const following = async (url: string, init: RequestInit): Promise<Response> => {
    hosts.push(new URL(url).host);
    if (url.startsWith(gh.base) && !url.includes("evil")) {
      const r = new Response(null, { status: 301, headers: { location: "https://evil.example.com/steal" } });
      return init.redirect === "manual" ? r : following("https://evil.example.com/steal", init);
    }
    return new Response("{}", { status: 200 });
  };
  const p = githubProvider(config(), { fetch: following });
  assert.equal((await rejects(p.head("main"))).code, "provider_error");
  assert.equal((await rejects(p.read("main", "content/posts/hello.md"))).code, "provider_error");
  assert.deepEqual([...new Set(hosts)], ["api.github.com"]);
});

test("the token never appears in a response, error, log line — and is sent only to the API host", async () => {
  const { gh, provider, blob } = await setup();
  const logs: string[] = [];
  const saved = { ...console };
  for (const k of ["log", "info", "warn", "error", "debug"] as const) console[k] = (...a: unknown[]) => { logs.push(a.map(String).join(" ")); };
  const outputs: unknown[] = [];
  const run = async (p: Promise<unknown>) => {
    try { outputs.push(await p); } catch (e) { outputs.push(e, (e as Error).message, (e as Error).stack, JSON.stringify(e)); }
  };
  try {
    await run(provider.read("main", "content/posts/hello.md"));
    await run(provider.tree("main", "content"));
    await run(provider.history("content/posts/hello.md", null));
    await run(provider.commit({ branch: "main", message: "m", author: AUTHOR, changes: [{ op: "put", path: "content/posts/hello.md", content: { text: "x" }, baseBlobId: blob("content/posts/hello.md") }] }));
    // the provider echoes our request (token included) in its error bodies
    gh.override = (_m, p) => new Response(JSON.stringify({ message: `Bad ${TOKEN} at ${p}`, token: TOKEN }), { status: [401, 403, 404, 409, 422, 500][p.length % 6] });
    for (const op of [provider.head("main"), provider.read("main", "content/posts/hello.md"), provider.tree("main", "content"), provider.history("content/posts/hello.md", null),
      provider.commit({ branch: "main", message: "m", author: AUTHOR, changes: [{ op: "put", path: "content/posts/x.md", content: { text: "x" }, baseBlobId: null }] })]) await run(op);
    const bad = githubProvider(config({ EDITOR_GIT_REPO: "github.com/acme/site", EDITOR_GIT_TOKEN: "ghp_wrongWRONGwrong123" }), { fetch: gh.fetch });
    await run(bad.head("main"));
  } finally {
    Object.assign(console, saved);
  }
  const text = (v: unknown) => (v instanceof Error ? `${v.message} ${v.stack} ${JSON.stringify(v)} ${JSON.stringify({ ...v })}` : JSON.stringify(v, (_k, x) => (x instanceof Uint8Array ? new TextDecoder().decode(x) : x)));
  const all = outputs.map(text).join("\n") + logs.join("\n");
  assert.ok(outputs.length > 10);
  assert.ok(!all.includes(TOKEN.slice(4)), "token leaked into an output");
  assert.ok(!all.includes("wrongWRONG"));
  for (const r of gh.requests) {
    assert.ok(r.url.startsWith(`${API}/repos/acme/site`), r.url);
    assert.equal(r.redirect, "manual");
  }
});

test("defaults: the platform fetch and clock, 15 s timeout", async () => {
  const saved = globalThis.fetch;
  const seen: RequestInit[] = [];
  globalThis.fetch = (async (_u: string, init: RequestInit) => { seen.push(init); return res(429, {}, {}); }) as typeof fetch;
  try {
    const before = Date.now();
    const e = await rejects(githubHttp(config()).json({ path: "" }));
    assert.equal(e.code, "rate_limited");
    assert.ok(Date.parse(e.retryAt!) >= before + 60_000);
    assert.ok(seen[0].signal instanceof AbortSignal);
  } finally {
    globalThis.fetch = saved;
  }
});
