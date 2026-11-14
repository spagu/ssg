// Regression test for the security review (workers/auth/docs/SECURITY-REVIEW.md).
import { test } from "node:test";
import assert from "node:assert/strict";
import { requireAdmin } from "../src/auth.ts";
import { ensureSchema } from "../src/schema.ts";
import { PASSWORD, makeEnv, req } from "./fakes.ts";

test("SR-12: parallel wrong passwords are counted atomically; at most MAX_FAILURES are answered 401, and the right one after them is refused", async () => {
  const env = makeEnv({ NEWSLETTER_AUTH_MAX_FAILURES: "3" });
  await ensureSchema(env as never);
  // A barrier: every request reads the lock before any of them goes on — the
  // worst case of a parallel burst, made deterministic.
  const db = env.NEWSLETTER_DB as unknown as { prepare(sql: string): Record<string, unknown> };
  const real = db.prepare.bind(db);
  let waiting = 0;
  let open: () => void = () => {};
  const gate = new Promise<void>((r) => (open = r));
  db.prepare = (sql: string) => {
    const stmt = real(sql);
    if (!sql.startsWith("SELECT failures, window_start")) return stmt;
    return { bind: (...a: unknown[]) => ({ first: async () => { if (++waiting === 31) open(); await gate; return (stmt.bind as (...a: unknown[]) => { first(): unknown })(...a).first(); } }) };
  };
  const res = (await Promise.all([...Array.from({ length: 30 }, (_, i) => requireAdmin(req("/x", { auth: `wrong-${i}` }), env as never)), requireAdmin(req("/x", { auth: PASSWORD }), env as never)])) as (Response | null)[];
  const wrong = res.slice(0, 30) as Response[];
  const n = wrong.reduce<Record<number, number>>((m, r) => ((m[r.status] = (m[r.status] ?? 0) + 1), m), {});
  assert.ok((n[401] ?? 0) <= 3, JSON.stringify(n));
  assert.equal((n[401] ?? 0) + (n[429] ?? 0), 30);
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT failures FROM auth_attempts").get()!.failures, 30, "every failure counted");
  assert.equal((res[30] as Response | null)?.status, 429);
});
