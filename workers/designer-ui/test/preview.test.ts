// GET {base}/api/preview end to end (GO-188): through the whole pipeline
// (gate, permission) and preview-entry.ts's headers, against a fake site on
// the same host. What leaves the worker carries no credential of the caller;
// another host and a redirect off the host are refused; the page is inert.
import { mock, test } from "node:test";
import assert from "node:assert/strict";
import { BASE, ORIGIN, makeEnv, request } from "./fixtures.ts";
import { handleFetch } from "../portable/app.ts";
import handler from "../src/index.ts";
import { MAX_HOPS, MAX_PAGE, PreviewError, fetchPage, sitePath } from "../src/preview-fetch.ts";
import { feedHref, firstEntry, latestPath } from "../src/preview-latest.ts";
import { LANGS, errorPage, langOf } from "../src/preview-messages.ts";
import { PREVIEW_CSP, withPreview } from "../src/preview-entry.ts";
import { MAX_TOKENS, tokensCSS, tokensFrom } from "../src/preview-tokens.ts";

type Answer = Response | (() => Response);
// fakeSite: the host's public pages; every outgoing request is recorded.
function fakeSite(pages: Record<string, Answer>) {
  const sent: { url: string; init: RequestInit }[] = [];
  const fetch = async (url: string, init: RequestInit): Promise<Response> => {
    sent.push({ url, init });
    const u = new URL(url);
    const a = pages[u.pathname + u.search] ?? pages[u.pathname];
    if (!a) return new Response("nope", { status: 404 });
    return typeof a === "function" ? a() : a.clone();
  };
  return { fetch, sent };
}
const html = (body: string, headers: Record<string, string> = {}): Answer => () => new Response(body, { headers: { "content-type": "text/html; charset=utf-8", "set-cookie": "site=1", ...headers } });
const redirect = (to: string): Answer => () => new Response(null, { status: 301, headers: { location: to } });

const POST = '<!doctype html><html><head><link rel="stylesheet" href="/css/style.css"></head><body><h1>Bread</h1><script>alert(1)</script><p onclick="x()">x</p><a href="javascript:alert(2)">y</a></body></html>';
const HOME = '<!doctype html><html><head><link rel="alternate" type="application/atom+xml" href="https://elsewhere.example.com/feed.xml?x=1"></head><body>home</body></html>';
const FEED = '<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry><title>b</title><link href="https://example.com/2026/bread/"/></entry><entry><link href="/old/"/></entry></feed>';

async function preview(query: string, site: ReturnType<typeof fakeSite>, perms: string[] | null = ["designer-ui:read"], init: RequestInit = {}) {
  const env = makeEnv() as never;
  const r = await request(`${BASE}/api/preview${query}`, perms, init);
  const res = await withPreview(r, env, (req) => handleFetch(req, env, { fetch: site.fetch }));
  return { res, text: await res.text() };
}

test("a page: inert, with the tokens, the base, the forced scheme and the preview's own headers", async () => {
  const site = fakeSite({ "/blog": redirect("/blog/"), "/blog/": html(POST) });
  const { res, text } = await preview("?path=/blog&scheme=dark&--color-accent=%23b42318&--font-sans=%22Inter%22%2C%20serif&lang=pl", site);
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("content-security-policy"), PREVIEW_CSP);
  assert.match(PREVIEW_CSP, /default-src 'none'.*frame-ancestors 'self'; sandbox$/);
  assert.doesNotMatch(PREVIEW_CSP, /script-src/);
  assert.equal(res.headers.get("x-frame-options"), "SAMEORIGIN");
  assert.equal(res.headers.get("set-cookie"), null, "the site's cookies stay with the worker");
  assert.equal(res.headers.get("cache-control"), "no-store");
  assert.match(text, /^<!doctype html><html data-theme="dark"><head><base href="https:\/\/example.com\/blog\/"><style>:root:root:root:root\{--color-accent:#b42318;--font-sans:"Inter", serif;color-scheme:dark\}<\/style>/);
  assert.doesNotMatch(text, /<script|onclick|javascript:/i);
  assert.deepEqual(site.sent.map((s) => s.url), ["https://example.com/blog", "https://example.com/blog/"]);
});

test("nothing of the caller travels: no cookie, no Authorization, no header of the request", async () => {
  const site = fakeSite({ "/": html("<p>home</p>") });
  const { res } = await preview("", site, ["designer-ui:read"], { headers: { "x-forwarded-for": "203.0.113.9", referer: `${ORIGIN}${BASE}/` } });
  assert.equal(res.status, 200);
  const bearer = await preview("", site, ["designer-ui:read"], { kind: "service" } as never);
  assert.equal(bearer.res.status, 200);
  assert.equal(site.sent.length, 2);
  for (const { url, init } of site.sent) {
    assert.equal(url, "https://example.com/");
    assert.equal(init.redirect, "manual");
    assert.deepEqual(Object.keys(init.headers as Record<string, string>).sort(), ["accept", "user-agent"]);
    assert.equal(init.credentials, undefined);
  }
});

test("off the host, the panel itself, dot segments: refused before any fetch", async () => {
  const site = fakeSite({});
  for (const path of ["//evil.example.com/", "https://evil.example.com/", "/\\evil.example.com", "/a/../b", "/%2e%2e/x", "/designer/", "/designer", "blog/", "/a b", "/" + "a".repeat(2048)]) {
    const { res, text } = await preview(`?path=${encodeURIComponent(path)}`, site);
    assert.equal(res.status, 400, path);
    assert.match(text, /<code>bad_path<\/code>/);
  }
  assert.equal(site.sent.length, 0);
  assert.equal(sitePath(null, "/designer"), "/");
  assert.equal(sitePath("/designers/x?q=1", "/designer"), "/designers/x?q=1");
  assert.equal(sitePath("/designer/x", ""), "/designer/x", "a worker on the whole host refuses nothing by base");
});

test("redirects: same origin followed up to MAX_HOPS; off the host or one more refused", async () => {
  const off = fakeSite({ "/": redirect("https://evil.example.com/") });
  assert.match((await preview("", off)).text, /redirect_off_host/);
  assert.equal(off.sent.length, 1, "never followed");
  const http = fakeSite({ "/": redirect("http://example.com/") });
  assert.equal((await preview("", http)).res.status, 502, "another scheme is another origin");
  const loop = fakeSite({ "/": redirect("/") });
  const { res, text } = await preview("", loop);
  assert.equal(res.status, 502);
  assert.match(text, /too_many_redirects/);
  assert.equal(loop.sent.length, MAX_HOPS + 1);
  const bodied = fakeSite({ "/": () => new Response("moved", { status: 307, headers: { location: "https://evil.example.com/" } }) });
  assert.match((await preview("", bodied)).text, /redirect_off_host/);
  const broken = fakeSite({ "/": () => new Response(null, { status: 302, headers: { location: "http://[::1" } }) });
  assert.match((await preview("", broken)).text, /page_unavailable/);
  const bare = fakeSite({ "/": () => new Response(null, { status: 302 }) });
  assert.match((await preview("", bare)).text, /too_many_redirects/, "a redirect without Location stays put");
});

test("what the site answers: 404, other errors, not HTML, too large (declared or streamed), network failure, empty", async () => {
  const cases: [Answer, number, string][] = [
    [() => new Response("x", { status: 500 }), 502, "page_unavailable"],
    [() => new Response("{}", { headers: { "content-type": "application/json" } }), 415, "not_html"],
    [() => new Response("x"), 415, "not_html"],
    [() => new Response(null), 415, "not_html"],
    [html("x", { "content-length": String(MAX_PAGE + 1) }), 413, "page_too_large"],
    [() => new Response(new ReadableStream({ pull: (c) => c.enqueue(new Uint8Array(64 * 1024)) }), { headers: { "content-type": "text/html" } }), 413, "page_too_large"],
    [() => { throw new TypeError("network down at 10.0.0.1"); }, 502, "page_unavailable"],
  ];
  for (const [answer, status, code] of cases) {
    const { res, text } = await preview("", fakeSite({ "/": answer }));
    assert.equal(res.status, status, code);
    assert.match(text, new RegExp(`<code>${code}</code>`));
    assert.doesNotMatch(text, /10\.0\.0\.1/);
  }
  assert.equal((await preview("?path=/gone/", fakeSite({}))).res.status, 404);
  const empty = await fetchPage(async () => new Response(null, { headers: { "content-type": "text/html" } }), ORIGIN, "/");
  assert.deepEqual(empty, { url: "https://example.com/", text: "" });
});

test("latest post: home page -> its feed -> first entry; only paths, always on this host", async () => {
  const site = fakeSite({ "/": html(HOME), "/feed.xml?x=1": () => new Response(FEED, { headers: { "content-type": "application/atom+xml" } }), "/2026/bread/": html(POST) });
  const { res, text } = await preview("?page=latest&path=/ignored/", site);
  assert.equal(res.status, 200);
  assert.match(text, /<h1>Bread<\/h1>/);
  assert.deepEqual(site.sent.map((s) => s.url), ["https://example.com/", "https://example.com/feed.xml?x=1", "https://example.com/2026/bread/"]);
  assert.match(String((site.sent[1].init.headers as Record<string, string>).accept), /application\/atom\+xml/);
  for (const [home, feed] of [
    ["<p>no feed</p>", FEED],
    ['<link rel="alternate" type="application/rss+xml" href="javascript:x">', FEED],
    ['<link rel="alternate" type="application/rss+xml" href="/designer/feed">', FEED],
    ['<link rel="alternate" type="application/rss+xml" href="/rss.xml">', "<rss><channel><item><title>x</title></item></channel></rss>"],
    ['<link rel="alternate" type="application/rss+xml" href="/rss.xml">', "<rss><channel><item><link>http://[bad</link></item></channel></rss>"],
  ]) {
    const s = fakeSite({ "/": html(home), "/rss.xml": () => new Response(feed, { headers: { "content-type": "application/rss+xml" } }), "/feed.xml": () => new Response(feed, { headers: { "content-type": "application/rss+xml" } }) });
    const out = await preview("?page=latest&lang=de", s);
    assert.equal(out.res.status, 404, home);
    assert.match(out.text, /no_latest_post/);
    assert.match(out.text, /<html lang="de">/);
  }
});

test("feeds: RSS, Atom and JSON Feed entries; feed links by rel and type", () => {
  assert.equal(firstEntry("<rss><channel><link>/site</link><item><link> /a?x=1&amp;y=2 </link></item><item><link>/b</link></item></channel></rss>"), "/a?x=1&y=2");
  assert.equal(firstEntry("<feed><entry><link rel='alternate' href='/c'/></entry></feed>"), "/c");
  assert.equal(firstEntry(' {"items":[{"url":"/d"},{"url":"/e"}]}'), "/d");
  assert.equal(firstEntry('{"items":[{"id":1}]}'), null);
  assert.equal(firstEntry('{"items":'), null);
  assert.equal(firstEntry("<rss></rss>"), null);
  assert.equal(feedHref('<link rel="stylesheet" href="/x.css"><link rel="Alternate" type="application/feed+json" href=" /f.json ">'), "/f.json");
  assert.equal(feedHref('<link rel="alternate" type="text/html" href="/pl/"><link rel="alternate" type="application/rss+xml">'), null);
});

test("latestPath with a JSON feed", async () => {
  const site = fakeSite({ "/": html('<link rel="alternate" type="application/feed+json" href="/feed.json">'), "/feed.json": () => Response.json({ items: [{ url: "https://www.example.com/p/?a=1" }] }) });
  assert.equal(await latestPath(site.fetch, ORIGIN, "/designer"), "/p/?a=1");
});

test("tokens: plain colours and font stacks only; anything that could leave the declaration is refused", async () => {
  const q = (s: string) => new URLSearchParams(s);
  assert.deepEqual(tokensFrom(q("path=/&--a=%23fff&--b=rgb(0 0 0 / 50%25)&--c=%22Geist%22, system-ui")), [["--a", "#fff"], ["--b", "rgb(0 0 0 / 50%)"], ["--c", '"Geist", system-ui']]);
  for (const bad of ["--a=red;}body{x:y", "--a=</style><script>", "--a=url(https://example.com/x)", "--a=expression(alert(1))", "--a=a\\62 c", "--a=@import", "--a=", "-x--=1&--=a", "--a b=red", `--${"a".repeat(101)}=red`, "--a=" + "a".repeat(201)]) {
    assert.throws(() => tokensFrom(q(bad)), (e: unknown) => e instanceof PreviewError && e.code === "bad_token" && e.status === 400, bad);
  }
  assert.throws(() => tokensFrom(q(Array.from({ length: MAX_TOKENS + 1 }, (_, i) => `--t${i}=red`).join("&"))), /bad_token/);
  assert.equal(tokensCSS([], null), "");
  assert.equal(tokensCSS([], "light"), ":root:root:root:root{color-scheme:light}");
  const { res, text } = await preview("?--a=red;}", fakeSite({}));
  assert.equal(res.status, 400);
  assert.match(text, /bad_token/);
});

test("error pages: five languages, our text only, English for anything else", () => {
  for (const lang of LANGS) assert.match(errorPage(lang, "bad_path"), new RegExp(`<html lang="${lang}">`));
  assert.equal(langOf("xx"), "en");
  assert.equal(langOf(null), "en");
  assert.match(errorPage("en", "<b>unknown</b>"), /The site did not answer[\s\S]*<code>&lt;b&gt;unknown&lt;\/b&gt;<\/code>/);
});

test("the pipeline first: no session, no read permission", async () => {
  const site = fakeSite({ "/": html("<p>x</p>") });
  const anon = await preview("", site, null);
  assert.equal(anon.res.status, 401);
  assert.equal(anon.res.headers.get("content-security-policy"), PREVIEW_CSP, "even a refusal is never a page that runs");
  const other = await preview("", site, ["editor-ui:read"]);
  assert.equal(other.res.status, 403);
  assert.equal(site.sent.length, 0);
});

test("the panel's page may frame the preview; JSON, other paths and other bases pass untouched", async () => {
  const page = { fetch: async () => new Response("<!doctype html>", { headers: { "content-type": "text/html; charset=utf-8" } }) };
  const env = makeEnv({ ASSETS: page });
  const res = await handler.fetch(await request(`${BASE}/`, ["designer-ui:read"]), env as never);
  assert.match(String(res.headers.get("content-security-policy")), /frame-ancestors 'none'; frame-src 'self'$/);
  const api = await handler.fetch(await request(`${BASE}/api/module`, ["designer-ui:read"]), env as never);
  assert.doesNotMatch(String(api.headers.get("content-security-policy")), /frame-src/);
  const outside = await handler.fetch(await request("/elsewhere", ["designer-ui:read"]), env as never);
  assert.equal(outside.status, 404);
  const badBase = await handler.fetch(await request(`${BASE}/`, ["designer-ui:read"]), makeEnv({ DESIGNER_BASE: "/a b" }) as never);
  assert.equal(badBase.status, 503);
  assert.doesNotMatch(String(badBase.headers.get("content-security-policy")), /frame-src/);
});

test("the worker's own fetch when none is injected; an unexpected failure is a plain 500", async () => {
  const real = globalThis.fetch;
  const seen: string[] = [];
  globalThis.fetch = (async (url: string) => (seen.push(url), new Response("<p>x</p>", { headers: { "content-type": "text/html" } }))) as typeof fetch;
  try {
    const env = makeEnv() as never;
    const res = await withPreview(await request(`${BASE}/api/preview?path=/x/`, ["designer-ui:read"]), env, (r) => handleFetch(r, env));
    assert.equal(res.status, 200);
    assert.deepEqual(seen, ["https://example.com/x/"]);
  } finally {
    globalThis.fetch = real;
  }
  const odd = { fetch: async () => ({ status: 200, ok: true, headers: { get: () => { throw new Error("odd"); } } }) as unknown as Response };
  const env = makeEnv() as never;
  const err = mock.method(console, "error", () => {});
  try {
    const res = await withPreview(await request(`${BASE}/api/preview`, ["designer-ui:read"]), env, (r) => handleFetch(r, env, odd));
    assert.equal(res.status, 500);
    assert.deepEqual(await res.json(), { error: "internal_error" });
  } finally {
    err.mock.restore();
  }
});
