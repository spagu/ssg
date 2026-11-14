// GO-141: what a service token may carry — the intersection table, wildcards
// on both sides, and the sensitive permissions an owner must allow.
import { test } from "node:test";
import assert from "node:assert/strict";
import { can } from "../functions/api/auth/_can.ts";
import { intersect, isSensitive, tokenPermissions } from "../functions/api/auth/_service-perms.ts";

// [account, token list, expected intersection]
const TABLE: [string[], string[], string[]][] = [
  [["newsletter:*"], ["newsletter:read"], ["newsletter:read"]],
  [["newsletter:read"], ["newsletter:*"], ["newsletter:read"]],
  [["*"], ["newsletter:*"], ["newsletter:*"]],
  [["newsletter:*"], ["*"], ["newsletter:*"]],
  [["*"], ["*"], ["*"]],
  [["newsletter:read"], ["newsletter:send"], []],
  [["newsletter:*"], ["comments:read"], []],
  [["newsletter:read", "comments:*"], ["comments:moderate", "newsletter:read", "web-push:notify"], ["comments:moderate", "newsletter:read"]],
  [["newsletter:read", "newsletter:*"], ["newsletter:*"], ["newsletter:*"]],
  [[], ["newsletter:read"], []],
  [["newsletter:read"], [], []],
  [["newsletter:send"], ["newsletter:sender"], []],
];

for (const [a, b, want] of TABLE) {
  test(`intersect: ${JSON.stringify(a)} ∩ ${JSON.stringify(b)} = ${JSON.stringify(want)}`, () => {
    assert.deepEqual(intersect(a, b), want);
    assert.deepEqual(intersect(b, a), want, "symmetric");
  });
}

test("intersect never allows more than either side (every concrete permission checked)", () => {
  const universe = ["newsletter:read", "newsletter:send", "newsletter:erase", "comments:read", "auth:admin", "web-push:notify"];
  const lists = [["*"], ["newsletter:*"], ["newsletter:read"], ["comments:*", "newsletter:send"], ["auth:*"], []];
  for (const a of lists) {
    for (const b of lists) {
      const r = intersect(a, b);
      for (const p of universe) assert.equal(can(r, p), can(a, p) && can(b, p), `${p}: ${JSON.stringify(a)} ∩ ${JSON.stringify(b)} = ${JSON.stringify(r)}`);
    }
  }
});

test("isSensitive: *, auth:…, …:erase and worker wildcards; nothing else", () => {
  for (const p of ["*", "auth:admin", "auth:invite", "auth:*", "newsletter:erase", "comments:erase", "newsletter:*", "web-push:*"]) assert.ok(isSensitive(p), p);
  for (const p of ["newsletter:read", "newsletter:send", "web-push:notify", "republish-trigger:run", "comments:delete", "newsletter:eraser"]) assert.ok(!isSensitive(p), p);
});

test("tokenPermissions: no list = the account's, minus the sensitive", () => {
  assert.deepEqual(tokenPermissions(["web-push:notify", "auth:admin", "newsletter:erase", "*"], null, []), ["web-push:notify"]);
  assert.deepEqual(tokenPermissions(["newsletter:read", "newsletter:*"], null, []), ["newsletter:read"], "the wildcard goes; the read named beside it stays");
  assert.deepEqual(tokenPermissions(["*", "web-push:notify"], null, []), ["web-push:notify"], "never folded into a * that is then removed");
  assert.deepEqual(tokenPermissions(["newsletter:read", "web-push:notify"], null, []), ["newsletter:read", "web-push:notify"]);
});

test("tokenPermissions: a list narrows; a wildcard account still yields the concrete names asked for", () => {
  assert.deepEqual(tokenPermissions(["*"], ["web-push:notify"], []), ["web-push:notify"]);
  assert.deepEqual(tokenPermissions(["newsletter:*"], ["newsletter:settings", "newsletter:erase"], []), ["newsletter:settings"]);
  assert.deepEqual(tokenPermissions(["newsletter:read"], ["newsletter:send"], []), []);
});

test("tokenPermissions: allowSensitive lets exactly what it covers through", () => {
  assert.deepEqual(tokenPermissions(["newsletter:*"], ["newsletter:settings", "newsletter:erase"], ["newsletter:erase"]), ["newsletter:erase", "newsletter:settings"]);
  assert.deepEqual(tokenPermissions(["newsletter:*"], null, ["newsletter:erase"]), [], "erase does not cover the wildcard");
  assert.deepEqual(tokenPermissions(["newsletter:*"], null, ["newsletter:*"]), ["newsletter:*"]);
  assert.deepEqual(tokenPermissions(["*", "auth:admin"], null, ["*"]), ["*"]);
  assert.deepEqual(tokenPermissions(["auth:admin"], null, ["auth:invite"]), []);
});
