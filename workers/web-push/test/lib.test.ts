// Small helpers. Run from the repository root:
//   node --import ./workers/web-push/test/resolve.mjs --test 'workers/web-push/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { b64urlDecode, intVar, timingSafeEqual } from "../functions/api/push/_lib.ts";

test("b64urlDecode takes padding and refuses junk", () => {
  assert.deepEqual(b64urlDecode("AQI="), new Uint8Array([1, 2]));
  assert.deepEqual(b64urlDecode("AQI"), new Uint8Array([1, 2]));
  assert.equal(b64urlDecode("A+B"), null);
  assert.equal(b64urlDecode("A"), null);
});

test("intVar falls back on junk", () => {
  assert.equal(intVar("5", 3), 5);
  assert.equal(intVar("0", 3), 3);
  assert.equal(intVar("x", 3), 3);
  assert.equal(intVar(undefined, 3), 3);
});

test("timingSafeEqual", () => {
  assert.ok(timingSafeEqual("abc", "abc"));
  assert.ok(!timingSafeEqual("abd", "abc"));
  assert.ok(!timingSafeEqual("ab", "abc"));
});
