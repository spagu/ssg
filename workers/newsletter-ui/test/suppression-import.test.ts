// GO-127: the suppression list's CSV import (for SMTP users' bounce lists).
import { test } from "node:test";
import assert from "node:assert/strict";
import { findSuppression, suppress } from "../src/suppress.ts";
import { api, readyEnv, row, seed } from "./subscriber-seed.ts";

test("import: preview writes nothing; the run writes new rows only; the matrix of rows", async () => {
  const env = await readyEnv();
  const [ann, bob] = seed(env.NEWSLETTER_DB, [{ email: "ann@example.com" }, { email: "bob@example.com" }]);
  await suppress(env as never, { email: "old@example.com", reason: "manual", source: "admin", keepEmail: true });
  const csv = [
    "Reason,Email,extra",
    ",gone@example.com,x", // default reason: hard
    "complaint,ANN@example.com,", // a complaint unsubscribes
    "manual,bob@example.com,",
    "hard,gone@example.com,", // again in the file
    "hard,old@example.com,", // already suppressed: left as it is
    "hard,not-an-address,",
    "soft,x@example.com,", // not importable
  ].join("\r\n");

  const preview = await (await api(env, "POST", "/api/suppressions/import/preview", csv)).json();
  assert.deepEqual(preview.counts, { new: 3, duplicate: 1, existing: 1, invalid: 2 });
  assert.equal(preview.total, 7);
  assert.deepEqual(preview.sample.new, [
    { line: 2, email: "gone@example.com", reason: "hard" },
    { line: 3, email: "ann@example.com", reason: "complaint" },
    { line: 4, email: "bob@example.com", reason: "manual" },
  ]);
  assert.deepEqual(preview.sample.duplicate, [{ line: 5, email: "gone@example.com", reason: "hard" }]);
  assert.deepEqual(preview.sample.existing, [{ line: 6, email: "old@example.com", reason: "hard" }]);
  assert.deepEqual(preview.invalid, [
    { line: 7, reason: "invalid_email", value: "not-an-address" },
    { line: 8, reason: "invalid_reason", value: "soft" },
  ]);
  assert.equal(await findSuppression(env as never, "gone@example.com"), null, "a preview writes nothing");

  const run = await (await api(env, "POST", "/api/suppressions/import", JSON.stringify({ csv }), { "content-type": "application/json" })).json();
  assert.equal(run.imported, 3);
  const gone = await findSuppression(env as never, "gone@example.com");
  assert.deepEqual([gone?.reason, gone?.source, gone?.email], ["hard", "import", "gone@example.com"]);
  assert.equal((await findSuppression(env as never, "old@example.com"))?.reason, "manual");
  assert.equal(row(env.NEWSLETTER_DB, ann)?.status, "unsubscribed");
  assert.equal(row(env.NEWSLETTER_DB, bob)?.status, "confirmed");
  // Nothing new the second time.
  assert.equal((await (await api(env, "POST", "/api/suppressions/import", "email\nbob@example.com\n")).json()).imported, 0);
});

test("import: refusals (empty, no email column, too many rows, too large, wrong type)", async () => {
  const env = await readyEnv({ NEWSLETTER_IMPORT_MAX_ROWS: "2" });
  const cases: [unknown, Record<string, string>, number, string][] = [
    ["", {}, 400, "empty_file"],
    ["address\na@example.com\n", {}, 400, "missing_email_column"],
    ["email\na@example.com\nb@example.com\nc@example.com\n", {}, 413, "too_many_rows"],
    ["email\n" + "a".repeat(3000), {}, 413, "too_large"],
    ["email\na@example.com", { "content-type": "application/xml" }, 415, "unsupported_media_type"],
  ];
  for (const [body, headers, status, error] of cases) {
    for (const path of ["/api/suppressions/import/preview", "/api/suppressions/import"]) {
      const res = await api(env, "POST", path, body, headers);
      assert.equal(res.status, status, `${path} ${error}`);
      assert.equal((await res.json()).error, error);
    }
  }
  // A short row (fewer fields than the header) reads the missing ones as empty.
  const short = await (await api(env, "POST", "/api/suppressions/import/preview", "reason,email\nhard\n")).json();
  assert.deepEqual(short.invalid, [{ line: 2, reason: "invalid_email", value: "" }]);
  const empty = await (await api(env, "POST", "/api/suppressions/import", "email\n")).json();
  assert.equal(empty.imported, 0);
});
