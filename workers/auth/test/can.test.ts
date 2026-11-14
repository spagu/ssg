// Permission names and the check (GO-133): can, validPermission, and the
// escalation helpers built on them.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { can, validPermission } from "../functions/api/auth/_can.ts";
import { beyond, covers, parsePermissions } from "../functions/api/auth/_perms.ts";

test("can: exact, wildcard, none, empty, case, bypass attempts", () => {
  const table: [string[], string, boolean, string][] = [
    [["newsletter:send"], "newsletter:send", true, "exact"],
    [["newsletter:read"], "newsletter:send", false, "another action"],
    [["newsletter:*"], "newsletter:erase", true, "worker wildcard"],
    [["*"], "comments:moderate", true, "everything"],
    [["comments:*"], "newsletter:send", false, "another worker's wildcard"],
    [[], "newsletter:read", false, "no permissions"],
    [["newsletter:read"], "", false, "empty requirement"],
    [[""], "newsletter:read", false, "empty permission"],
    [["Newsletter:Send"], "newsletter:send", false, "held upper case"],
    [["newsletter:send"], "Newsletter:Send", false, "required upper case"],
    [["newsletter:*x"], "newsletter:send", false, "star not at the end"],
    [["newsletter:*x"], "newsletter:*x", false, "malformed requirement"],
    [["*:erase"], "newsletter:erase", false, "star as the worker"],
    [["*:*"], "newsletter:erase", false, "star star"],
    [["news*"], "newsletter:read", false, "prefix star"],
    [["newsletter:"], "newsletter:read", false, "empty action"],
    [[":read"], "newsletter:read", false, "empty worker"],
    [[" newsletter:read"], "newsletter:read", false, "leading space"],
    [["newsletter:read "], "newsletter:read", false, "trailing space"],
    [["newsletter:read"], "newsletter:read ", false, "required with a space"],
    [["newsletter :read"], "newsletter :read", false, "space inside"],
    [["newsletter:send"], "newsletter:*", false, "a wildcard is never a requirement"],
    [["*"], "*", false, "* is never a requirement"],
    [["newsletter"], "newsletter", false, "no action"],
    [["newsletter:send:x"], "newsletter:send:x", false, "three parts"],
    [["newsletter-ui:read"], "newsletter:read", false, "a longer worker name is another worker"],
    [["newsletter:*"], "newsletter-ui:read", false, "worker wildcard does not reach a similar name"],
  ];
  for (const [perms, required, want, why] of table) assert.equal(can(perms, required), want, why);
  assert.equal(can("newsletter:send" as never, "newsletter:send"), false, "not an array");
  assert.equal(can(null as never, "newsletter:send"), false);
});

test("can: _can.ts imports nothing (GO-135 copies it byte for byte)", () => {
  const src = readFileSync(new URL("../functions/api/auth/_can.ts", import.meta.url), "utf8");
  assert.doesNotMatch(src, /^\s*import\b/m);
  assert.ok(src.split("\n").length < 40, "tiny");
});

test("validPermission: names and trailing wildcards only", () => {
  for (const ok of ["*", "newsletter:*", "newsletter:send", "web-push:notify", "a:b", "a1:b2", "x:" + "a".repeat(62)]) assert.ok(validPermission(ok), ok);
  for (const bad of ["", " ", "x:" + "a".repeat(63), "*:erase", "*:*", "newsletter:*x", "news*", "newsletter", "Newsletter:send", "newsletter:Send", "1x:send", "x:1send", "-x:a", "x:-a", "a:b:c", "a :b", "a: b", "a:b\n", 7, null, undefined, ["a:b"]]) {
    assert.equal(validPermission(bad), false, JSON.stringify(bad));
  }
});

test("covers and beyond: what an administrator may hand out", () => {
  assert.ok(covers(["*"], "*"));
  assert.ok(covers(["*"], "newsletter:*"));
  assert.ok(!covers(["newsletter:*"], "*"));
  assert.ok(covers(["newsletter:*"], "newsletter:*"));
  assert.ok(covers(["newsletter:*"], "newsletter:erase"));
  assert.ok(!covers(["newsletter:read", "newsletter:send"], "newsletter:*"), "a list never covers the wildcard");
  assert.ok(covers(["newsletter:send"], "newsletter:send"));
  assert.ok(!covers(["newsletter:send"], "newsletter:erase"));
  assert.deepEqual(beyond(["auth:admin", "newsletter:read"], ["newsletter:read", "newsletter:erase", "*"]), ["newsletter:erase", "*"]);
  assert.deepEqual(beyond(["*"], ["*", "x:y"]), []);
});

test("parsePermissions: sorted, de-duplicated, every entry valid, bounded", () => {
  assert.deepEqual(parsePermissions(["b:x", "a:y", "b:x", "*"]), { ok: true, perms: ["*", "a:y", "b:x"] });
  assert.deepEqual(parsePermissions([]), { ok: true, perms: [] });
  assert.equal(parsePermissions("a:b").ok, false);
  const bad = parsePermissions(["a:b", "*:erase"]);
  assert.ok(!bad.ok && bad.detail.includes('"*:erase"'));
  const many = parsePermissions(Array.from({ length: 65 }, (_, i) => `w:a${i}`));
  assert.ok(!many.ok && /at most 64/.test(many.detail));
});
