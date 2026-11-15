// portable/ops.ts is shared with designer-ui (GO-185). editor-ui's own
// operations check one permission each, so the per-key refusal designer-ui's
// settings publication uses is exercised here, against editor-ui's names.
import { test } from "node:test";
import assert from "node:assert/strict";
import { OpError, errorResponse, requirePermission } from "../portable/ops.ts";
import type { Caller } from "../portable/gate.ts";
import { PERM } from "../src/permissions.ts";

const caller = (perms: string[]): Caller => ({ kind: "user", user: { id: "u1", email: "ada@example.com" }, perms, sessionId: "s1", expiresAt: "2026-01-01T00:00:00.000Z" });

test("requirePermission: passes with the permission; without it 403 naming it, and each key when keys are given", async () => {
  assert.equal(requirePermission(caller([PERM.read]), PERM.read), undefined);
  assert.throws(() => requirePermission(caller([]), PERM.read), (e: unknown) => e instanceof OpError && e.status === 403 && e.extra.permission === PERM.read && !("keys" in e.extra));
  let thrown: unknown;
  try {
    requirePermission(caller([PERM.read]), PERM.delete, ["a.b", "c"]);
  } catch (err) {
    thrown = err;
  }
  const res = errorResponse(thrown, "test");
  assert.equal(res.status, 403);
  assert.deepEqual(await res.json(), {
    error: "forbidden",
    permission: PERM.delete,
    keys: ["a.b", "c"],
    refused: [{ key: "a.b", reason: `needs ${PERM.delete}` }, { key: "c", reason: `needs ${PERM.delete}` }],
  });
});
