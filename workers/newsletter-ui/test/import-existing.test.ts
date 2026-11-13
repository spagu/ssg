// An import against people already on the list: a bounce or complaint in the
// file suppresses an existing subscriber without touching their row (unless a
// suppression at least as strong is already there), and the send engine then
// leaves them out. Also: free-text sign-up sources are reported, not dropped
// silently.
import { test } from "node:test";
import assert from "node:assert/strict";
import { SOURCE_WARNING_MAX } from "../src/import-plan.ts";
import { suppress } from "../src/suppress.ts";
import { addCampaign, engineEnv, fakeMailer, pump, subId, truth } from "./engine-fixtures.ts";
import { api, readyEnv, seed } from "./subscriber-seed.ts";

type D1 = Awaited<ReturnType<typeof readyEnv>>["NEWSLETTER_DB"];
const subs = (d1: D1) => d1.sqlite.prepare("SELECT * FROM subscribers ORDER BY id").all().map((r) => ({ ...r }));
const sups = (d1: D1) =>
  d1.sqlite.prepare("SELECT email, reason, source, count, created_at, updated_at FROM suppressions ORDER BY email").all().map((r) => ({ ...r }));
const addr = (i: number) => `${subId(i)}@example.com`;

test("existing confirmed subscriber + complaint in the file: row unchanged, suppressed, never mailed", async () => {
  const env = await engineEnv(3);
  const before = subs(env.NEWSLETTER_DB);
  const csv = ["email,status", `${addr(2)},spam complaint`, `${addr(3)},cleaned`].join("\n");
  const res = await api(env, "POST", "/api/import", { csv }); // nothing mailable: no consentSource needed
  assert.equal(res.status, 200);
  const out = await res.json();
  assert.deepEqual([out.imported, out.suppressionsAdded, out.counts.existing, out.classCounts.toSuppressExisting], [0, 2, 2, 2]);
  assert.deepEqual(subs(env.NEWSLETTER_DB), before, "subscriber rows byte for byte");
  assert.deepEqual(
    sups(env.NEWSLETTER_DB).map((s) => [s.email, s.reason, s.source, s.count]),
    [[addr(2), "complaint", "import", 1], [addr(3), "hard", "import", 1]],
  );

  const id = addCampaign(env.NEWSLETTER_DB);
  const mail = fakeMailer();
  await pump(env, { open: mail.open });
  assert.deepEqual(mail.sent.map((m) => m.to), [addr(1)]);
  assert.equal(truth(env.NEWSLETTER_DB, id).sent, 1);
});

test("an existing subscriber already suppressed as strongly stays as is; a weaker suppression is raised", async () => {
  const env = await readyEnv();
  seed(env.NEWSLETTER_DB, [
    { email: "complained@example.com" },
    { email: "hard@example.com" },
    { email: "manual@example.com" },
  ]);
  await suppress(env as never, { email: "complained@example.com", reason: "complaint", source: "smtp", keepEmail: true });
  await suppress(env as never, { email: "hard@example.com", reason: "hard", source: "smtp", keepEmail: true });
  await suppress(env as never, { email: "manual@example.com", reason: "manual", source: "admin", keepEmail: true });
  const before = sups(env.NEWSLETTER_DB);
  const csv = ["email,status", "complained@example.com,bounced", "hard@example.com,bounced", "manual@example.com,complaint"].join("\n");

  const preview = await (await api(env, "POST", "/api/import/preview", { csv })).json();
  assert.deepEqual(preview.counts, { new: 0, duplicate: 0, existing: 0, suppressed: 3, invalid: 0 }, "counts keep their meaning");
  assert.equal(preview.classCounts.toSuppressExisting, 1);
  assert.deepEqual(preview.classSample.toSuppressExisting, [{ line: 4, email: "manual@example.com" }]);

  const out = await (await api(env, "POST", "/api/import", { csv })).json();
  assert.equal(out.suppressionsAdded, 1);
  const after = sups(env.NEWSLETTER_DB);
  assert.deepEqual(after.slice(0, 2), before.slice(0, 2), "equal or stronger: untouched");
  assert.deepEqual([after[2].reason, after[2].source, after[2].count], ["complaint", "import", 2]);
});

test("a new address suppressed for a weaker reason is raised to the file's bounce or complaint, and counted", async () => {
  const env = await readyEnv();
  await suppress(env as never, { email: "soft@example.com", reason: "soft", source: "smtp", keepEmail: true });
  await suppress(env as never, { email: "manual@example.com", reason: "manual", source: "admin", keepEmail: true });
  await suppress(env as never, { email: "hard@example.com", reason: "hard", source: "smtp", keepEmail: true });
  const csv = ["email,status", "soft@example.com,bounced", "manual@example.com,complaint", "hard@example.com,bounced", "hard@example.com,complaint"].join("\n");

  const preview = await (await api(env, "POST", "/api/import/preview", { csv })).json();
  assert.deepEqual(preview.counts, { new: 0, duplicate: 1, existing: 0, suppressed: 3, invalid: 0 }, "still counted as suppressed");
  assert.equal(preview.classCounts.toStrengthen, 2);
  assert.deepEqual(preview.classSample.toStrengthen, [{ line: 2, email: "soft@example.com" }, { line: 3, email: "manual@example.com" }]);

  const res = await api(env, "POST", "/api/import", { csv }); // nobody mailable: no consentSource needed
  assert.equal(res.status, 200);
  const out = await res.json();
  assert.deepEqual([out.imported, out.suppressionsAdded, out.classCounts.toStrengthen], [0, 2, 2]);
  assert.deepEqual(
    sups(env.NEWSLETTER_DB).map((s) => [s.email, s.reason, s.source]),
    [["hard@example.com", "hard", "smtp"], ["manual@example.com", "complaint", "import"], ["soft@example.com", "hard", "import"]],
  );
  assert.deepEqual(subs(env.NEWSLETTER_DB), [], "no subscriber row is written for them");
});

test("preview: toSuppressExisting counted on its own, with a sample; nothing written", async () => {
  const env = await readyEnv();
  seed(env.NEWSLETTER_DB, [{ email: "a@example.com" }, { email: "b@example.com", status: "unsubscribed" }, { email: "c@example.com" }]);
  const csv = ["email,status", "new@example.com,bounced", "a@example.com,bounced", "b@example.com,complaint", "c@example.com,subscribed"].join("\n");
  const before = [subs(env.NEWSLETTER_DB), sups(env.NEWSLETTER_DB)];
  const p = await (await api(env, "POST", "/api/import/preview", { csv })).json();
  assert.deepEqual(p.counts, { new: 1, duplicate: 0, existing: 3, suppressed: 0, invalid: 0 });
  assert.deepEqual(p.classCounts, { toConfirm: 0, toPending: 0, toUnsubscribed: 0, toSuppress: 1, toSuppressExisting: 2, toStrengthen: 0 });
  assert.deepEqual(p.classSample.toSuppressExisting, [{ line: 3, email: "a@example.com" }, { line: 4, email: "b@example.com" }]);
  assert.deepEqual(p.sample.existing.map((e: { email: string }) => e.email), ["a@example.com", "b@example.com", "c@example.com"]);
  assert.deepEqual([subs(env.NEWSLETTER_DB), sups(env.NEWSLETTER_DB)], before);
});

test("consentSource: not needed to suppress existing people; still needed when nothing would be written", async () => {
  const env = await readyEnv();
  seed(env.NEWSLETTER_DB, [{ email: "a@example.com" }]);
  assert.equal((await api(env, "POST", "/api/import", { csv: "email,status\na@example.com,subscribed" })).status, 400);
  assert.equal((await api(env, "POST", "/api/import", { csv: "email,status\na@example.com,bounced" })).status, 200);
});

test("sourceWarnings: free-text sources are counted with up to 20 samples; paths and URLs are kept", async () => {
  const env = await readyEnv();
  const rows = [
    "p1@example.com,/blog/",
    "p2@example.com,https://example.com/x/?utm=1",
    "p3@example.com,landing form",
    "p4@example.com,-",
    "p5@example.com,",
    "p6@example.com,//elsewhere.example.com/",
    "p7@example.com,http://[bad",
  ];
  const csv = ["email,source", ...rows].join("\n");
  const p = await (await api(env, "POST", "/api/import/preview", { csv })).json();
  assert.equal(p.sourceWarnings, 3);
  assert.deepEqual(p.sourceWarningSample, [
    { line: 4, value: "landing form" },
    { line: 7, value: "//elsewhere.example.com/" },
    { line: 8, value: "http://[bad" },
  ]);
  const out = await (await api(env, "POST", "/api/import", { csv, consentSource: "Old form" })).json();
  assert.equal(out.sourceWarnings, 3);
  const kept = env.NEWSLETTER_DB.sqlite.prepare("SELECT email, source_page FROM subscribers ORDER BY email").all().map((r) => [r.email, r.source_page]);
  assert.deepEqual(kept.slice(0, 3), [["p1@example.com", "/blog/"], ["p2@example.com", "/x/"], ["p3@example.com", null]]);

  const many = ["email,source", ...Array.from({ length: 25 }, (_, i) => `m${i}@example.com,signup box ${"x".repeat(i ? 0 : 150)}`)].join("\n");
  const m = await (await api(env, "POST", "/api/import/preview", { csv: many })).json();
  assert.equal(m.sourceWarnings, 25);
  assert.equal(m.sourceWarningSample.length, SOURCE_WARNING_MAX);
  assert.equal(m.sourceWarningSample[0].value.length, 100, "a long value is cut");
});
