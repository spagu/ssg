// What each kind of row does on import: unsubscribed rows stay unsubscribed
// for good, bounced and complained ones become suppressions, names are stored
// nowhere, formula-like cells are inert, and 10 000 mixed rows stay cheap.
import { test } from "node:test";
import assert from "node:assert/strict";
import { UNSUBSCRIBED_CONSENT } from "../src/import-write.ts";
import { suppress } from "../src/suppress.ts";
import { LAYOUTS } from "./import-fixtures.ts";
import { api, readyEnv, seed } from "./subscriber-seed.ts";

type Env = Awaited<ReturnType<typeof readyEnv>>;
const sub = (env: Env, email: string) => {
  const r = env.NEWSLETTER_DB.sqlite.prepare("SELECT * FROM subscribers WHERE email = ?").get(email);
  return r ? { ...r } : undefined;
};
const sup = (env: Env, email: string) => {
  const r = env.NEWSLETTER_DB.sqlite.prepare("SELECT email, reason, source FROM suppressions WHERE email = ?").get(email);
  return r ? { ...r } : undefined;
};

const MIXED = [
  "Email,Status,Signed up,Unsubscribed at",
  "act@example.com,subscribed,2024-01-01,",
  "pen@example.com,unconfirmed,2024-01-02,",
  "uns@example.com,opted out,2023-05-05,2024-02-02 10:00",
  "unsnow@example.com,unsubscribed,,",
  "bou@example.com,cleaned,,",
  "spam@example.com,Spam complaint,,",
  "odd@example.com,maybe,,",
  "implied@example.com,,,2024-03-03",
  "old@example.com,unsubscribed,,", // already a subscriber: untouched
  "blocked@example.com,bounced,,", // already suppressed: skipped
].join("\n");

async function mixedEnv() {
  const env = await readyEnv();
  seed(env.NEWSLETTER_DB, [{ email: "old@example.com", status: "confirmed" }]);
  await suppress(env as never, { email: "blocked@example.com", reason: "complaint", source: "smtp", keepEmail: true });
  return env;
}

test("preview: per-class counts and samples, invalid status with its value", async () => {
  const env = await mixedEnv();
  const p = await (await api(env, "POST", "/api/import/preview", { csv: MIXED, confirmed: true })).json();
  assert.deepEqual(p.classCounts, { toConfirm: 1, toPending: 1, toUnsubscribed: 3, toSuppress: 2, toSuppressExisting: 0, toStrengthen: 0 });
  assert.deepEqual(p.classSample.toSuppress, [{ line: 6, email: "bou@example.com" }, { line: 7, email: "spam@example.com" }]);
  assert.deepEqual(p.counts, { new: 7, duplicate: 0, existing: 1, suppressed: 1, invalid: 1 });
  assert.deepEqual(p.invalid, [{ line: 8, reason: "invalid_status", value: "maybe" }]);
  const pending = await (await api(env, "POST", "/api/import/preview", MIXED)).json();
  assert.deepEqual(pending.classCounts, { toConfirm: 0, toPending: 2, toUnsubscribed: 3, toSuppress: 2, toSuppressExisting: 0, toStrengthen: 0 });
});

test("import: each class written its own way; existing rows and suppressions untouched", async () => {
  const env = await mixedEnv();
  const before = sub(env, "old@example.com");
  const out = await (await api(env, "POST", "/api/import", { csv: MIXED, consentSource: "Old list", confirmed: true })).json();
  assert.deepEqual([out.imported, out.suppressionsAdded, out.status], [5, 2, "confirmed"]);

  const act = sub(env, "act@example.com")!;
  assert.deepEqual([act.status, act.created_at, act.confirmed_at, act.consent_text], ["confirmed", "2024-01-01T00:00:00.000Z", "2024-01-01T00:00:00.000Z", "Old list"]);
  assert.deepEqual([sub(env, "pen@example.com")!.status, sub(env, "pen@example.com")!.confirmed_at], ["pending", null]);
  const uns = sub(env, "uns@example.com")!;
  assert.deepEqual([uns.status, uns.unsubscribed_at, uns.created_at, uns.confirmed_at], ["unsubscribed", "2024-02-02T10:00:00.000Z", "2023-05-05T00:00:00.000Z", null]);
  assert.equal(sub(env, "unsnow@example.com")!.unsubscribed_at, sub(env, "unsnow@example.com")!.updated_at);
  assert.equal(sub(env, "implied@example.com")!.status, "unsubscribed");

  for (const e of ["bou@example.com", "spam@example.com", "blocked@example.com", "odd@example.com"]) assert.equal(sub(env, e), undefined, e);
  assert.deepEqual(sup(env, "bou@example.com"), { email: "bou@example.com", reason: "hard", source: "import" });
  assert.deepEqual(sup(env, "spam@example.com"), { email: "spam@example.com", reason: "complaint", source: "import" });
  assert.deepEqual(sup(env, "blocked@example.com"), { email: "blocked@example.com", reason: "complaint", source: "smtp" });
  assert.deepEqual(sub(env, "old@example.com"), before);
});

test("unsubscribed in one file, active in a later one: never reactivated", async () => {
  const env = await readyEnv();
  const first = await api(env, "POST", "/api/import", { csv: "email,status\nleft@example.com,unsubscribed\n" });
  assert.equal(first.status, 200, "no consentSource needed for unsubscribed rows only");
  assert.equal(sub(env, "left@example.com")!.consent_text, UNSUBSCRIBED_CONSENT);
  const again = await (await api(env, "POST", "/api/import", { csv: "email,status\nleft@example.com,active\n", consentSource: "x", confirmed: true })).json();
  assert.deepEqual([again.imported, again.counts.existing], [0, 1]);
  assert.equal(sub(env, "left@example.com")!.status, "unsubscribed");
  // With a consentSource, unsubscribed rows keep it as their consent text.
  await api(env, "POST", "/api/import", { csv: "email,status\nleft2@example.com,unsubscribed\n", consentSource: "Fair 2023" });
  assert.equal(sub(env, "left2@example.com")!.consent_text, "Fair 2023");
});

test("consentSource: needed when anyone mailable is added, and for an empty write as before", async () => {
  const env = await readyEnv();
  const cases: [string, number][] = [
    ["email,status\nb@example.com,bounced\n", 200],
    ["email,status\nc@example.com,complained\nu@example.com,removed\n", 200],
    ["email,status\np@example.com,pending\n", 400],
    ["email,status\nu2@example.com,removed\na@example.com,active\n", 400],
    ["email,status\nbad,active\n", 400],
  ];
  for (const [csv, status] of cases) assert.equal((await api(env, "POST", "/api/import", { csv })).status, status, csv);
  assert.equal(sup(env, "b@example.com")!.reason, "hard");
  assert.equal(sub(env, "u2@example.com"), undefined);
});

test("names are recognised and stored nowhere", async () => {
  const env = await readyEnv();
  for (const layout of LAYOUTS) await api(env, "POST", "/api/import", { csv: layout.csv, consentSource: "Old list", confirmed: true });
  const csv = "First Name,Last Name,Email,Full Name\nZbigniewo,Quixwell,zq@example.com,Zbigniewo Quixwell\n";
  const p = await (await api(env, "POST", "/api/import", { csv, consentSource: "x" })).json();
  assert.equal(p.imported, 1);
  assert.deepEqual(p.detected.ignored.map((i: { reason: string }) => i.reason), Array(3).fill("ignored: this worker does not store names"));
  const names = ["Zbigniewo", "Quixwell", "Ana", "Nowak", "Gus Grey", "Lee"];
  const tables = env.NEWSLETTER_DB.sqlite.prepare("SELECT name FROM sqlite_master WHERE type = 'table'").all() as { name: string }[];
  for (const { name } of tables) {
    for (const row of env.NEWSLETTER_DB.sqlite.prepare(`SELECT * FROM "${name}"`).all()) {
      const text = JSON.stringify(row);
      for (const n of names) assert.ok(!text.includes(n), `${n} found in ${name}`);
    }
  }
  assert.ok(sub(env, "ana@example.com") && sub(env, "gus@example.com"));
});

test("formula-like cells are inert: refused where invalid, kept as text where allowed", async () => {
  const env = await readyEnv();
  const csv = [
    "email,tags,source,status",
    '=HYPERLINK("http://example.com"),,,',
    "f1@example.com,=cmd,,",
    "f2@example.com,,/=1+1/,",
    'f3@example.com,,"@SUM(A1)",',
    "f4@example.com,,,=active",
  ].join("\n");
  const out = await (await api(env, "POST", "/api/import", { csv, consentSource: "x" })).json();
  assert.deepEqual(out.invalid.map((i: { reason: string }) => i.reason), ["invalid_email", "invalid_tags", "invalid_status"]);
  assert.equal(out.imported, 2);
  assert.equal(sub(env, "f2@example.com")!.source_page, "/=1+1/");
  assert.equal(sub(env, "f3@example.com")!.source_page, null); // not a page path: not stored
});

test("10 000 mixed rows: a handful of statements, one batch", async () => {
  const env = await readyEnv();
  const status = ["subscribed", "pending", "unsubscribed", "cleaned", "spam"];
  const csv = "Email Address;Status;Opt-in Time;Groups;First Name\r\n" +
    Array.from({ length: 10_000 }, (_, i) => `m${i}@example.com;${status[i % 5]};2024-01-${String((i % 28) + 1).padStart(2, "0")} 10:00;bulk;Name${i}`).join("\r\n");
  const start = env.NEWSLETTER_DB.calls.length;
  const t0 = Date.now();
  const out = await (await api(env, "POST", "/api/import", { csv, consentSource: "Old list", confirmed: true })).json();
  assert.deepEqual(out.classCounts, { toConfirm: 2000, toPending: 2000, toUnsubscribed: 2000, toSuppress: 4000, toSuppressExisting: 0, toStrengthen: 0 });
  assert.deepEqual([out.imported, out.suppressionsAdded], [6000, 4000]);
  const count = (sql: string) => (env.NEWSLETTER_DB.sqlite.prepare(sql).get() as { n: number }).n;
  assert.equal(count("SELECT COUNT(*) AS n FROM subscribers"), 6000);
  assert.equal(count("SELECT COUNT(*) AS n FROM suppressions WHERE source = 'import'"), 4000);
  const used = env.NEWSLETTER_DB.calls.length - start;
  assert.ok(used <= 30, `used ${used} statements`);
  assert.ok(Date.now() - t0 < 20_000);
});
