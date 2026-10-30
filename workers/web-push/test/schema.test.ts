// The schema is created once per isolate, and retried after a failure. Its
// own file, so the module-level cache starts empty. Run from the repository
// root:
//   node --import ./workers/web-push/test/resolve.mjs --test 'workers/web-push/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { ensureSchema } from "../functions/api/push/_schema.ts";

test("ensureSchema retries after a failure, then runs once", async () => {
  let calls = 0;
  const db = {
    prepare: (sql: string) => sql,
    batch: async (list: string[]) => {
      calls++;
      assert.match(list[0], /CREATE TABLE IF NOT EXISTS push_subscriptions/);
      if (calls === 1) throw new Error("d1 down");
      return [];
    },
  };
  const env = { PUSH_DB: db } as never;
  await assert.rejects(ensureSchema(env), /d1 down/);
  await ensureSchema(env);
  await ensureSchema(env);
  assert.equal(calls, 2);
});
