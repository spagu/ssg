// The suppression helpers later tickets build on.
import { test } from "node:test";
import assert from "node:assert/strict";
import { sha256hex } from "../src/lib.ts";
import {
  NOT_SUPPRESSED_SQL, dropHashOnly, findSuppression, hashOnlyKeys, isSuppressed, suppress, suppressedKeys, suppressionApplies, suppressionKey, unsuppress,
} from "../src/suppress.ts";
import { readyEnv, seed } from "./subscriber-seed.ts";

test("suppressionKey is the unsalted hash of the trimmed lower-case address", async () => {
  assert.equal(await suppressionKey(" Ann@Example.com "), await sha256hex("ann@example.com"));
});

test("suppress / isSuppressed / findSuppression / unsuppress", async () => {
  const env = await readyEnv({ NEWSLETTER_HASH_SALT: "salt-does-not-matter" });
  assert.equal(await isSuppressed(env as never, "ann@example.com"), false);
  await suppress(env as never, { email: "Ann@example.com", reason: "soft", source: "smtp", keepEmail: true });
  await suppress(env as never, { email: "ann@example.com", reason: "soft", source: "smtp", keepEmail: true });
  const s = await findSuppression(env as never, "ANN@example.com");
  assert.equal(s?.email, "ann@example.com");
  assert.equal(s?.count, 2);
  assert.equal(s?.reason, "soft");
  // A later hash-only suppression clears the kept address.
  await suppress(env as never, { email: "ann@example.com", reason: "manual", source: "erasure", keepEmail: false });
  const after = await findSuppression(env as never, "ann@example.com");
  assert.equal(after?.email, null);
  assert.equal(after?.reason, "manual");
  assert.equal(await isSuppressed(env as never, "ann@example.com"), true);
  assert.equal(await unsuppress(env as never, (after as { emailHash: string }).emailHash), true);
  assert.equal(await unsuppress(env as never, "nope"), false);
  assert.equal(await isSuppressed(env as never, "ann@example.com"), false);
});

test("set exclusion: SQL for kept addresses, dropHashOnly for erased ones", async () => {
  const env = await readyEnv();
  const d1 = env.NEWSLETTER_DB;
  seed(d1, [{ email: "a@example.com" }, { email: "b@example.com" }, { email: "c@example.com" }]);
  const query = async () =>
    (await d1.prepare(`SELECT s.email FROM subscribers s WHERE ${NOT_SUPPRESSED_SQL("s")} ORDER BY s.email`).all()).results as { email: string }[];
  // No hash-only rows: dropHashOnly hashes nothing and returns the same array.
  const all = await query();
  assert.equal(await dropHashOnly(env as never, all), all);

  await suppress(env as never, { email: "a@example.com", reason: "hard", source: "smtp", keepEmail: true });
  await suppress(env as never, { email: "b@example.com", reason: "manual", source: "erasure", keepEmail: false });
  const sqlOnly = await query();
  assert.deepEqual(sqlOnly.map((r) => r.email), ["b@example.com", "c@example.com"]);
  assert.deepEqual((await dropHashOnly(env as never, sqlOnly)).map((r) => r.email), ["c@example.com"]);
  // The index makes the SQL step a lookup, not a scan of suppressions.
  const plan = d1.sqlite.prepare(`EXPLAIN QUERY PLAN SELECT 1 FROM subscribers s WHERE ${NOT_SUPPRESSED_SQL("s")}`).all();
  assert.match(JSON.stringify(plan), /idx_suppressions_email/);
});

test("suppressedKeys looks up many keys in chunks", async () => {
  const env = await readyEnv();
  await suppress(env as never, { email: "x@example.com", reason: "hard", source: "smtp", keepEmail: true });
  const keys = await Promise.all(["x@example.com", "y@example.com", "z@example.com"].map(suppressionKey));
  const found = await suppressedKeys(env as never, keys, 2);
  assert.deepEqual([...found], [keys[0]]);
  assert.equal(env.NEWSLETTER_DB.calls.filter((s) => s.includes("json_each")).length, 2);
});

test("suppressionApplies: fresh consent lifts only a hash-only manual suppression", () => {
  const at = "2026-06-01T00:00:00.000Z";
  const later = "2026-07-01T00:00:00.000Z";
  const earlier = "2026-05-01T00:00:00.000Z";
  assert.equal(suppressionApplies({ reason: "manual", email: null, updatedAt: at }, later), false);
  assert.equal(suppressionApplies({ reason: "manual", email: null, updatedAt: at }, earlier), true);
  assert.equal(suppressionApplies({ reason: "manual", email: null, updatedAt: at }, at), true, "same instant is not newer");
  assert.equal(suppressionApplies({ reason: "manual", email: null, updatedAt: at }, null), true);
  assert.equal(suppressionApplies({ reason: "manual", email: null, updatedAt: at }, "not a date"), true, "fails closed");
  assert.equal(suppressionApplies({ reason: "manual", email: "a@example.com", updatedAt: at }, later), true);
  for (const reason of ["hard", "soft", "complaint"]) assert.equal(suppressionApplies({ reason, email: null, updatedAt: at }, later), true);
});

test("dropHashOnly with freshConsentWins keeps a person who signed up again after an erasure", async () => {
  const env = await readyEnv();
  await suppress(env as never, { email: "a@example.com", reason: "manual", source: "erasure", keepEmail: false });
  await suppress(env as never, { email: "b@example.com", reason: "complaint", source: "webhook", keepEmail: false });
  const rows = [
    { email: "a@example.com", consent_at: "2999-01-01T00:00:00.000Z" },
    { email: "b@example.com", consent_at: "2999-01-01T00:00:00.000Z" },
    { email: "c@example.com" },
  ];
  assert.deepEqual((await dropHashOnly(env as never, rows)).map((r) => r.email), ["c@example.com"], "off by default (export)");
  assert.deepEqual((await dropHashOnly(env as never, rows, true)).map((r) => r.email), ["a@example.com", "c@example.com"]);
  assert.equal((await hashOnlyKeys(env as never)).size, 2);
});

test("dropHashOnly with freshConsentWins: a row without consent_at fails closed", async () => {
  const env = await readyEnv();
  await suppress(env as never, { email: "a@example.com", reason: "manual", source: "erasure", keepEmail: false });
  assert.deepEqual(await dropHashOnly(env as never, [{ email: "a@example.com" }], true), []);
});

test("suppress merges: complaint > hard > soft = manual; a dropped address stays dropped", async () => {
  const env = await readyEnv();
  const a = "a@example.com";
  const put = (reason: "hard" | "soft" | "complaint" | "manual", source: string, keepEmail = true) => suppress(env as never, { email: a, reason, source, keepEmail });
  await put("soft", "smtp");
  await put("hard", "resend");
  await put("manual", "admin");
  assert.deepEqual([(await findSuppression(env as never, a))?.reason, (await findSuppression(env as never, a))?.source], ["hard", "resend"]);
  await put("complaint", "mailchannels");
  await put("hard", "smtp");
  await put("manual", "erasure", false);
  await put("hard", "smtp");
  const s = await findSuppression(env as never, a);
  assert.deepEqual([s?.reason, s?.source, s?.email, s?.count], ["complaint", "mailchannels", null, 7]);
});
