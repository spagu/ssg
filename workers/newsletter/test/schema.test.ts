// ensureSchema (_schema.ts): runs once per isolate, and a failure is not
// cached. Its own file, so the per-process cache starts empty here.
// Run from the repository root:
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { ensureSchema } from "../functions/api/newsletter/_schema.ts";

test("ensureSchema: a failed batch is retried by the next call, a good one runs once", async () => {
  let batches = 0;
  let fail = true;
  const env = {
    NEWSLETTER_DB: {
      prepare: (sql: string) => sql,
      batch: async (statements: string[]) => {
        batches++;
        assert.match(statements[0], /CREATE TABLE IF NOT EXISTS subscribers/);
        if (fail) throw new Error("d1 down");
        return [];
      },
    },
  } as never;
  await assert.rejects(ensureSchema(env), /d1 down/);
  fail = false;
  await ensureSchema(env);
  await ensureSchema(env);
  assert.equal(batches, 2);
});
