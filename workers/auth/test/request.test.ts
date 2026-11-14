// Cookies, the CSRF rule, body parsing and Turnstile.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ACCESS_COOKIE, REFRESH_COOKIE, accessCookie, clearCookies, readCookie, refreshCookie } from "../functions/api/auth/_cookies.ts";
import { csrfProblem, isForm, readBody, str } from "../functions/api/auth/_request.ts";
import { SITEVERIFY, verifyTurnstile } from "../functions/api/auth/_turnstile.ts";
import { ORIGIN } from "./fakes.ts";

test("cookies: names, attributes, clearing, reading", () => {
  assert.equal(accessCookie("t", 600), `${ACCESS_COOKIE}=t; Max-Age=600; Path=/; Secure; HttpOnly; SameSite=Lax`);
  assert.equal(refreshCookie("r", 60), `${REFRESH_COOKIE}=r; Max-Age=60; Path=/api/auth/; Secure; HttpOnly; SameSite=Strict`);
  assert.match(ACCESS_COOKIE, /^__Host-/);
  assert.match(REFRESH_COOKIE, /^__Secure-/);
  for (const c of clearCookies()) assert.match(c, /=; Max-Age=0;/);
  const r = new Request(ORIGIN, { headers: { cookie: `junk; a=1; ${ACCESS_COOKIE}=tok=en ; b=2` } });
  assert.equal(readCookie(r, ACCESS_COOKIE), "tok=en");
  assert.equal(readCookie(r, "missing"), "");
  assert.equal(readCookie(new Request(ORIGIN), "a"), "");
});

const post = (headers: Record<string, string>) => new Request(ORIGIN + "/api/auth/x", { method: "POST", headers });
const FORM = { "content-type": "application/x-www-form-urlencoded" };

test("csrfProblem: header path, form path, and cross-origin evidence", () => {
  assert.equal(csrfProblem(post({ "x-ssg-auth": "1" }), false), null, "a script without Origin (curl) passes with the header");
  assert.equal(csrfProblem(post({ "x-ssg-auth": "1", origin: ORIGIN, "sec-fetch-site": "same-origin" }), false), null);
  assert.equal(csrfProblem(post({}), false), "csrf_check_failed");
  assert.equal(csrfProblem(post({ origin: ORIGIN }), false), "csrf_check_failed", "Origin alone is not proof on a JSON route");
  assert.equal(csrfProblem(post({ "x-ssg-auth": "yes" }), false), "csrf_check_failed");
  assert.equal(csrfProblem(post({ "x-ssg-auth": "1", origin: "https://evil.example.com" }), false), "cross_origin");
  assert.equal(csrfProblem(post({ "x-ssg-auth": "1", origin: "null" }), false), "cross_origin");
  assert.equal(csrfProblem(post({ "x-ssg-auth": "1", "sec-fetch-site": "same-site" }), false), "cross_origin");
  assert.equal(csrfProblem(post({ "x-ssg-auth": "1", "sec-fetch-site": "cross-site" }), false), "cross_origin");
  assert.equal(csrfProblem(post({ ...FORM, origin: ORIGIN }), true), null);
  assert.equal(csrfProblem(post({ ...FORM, "sec-fetch-site": "same-origin" }), true), null);
  assert.equal(csrfProblem(post({ ...FORM }), true), "csrf_check_failed", "a form with no evidence at all is refused");
  assert.equal(csrfProblem(post({ ...FORM, origin: ORIGIN }), false), "csrf_check_failed", "a JSON-only route refuses forms");
  assert.equal(csrfProblem(post({ origin: ORIGIN }), true), "csrf_check_failed", "the form path needs a form body");
});

test("isForm and readBody: JSON, urlencoded, multipart, limits", async () => {
  const body = (b: BodyInit, headers: Record<string, string> = {}) => new Request(ORIGIN, { method: "POST", body: b, headers });
  assert.equal(isForm(body("", { "content-type": "multipart/form-data; boundary=x" })), true);
  assert.equal(isForm(body("", { "content-type": "Application/X-WWW-Form-Urlencoded" })), true);
  assert.equal(isForm(body("", { "content-type": "application/json" })), false);
  assert.equal(isForm(new Request(ORIGIN)), false);
  assert.deepEqual(await readBody(body('{"a":1}', { "content-type": "application/json" })), { a: 1 });
  assert.equal(await readBody(body("[1]")), null);
  assert.equal(await readBody(body("null")), null);
  assert.equal(await readBody(body("{nope")), null);
  assert.deepEqual(await readBody(body("a=1&b=x%20y", FORM)), { a: "1", b: "x y" });
  const fd = new FormData();
  fd.append("email", "ada@example.com");
  fd.append("file", new Blob(["x"]), "x.txt");
  assert.deepEqual(await readBody(body(fd)), { email: "ada@example.com" }, "files are dropped");
  assert.equal(await readBody(body("--x\r\nbroken", { "content-type": "multipart/form-data; boundary=y" })), null);
  assert.equal(await readBody(body("{}", { "content-length": String(17 * 1024) })), null);
  assert.equal(await readBody(body(JSON.stringify({ a: "x".repeat(17 * 1024) }))), null);
  assert.equal(str("x"), "x");
  assert.equal(str(5), "");
});

test("verifyTurnstile: success, refusal, network failure, no token", async () => {
  const realFetch = globalThis.fetch;
  const seen: FormData[] = [];
  try {
    globalThis.fetch = (async (url: string, init: RequestInit) => {
      assert.equal(url, SITEVERIFY);
      seen.push(init.body as FormData);
      return Response.json({ success: (init.body as FormData).get("response") === "good" });
    }) as typeof fetch;
    assert.equal(await verifyTurnstile("sec", "good", "203.0.113.7"), true);
    assert.equal(await verifyTurnstile("sec", "bad", "203.0.113.7"), false);
    assert.equal(seen[0].get("secret"), "sec");
    assert.equal(seen[0].get("remoteip"), "203.0.113.7");
    assert.equal(await verifyTurnstile("sec", "", "203.0.113.7"), false);
    assert.equal(seen.length, 2, "no token, no call");
    globalThis.fetch = (async () => {
      throw new Error("down");
    }) as typeof fetch;
    assert.equal(await verifyTurnstile("sec", "good", "203.0.113.7"), false);
  } finally {
    globalThis.fetch = realFetch;
  }
});
