// The small helpers: answers and headers, encodings, hashing, settings, the
// `next` rule. Cookies, CSRF, bodies and Turnstile are in request.test.ts.
import { test } from "node:test";
import assert from "node:assert/strict";
import * as lib from "../functions/api/auth/_lib.ts";
import { DEFAULT_ITERATIONS, MIN_ITERATIONS, settings } from "../functions/api/auth/_config.ts";
import { safeNext, withParams } from "../functions/api/auth/_next.ts";
import { ORIGIN } from "./fakes.ts";

test("json, redirect and secure: security headers, cookies appended, extra headers win", async () => {
  const r = lib.json({ a: 1 }, 418, { "cache-control": "public" }, ["a=1", "b=2"]);
  assert.equal(r.status, 418);
  assert.deepEqual(await r.json(), { a: 1 });
  assert.equal(r.headers.get("cache-control"), "public");
  assert.equal(r.headers.get("x-frame-options"), "DENY");
  assert.deepEqual(r.headers.getSetCookie(), ["a=1", "b=2"]);
  const d = lib.redirect("/x", ["c=3"]);
  assert.equal(d.status, 303);
  assert.equal(d.headers.get("location"), "/x");
  assert.equal(d.headers.get("cache-control"), "no-store");
  assert.deepEqual(d.headers.getSetCookie(), ["c=3"]);
  assert.equal(lib.json(null).headers.get("content-security-policy")?.includes("default-src 'none'"), true);
  assert.equal(lib.redirect("/").headers.getSetCookie().length, 0);
});

test("intVar, iso, clientIP, normEmail, looksLikeEmail", () => {
  assert.equal(lib.intVar("7", 3), 7);
  assert.equal(lib.intVar("0", 3), 3);
  assert.equal(lib.intVar("x", 3), 3);
  assert.equal(lib.intVar(undefined, 3), 3);
  assert.equal(lib.iso(0), "1970-01-01T00:00:00.000Z");
  assert.ok(Math.abs(lib.clock.now() - Date.now()) < 1000, "the real clock unless a test pins it");
  assert.equal(lib.clientIP(new Request(ORIGIN, { headers: { "cf-connecting-ip": "198.51.100.1" } })), "198.51.100.1");
  assert.equal(lib.clientIP(new Request(ORIGIN)), "unknown");
  assert.equal(lib.normEmail("  Ada@Example.COM "), "ada@example.com");
  assert.equal(lib.normEmail(42), "");
  assert.equal(lib.looksLikeEmail("ada@example.com"), true);
  assert.equal(lib.looksLikeEmail("ada@example"), false);
  assert.equal(lib.looksLikeEmail("a b@example.com"), false);
  assert.equal(lib.looksLikeEmail("a".repeat(250) + "@example.com"), false);
});

test("base64url round trip, and garbage decodes to null", () => {
  const bytes = new Uint8Array([0, 1, 250, 251, 252, 253, 254, 255]);
  const s = lib.b64urlEncode(bytes);
  assert.doesNotMatch(s, /[+/=]/);
  assert.deepEqual(lib.b64urlDecode(s), bytes);
  assert.equal(lib.b64urlDecode("ab+c"), null);
  assert.equal(lib.b64urlDecode("abc="), null);
  assert.equal(lib.b64urlDecode("abcde"), null, "a length of 4n+1 is never base64");
  assert.deepEqual(lib.b64urlDecode(""), new Uint8Array());
});

test("hashing helpers and comparisons", async () => {
  assert.equal(await lib.sha256hex("abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
  assert.equal(await lib.saltedHash({ AUTH_HASH_SALT: "s" } as lib.Env, null), null);
  assert.equal(await lib.saltedHash({} as lib.Env, "1.2.3.4"), null);
  assert.equal(await lib.saltedHash({ AUTH_HASH_SALT: "s" } as lib.Env, "x"), await lib.sha256hex("sx"));
  assert.equal(lib.bytesEqual(new Uint8Array([1, 2]), new Uint8Array([1, 2])), true);
  assert.equal(lib.bytesEqual(new Uint8Array([1, 2]), new Uint8Array([1, 3])), false);
  assert.equal(lib.bytesEqual(new Uint8Array([1]), new Uint8Array([1, 2])), false);
  assert.equal(await lib.secretEqual("abc", "abc"), true);
  assert.equal(await lib.secretEqual("abd", "abc"), false);
  assert.equal(await lib.secretEqual("", "abc"), false);
  const t = lib.randomToken();
  assert.equal(lib.isToken(t), true);
  assert.notEqual(t, lib.randomToken());
  assert.equal(lib.isToken("short"), false);
  assert.equal(lib.isToken(null), false);
});

test("settings: defaults from the request, overrides from vars", () => {
  const r = new Request("https://example.com:8443/api/auth/login");
  const d = settings({} as lib.Env, r);
  assert.equal(d.issuer, "https://example.com:8443/api/auth");
  assert.equal(d.audience, "example.com:8443");
  assert.equal(d.tokenSeconds, 600);
  assert.equal(d.sessionSeconds, 14 * 86400);
  assert.equal(d.iterations, DEFAULT_ITERATIONS);
  assert.equal(d.turnstileSecret, undefined);
  assert.equal(d.loginUrl, "/auth/login");
  assert.equal(d.afterLoginUrl, "/");
  const o = settings(
    {
      AUTH_ISSUER: "https://id.example.com",
      AUTH_AUDIENCE: "example.com",
      AUTH_PBKDF2_ITERATIONS: "500",
      TURNSTILE_SECRET: "shared",
      AUTH_LOGIN_URL: "/in",
      AUTH_AFTER_LOGIN_URL: "/home",
    } as lib.Env,
    r,
  );
  assert.equal(o.issuer, "https://id.example.com");
  assert.equal(o.audience, "example.com");
  assert.equal(o.iterations, MIN_ITERATIONS, "a cost below the floor is raised to it");
  assert.equal(o.turnstileSecret, "shared");
  assert.equal(settings({ AUTH_TURNSTILE_SECRET: "own", TURNSTILE_SECRET: "shared" } as lib.Env, r).turnstileSecret, "own");
  assert.equal(o.loginUrl, "/in");
  assert.equal(o.afterLoginUrl, "/home");
});

test("safeNext: only a path on this host", () => {
  const ok: [unknown, string][] = [
    ["/admin", "/admin"],
    ["/a/b?c=1#d", "/a/b?c=1#d"],
    ["/a/../b", "/b"],
    ["/%2F%2Fevil.example.com", "/%2F%2Fevil.example.com"],
    ["/ /x", "/%20/x"],
  ];
  for (const [raw, want] of ok) assert.equal(safeNext(raw, "F"), want, String(raw));
  const bad: unknown[] = [
    undefined,
    42,
    "",
    "admin",
    "//evil.example.com",
    "/\\evil.example.com",
    "\\\\evil.example.com",
    "https://evil.example.com/",
    "javascript:alert(1)",
    "/\t/evil.example.com",
    "/\n/evil.example.com",
    "/x\u007f",
    "/" + "a".repeat(512),
  ];
  for (const raw of bad) assert.equal(safeNext(raw, "F"), "F", JSON.stringify(raw));
});

test("withParams keeps the path's own query and skips empty values", () => {
  assert.equal(withParams("/auth/login", { error: "x", next: "" }), "/auth/login?error=x");
  assert.equal(withParams("/auth/login?lang=pl", { error: "x", next: "/a?b=1" }), "/auth/login?lang=pl&error=x&next=%2Fa%3Fb%3D1");
  assert.equal(withParams("https://id.example.com/in", { error: "x" }), "https://id.example.com/in?error=x");
});
