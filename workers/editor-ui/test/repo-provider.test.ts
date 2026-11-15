// repo-provider.ts: errors, limits, content sizes without decoding, author/message sanitising, ref names.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  ERROR_STATUS, HARD_LIMITS, MB, PLAN_LIMITS, RepoError, changeLimits, contentBytes, sanitizeAuthor, sanitizeMessage,
  validBranch, validCommitId,
} from "../portable/repo-provider";

test("RepoError carries a code, a status and only what we put in it", () => {
  const e = new RepoError("rate_limited", { retryAt: "2026-11-14T12:00:00.000Z" });
  assert.equal(e.message, "rate_limited");
  assert.equal(e.status, 429);
  assert.equal(e.retryAt, "2026-11-14T12:00:00.000Z");
  const c = new RepoError("conflict", { detail: "x", conflicts: [] });
  assert.equal(c.message, "conflict: x");
  assert.equal(c.status, 409);
  assert.equal(new RepoError("not_configured").status, 503);
  assert.equal(Object.keys(ERROR_STATUS).length, 12);
});

test("changeLimits: plan defaults, positive integer overrides, clamped to the hard limits", () => {
  assert.deepEqual(changeLimits("free"), PLAN_LIMITS.free);
  assert.deepEqual(changeLimits("paid"), PLAN_LIMITS.paid);
  const l = changeLimits("free", { maxFilesPerChange: "4", maxFileBytes: 0, maxChangeBytes: 10 ** 12 });
  assert.deepEqual(l, { maxFilesPerChange: 4, maxFileBytes: PLAN_LIMITS.free.maxFileBytes, maxChangeBytes: HARD_LIMITS.maxChangeBytes });
  assert.equal(changeLimits("paid", { maxFileBytes: 1.5 }).maxFileBytes, PLAN_LIMITS.paid.maxFileBytes);
  assert.equal(MB, 1048576);
});

test("contentBytes counts UTF-8 and base64 without decoding, refuses malformed input", () => {
  assert.equal(contentBytes({ text: "zażółć" }), 10);
  assert.equal(contentBytes({ base64: "" }), 0);
  assert.equal(contentBytes({ base64: "QQ==" }), 1);
  assert.equal(contentBytes({ base64: "QUI=" }), 2);
  assert.equal(contentBytes({ base64: "QUJD" }), 3);
  for (const bad of [{ base64: "QQ=" }, { base64: "Q$==" }, { base64: "QU I=" }, { base64: "Q===" }, { base64: 5 }, { text: 5 }] as never[]) {
    assert.throws(() => contentBytes(bad), (e: RepoError) => e.code === "invalid_change");
  }
});

test("sanitizeAuthor: no header or identity injection through name or e-mail", () => {
  assert.deepEqual(sanitizeAuthor({ name: "  Ada \r\n Lovelace <x@example.com> ", email: " ada@example.com " }), { name: "Ada Lovelace x@example.com", email: "ada@example.com" });
  assert.equal(sanitizeAuthor({ name: "Zoë‮", email: "z@example.com" }).name, "Zoë");
  assert.equal(sanitizeAuthor({ name: "x".repeat(300), email: "z@example.com" }).name.length, 100);
  const bad = [undefined, {}, { name: "\n\t", email: "a@example.com" }, { name: 1, email: "a@example.com" },
    { name: "A", email: "a@example.com\nCommitter: x" }, { name: "A", email: "a <b@example.com>" }, { name: "A", email: "no-at" },
    { name: "A", email: 7 }, { name: "A", email: "a@" + "e".repeat(260) }];
  for (const a of bad) assert.throws(() => sanitizeAuthor(a), (e: RepoError) => e.code === "invalid_author", JSON.stringify(a));
});

test("sanitizeMessage: CRLF → LF, control characters dropped, tabs and newlines kept, bounded", () => {
  assert.equal(sanitizeMessage(" Fix typo\r\n\r\n\tbody\u0000\u001b[31m \r"), "Fix typo\n\n\tbody[31m");
  assert.equal(sanitizeMessage("m".repeat(5000)).length, 4000);
  for (const m of ["", " \u0000 ", 3]) assert.throws(() => sanitizeMessage(m), (e: RepoError) => e.code === "invalid_message");
});

test("validBranch / validCommitId", () => {
  for (const ok of ["main", "release/1.9.10", "feature_x-2", "a".repeat(40)]) assert.equal(validBranch(ok), true, ok);
  for (const no of ["", "-x", "/x", "x/", ".x", "a..b", "a//b", "a/.b", "x.lock", "x.", "a b", "a~1", "a:b", "x".repeat(201), 5]) {
    assert.equal(validBranch(no), false, String(no));
  }
  assert.equal(validCommitId("a".repeat(40)), true);
  assert.equal(validCommitId("b".repeat(64)), true);
  assert.equal(validCommitId("A".repeat(40)), false);
  assert.equal(validCommitId("a".repeat(39)), false);
  assert.equal(validCommitId(null), false);
});
