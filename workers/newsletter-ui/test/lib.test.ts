// Helpers: base path, headers, small parsers.
import { test } from "node:test";
import assert from "node:assert/strict";
import { CSP, basePath, clientIP, intVar, json, secure, sha256hex, underBase } from "../src/lib.ts";

const env = (base?: string) => ({ NEWSLETTER_UI_BASE: base }) as never;

test("basePath defaults and normalises", () => {
  assert.equal(basePath(env()), "/newsletter-admin");
  assert.equal(basePath(env("admin/news/")), "/admin/news");
  assert.equal(basePath(env(" /x// ")), "/x");
  assert.equal(basePath(env("/")), "");
});

test("basePath refuses anything that is not a plain path", () => {
  for (const bad of ["/a b", "/a?b", "/../x", "/x/.", "/a//b", "/a%2f"]) assert.equal(basePath(env(bad)), null, bad);
});

test("underBase", () => {
  assert.equal(underBase("/newsletter-admin", "/newsletter-admin"), "");
  assert.equal(underBase("/newsletter-admin/api/x", "/newsletter-admin"), "/api/x");
  assert.equal(underBase("/newsletter-administrator", "/newsletter-admin"), null);
  assert.equal(underBase("/", "/newsletter-admin"), null);
  assert.equal(underBase("/anything", ""), "/anything");
});

test("intVar", () => {
  assert.equal(intVar("5", 10), 5);
  assert.equal(intVar(undefined, 10), 10);
  assert.equal(intVar("0", 10), 10);
  assert.equal(intVar("x", 10), 10);
});

test("json carries the security headers and extra headers", async () => {
  const res = json({ a: 1 }, 418, { "retry-after": "3" });
  assert.equal(res.status, 418);
  assert.equal(res.headers.get("retry-after"), "3");
  assert.equal(res.headers.get("content-type"), "application/json; charset=utf-8");
  assert.equal(res.headers.get("cache-control"), "no-store");
  assert.equal(res.headers.get("x-robots-tag"), "noindex, nofollow");
  assert.deepEqual(await res.json(), { a: 1 });
});

test("secure overrides caching on immutable fetched responses", async () => {
  const upstream = await fetch("data:text/plain,hi");
  const res = secure(upstream);
  assert.equal(res.headers.get("cache-control"), "no-store");
  assert.equal(res.headers.get("content-security-policy"), CSP);
  assert.equal(res.headers.get("referrer-policy"), "no-referrer");
  assert.equal(res.headers.get("x-content-type-options"), "nosniff");
  assert.equal(await res.text(), "hi");
});

test("the CSP allows nothing inline", () => {
  assert.doesNotMatch(CSP, /unsafe-inline|unsafe-eval|\*/);
  assert.match(CSP, /default-src 'none'/);
  assert.match(CSP, /frame-ancestors 'none'/);
});

test("clientIP and sha256hex", async () => {
  assert.equal(clientIP(new Request("https://example.com", { headers: { "cf-connecting-ip": "198.51.100.1" } })), "198.51.100.1");
  assert.equal(clientIP(new Request("https://example.com")), "unknown");
  assert.equal(await sha256hex("abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
});
