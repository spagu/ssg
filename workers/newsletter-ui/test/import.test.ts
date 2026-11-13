// CSV import: preview, the classification matrix, consent rules, never
// touching existing rows, limits, and 10 000 rows in a few statements.
import { test } from "node:test";
import assert from "node:assert/strict";
import { newToken, readImport } from "../src/import.ts";
import { readRows } from "../src/import-plan.ts";
import { suppress } from "../src/suppress.ts";
import { api, readyEnv, seed } from "./subscriber-seed.ts";

const CSV = [
  "﻿Email,Language,Tags,Name",
  "new1@example.com,pl,\"news,offers\",Ann",
  "NEW1@example.com,,,dup", // duplicate in file (case-insensitive)
  "bad-address,en,,x",
  "lang@example.com,english,,x",
  "tags@example.com,en,bad tag,x",
  "",
  "present@example.com,en,,x",
  "gone@example.com,en,,x",
  "erased@example.com,en,,x",
  "bounced@example.com,en,,x",
  "\"new2@example.com\",EN,,\"multi",
  "line\"",
].join("\r\n");

async function setup() {
  const env = await readyEnv();
  const ids = seed(env.NEWSLETTER_DB, [
    { email: "present@example.com", status: "pending", tags: "keep" },
    { email: "gone@example.com", status: "unsubscribed", unsubscribed_at: "2026-01-02T00:00:00.000Z" },
  ]);
  await suppress(env as never, { email: "erased@example.com", reason: "manual", source: "erasure", keepEmail: false });
  await suppress(env as never, { email: "bounced@example.com", reason: "hard", source: "smtp", keepEmail: true });
  return { env, ids };
}

const snapshot = (env: Awaited<ReturnType<typeof readyEnv>>) =>
  env.NEWSLETTER_DB.sqlite.prepare("SELECT * FROM subscribers ORDER BY email").all().map((r) => ({ ...r }));

test("preview: counts, samples and invalid lines; writes nothing", async () => {
  const { env } = await setup();
  const before = snapshot(env);
  const res = await api(env, "POST", "/api/import/preview", CSV);
  assert.equal(res.status, 200);
  const p = await res.json();
  assert.equal(p.total, 10);
  assert.deepEqual(p.counts, { new: 2, duplicate: 1, existing: 2, suppressed: 2, invalid: 3 });
  assert.deepEqual(p.sample.new, [{ line: 2, email: "new1@example.com" }, { line: 12, email: "new2@example.com" }]);
  assert.deepEqual(p.sample.duplicate, [{ line: 3, email: "new1@example.com" }]);
  assert.deepEqual(p.sample.existing.map((e: { email: string }) => e.email), ["present@example.com", "gone@example.com"]);
  assert.deepEqual(p.sample.suppressed.map((e: { email: string }) => e.email), ["erased@example.com", "bounced@example.com"]);
  assert.deepEqual(p.invalid, [
    { line: 4, reason: "invalid_email", value: "bad-address" },
    { line: 5, reason: "invalid_language", value: "english" },
    { line: 6, reason: "invalid_tags", value: "bad tag" },
  ]);
  assert.deepEqual(snapshot(env), before);
});

test("import: inserts only new rows, never updates existing or suppressed ones", async () => {
  const { env } = await setup();
  const before = snapshot(env);
  const res = await api(env, "POST", "/api/import", { csv: CSV, consentSource: "  Paper form, fair 2026  ", confirmed: true });
  assert.equal(res.status, 200);
  const out = await res.json();
  assert.equal(out.imported, 2);
  assert.equal(out.status, "confirmed");
  assert.equal(out.counts.new, 2);

  const after = snapshot(env);
  for (const old of before) assert.deepEqual(after.find((r) => r.id === old.id), old, "existing row untouched");
  const added = after.filter((r) => !before.some((b) => b.id === r.id));
  assert.deepEqual(added.map((r) => [r.email, r.language, r.tags, r.status]), [
    ["new1@example.com", "pl", "news,offers", "confirmed"],
    ["new2@example.com", "en", "", "confirmed"],
  ]);
  for (const r of added) {
    assert.equal(r.consent_text, "Paper form, fair 2026");
    assert.equal(r.confirmed_at, r.created_at);
    assert.match(r.token as string, /^[A-Za-z0-9_-]{43}$/);
    assert.match(r.id as string, /^[0-9a-f-]{36}$/);
    assert.equal(r.ip_hash, null);
  }
  assert.notEqual(added[0].token, added[1].token);
  assert.ok(!after.some((r) => r.email === "erased@example.com" || r.email === "bounced@example.com"));

  // Running it again imports nothing and changes nothing.
  const again = await (await api(env, "POST", "/api/import", { csv: CSV, consentSource: "x", confirmed: true })).json();
  assert.equal(again.imported, 0);
  assert.equal(again.counts.existing, 4);
  assert.deepEqual(snapshot(env), after);
});

test("import: pending unless confirmed:true; consentSource required", async () => {
  const { env } = await setup();
  const csv = "email\nfresh@example.com\n";
  const missing = await api(env, "POST", "/api/import", { csv, confirmed: true });
  assert.equal(missing.status, 400);
  assert.equal((await missing.json()).error, "consent_source_required");
  const blank = await api(env, "POST", "/api/import?confirmed=true", csv);
  assert.equal((await blank.json()).error, "consent_source_required");

  // text/csv with options in the query string; confirmed omitted → pending.
  const res = await (await api(env, "POST", "/api/import?consentSource=Old%20list", csv)).json();
  assert.equal(res.status, "pending");
  const r = env.NEWSLETTER_DB.sqlite.prepare("SELECT status, confirmed_at, consent_text FROM subscribers WHERE email = 'fresh@example.com'").get();
  assert.deepEqual({ ...r }, { status: "pending", confirmed_at: null, consent_text: "Old list" });
  // confirmed must be literally true in JSON.
  const notTrue = await (await api(env, "POST", "/api/import", { csv: "email\nf2@example.com", consentSource: "x", confirmed: "true" })).json();
  assert.equal(notTrue.status, "pending");
  const viaQuery = await (await api(env, "POST", "/api/import?consentSource=x&confirmed=1", "email\nf3@example.com", { "content-type": "text/plain" })).json();
  assert.equal(viaQuery.status, "confirmed");
});

test("import: request and file errors", async () => {
  const { env } = await setup();
  const cases: [unknown, Record<string, string>, number, string][] = [
    ["", {}, 400, "empty_file"],
    ["name\nx", {}, 400, "missing_email_column"],
    [{ csv: 5 }, {}, 400, "invalid_body"],
    ["{", { "content-type": "application/json" }, 400, "invalid_body"],
    ["email\na@example.com", { "content-type": "application/octet-stream" }, 415, "unsupported_media_type"],
  ];
  for (const [body, headers, status, error] of cases) {
    const res = await api(env, "POST", "/api/import/preview", body, headers);
    assert.equal(res.status, status, error);
    assert.equal((await res.json()).error, error);
  }
});

test("import: row and byte limits from NEWSLETTER_IMPORT_MAX_ROWS", async () => {
  const env = await readyEnv({ NEWSLETTER_IMPORT_MAX_ROWS: "3" });
  const rows = (n: number) => "email\n" + Array.from({ length: n }, (_, i) => `r${i}@example.com`).join("\n");
  assert.equal((await api(env, "POST", "/api/import/preview", rows(3))).status, 200);
  const many = await api(env, "POST", "/api/import/preview", rows(4));
  assert.equal(many.status, 413);
  assert.equal((await many.json()).error, "too_many_rows");
  const big = await api(env, "POST", "/api/import/preview", "email\n" + "x".repeat(3 * 1024));
  assert.equal(big.status, 413);
  assert.equal((await big.json()).error, "too_large");
  const declared = await readImport(
    new Request("https://example.com", { method: "POST", headers: { "content-length": "99999" }, body: "email" }),
    new URL("https://example.com"),
    100,
  );
  assert.equal("error" in declared && declared.error, "too_large");
});

test("readRows: missing optional columns, short rows", () => {
  const r = readRows("email\na@example.com\n", 10);
  assert.ok(!("error" in r));
  assert.deepEqual(r.rows, [{ line: 2, email: "a@example.com", language: null, tags: "" }]);
  const short = readRows("language,email,tags\npl", 10);
  assert.ok(!("error" in short) && short.invalid[0].reason === "invalid_email" && short.invalid[0].value === "");
});

test("newToken matches the newsletter template's token shape", () => {
  const seen = new Set(Array.from({ length: 50 }, newToken));
  assert.equal(seen.size, 50);
  for (const t of seen) assert.match(t, /^[A-Za-z0-9_-]{43}$/);
});

test("10 000 rows: imported in a handful of statements, one batch", async () => {
  const env = await readyEnv();
  seed(env.NEWSLETTER_DB, [{ email: "u5000@example.com", status: "unsubscribed" }]);
  await suppress(env as never, { email: "u7000@example.com", reason: "manual", source: "erasure", keepEmail: false });
  const csv = "email,language,tags\r\n" + Array.from({ length: 10_000 }, (_, i) => `u${i}@example.com,en,bulk`).join("\r\n");
  const calls = env.NEWSLETTER_DB.calls;
  const start = calls.length;
  const t0 = Date.now();
  const res = await api(env, "POST", "/api/import", { csv, consentSource: "Old list export", confirmed: true });
  assert.equal(res.status, 200);
  const out = await res.json();
  assert.equal(out.imported, 9_998);
  assert.deepEqual(out.counts, { new: 9_998, duplicate: 0, existing: 1, suppressed: 1, invalid: 0 });
  const n = env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM subscribers WHERE consent_text = 'Old list export'").get()?.n;
  assert.equal(n, 9_998);
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT status FROM subscribers WHERE email = 'u5000@example.com'").get()?.status, "unsubscribed");
  // Well under D1's per-invocation statement limit (50 on the free plan).
  const used = calls.length - start;
  assert.ok(used <= 30, `used ${used} statements`);
  assert.ok(Date.now() - t0 < 20_000);
});

test("a sign-up racing the import is skipped, not overwritten", async () => {
  const { insertRows } = await import("../src/import.ts");
  const env = await readyEnv();
  seed(env.NEWSLETTER_DB, [{ email: "race@example.com", status: "unsubscribed", tags: "mine" }]);
  const n = await insertRows(env as never, [{ line: 2, email: "race@example.com", language: null, tags: "" }], "confirmed", "x");
  assert.equal(n, 0);
  const r = env.NEWSLETTER_DB.sqlite.prepare("SELECT status, tags FROM subscribers WHERE email = 'race@example.com'").get();
  assert.deepEqual({ ...r }, { status: "unsubscribed", tags: "mine" });
  assert.equal(await insertRows(env as never, [], "pending", "x"), 0);
});
