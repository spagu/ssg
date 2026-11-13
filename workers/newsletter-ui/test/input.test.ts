// Input rules mirrored from the sign-up form, and the small parsers around them.
import { test } from "node:test";
import assert from "node:assert/strict";
import { escapeLike, isStatus, isTrue, normaliseEmail, normaliseLang, parseTags, readJSONObject, splitTags, text } from "../src/input.ts";

test("normaliseEmail: the form's rules", () => {
  assert.equal(normaliseEmail("  Ann@Example.COM "), "ann@example.com");
  for (const bad of ["", "ann", "ann@example", "a b@example.com", "a@b@example.com", "a,b@example.com", "<a>@example.com", 5, null]) {
    assert.equal(normaliseEmail(bad), null, String(bad));
  }
  assert.equal(normaliseEmail("x".repeat(65) + "@example.com"), null);
  assert.equal(normaliseEmail("a@" + "b".repeat(250) + ".com"), null);
});

test("normaliseLang", () => {
  assert.equal(normaliseLang("PL"), "pl");
  assert.equal(normaliseLang("pt-BR"), "pt-BR");
  assert.equal(normaliseLang("en_US"), null);
  assert.equal(normaliseLang(1), null);
});

test("parseTags: valid, de-duplicated, refused when any is bad", () => {
  assert.deepEqual(parseTags("News, offers,,news"), ["news", "offers"]);
  assert.deepEqual(parseTags(["a_b", "c-d"]), ["a_b", "c-d"]);
  assert.deepEqual(parseTags(null), []);
  assert.deepEqual(parseTags(undefined), []);
  assert.deepEqual(parseTags(""), []);
  assert.equal(parseTags("ok, not ok"), null);
  assert.equal(parseTags([1]), null);
  assert.equal(parseTags(5), null);
  assert.equal(parseTags("x".repeat(33)), null);
  assert.equal(parseTags(Array.from({ length: 11 }, (_, i) => "t" + i)), null);
  assert.equal(parseTags(Array.from({ length: 10 }, (_, i) => "t" + i))?.length, 10);
});

test("helpers", async () => {
  assert.deepEqual(splitTags("a,b"), ["a", "b"]);
  assert.deepEqual(splitTags(""), []);
  assert.deepEqual(splitTags(null), []);
  assert.equal(escapeLike("50%_a\\b"), "50\\%\\_a\\\\b");
  assert.equal(text("  x  ", 10), "x");
  assert.equal(text(3, 10), "");
  assert.ok(isStatus("pending") && !isStatus("active") && !isStatus(1));
  assert.ok(isTrue(true) && isTrue(" Yes") && isTrue("1") && !isTrue("no") && !isTrue(1) && !isTrue(null));
  const body = (b: string) => new Request("https://example.com", { method: "POST", body: b });
  assert.deepEqual(await readJSONObject(body('{"a":1}')), { a: 1 });
  assert.equal(await readJSONObject(body("[1]")), null);
  assert.equal(await readJSONObject(body("null")), null);
  assert.equal(await readJSONObject(body("{")), null);
});
