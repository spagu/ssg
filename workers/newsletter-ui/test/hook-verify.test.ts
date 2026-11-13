// GO-127: the signature helpers' edge cases (the routes are in hooks.test.ts).
import { test } from "node:test";
import assert from "node:assert/strict";
import { b64decode, timingSafeEqual, verifyMessageSignature, verifySvix } from "../src/hook-verify.ts";
import { MC_KEYS, RESEND_SECRET, svixHeaders } from "./hook-fixtures.ts";

const enc = new TextEncoder();

test("timingSafeEqual: equal only for the same bytes, any lengths", () => {
  assert.equal(timingSafeEqual("abc", "abc"), true);
  assert.equal(timingSafeEqual("abc", "abd"), false);
  assert.equal(timingSafeEqual("abc", "abcd"), false);
  assert.equal(timingSafeEqual("abcd", "abc"), false);
  assert.equal(timingSafeEqual("", ""), true);
  assert.equal(b64decode("%%%"), null);
});

test("verifySvix accepts a secret without the whsec_ prefix", async () => {
  const body = '{"type":"email.sent"}';
  const headers = new Headers(await svixHeaders(body));
  assert.equal(await verifySvix(RESEND_SECRET.slice(6), headers, enc.encode(body)), true);
  assert.equal(await verifySvix(RESEND_SECRET, headers, enc.encode(body), Date.now() + 301_000), false, "the window is checked against now");
});

test("verifyMessageSignature refuses a request without the headers", async () => {
  assert.equal(await verifyMessageSignature(MC_KEYS.publicKey, new Headers(), enc.encode("[]")), false);
});
