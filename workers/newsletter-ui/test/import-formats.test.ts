// The import through the API with files as other tools write them: delimiter
// and encoding matrix, headerless lists, the override, dates and their
// warnings, the ambiguous day/month rule.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { PASSWORD, fakeCtx, req } from "./fakes.ts";
import { B, api, readyEnv } from "./subscriber-seed.ts";

type Env = Awaited<ReturnType<typeof readyEnv>>;
const rows = (env: Env) =>
  env.NEWSLETTER_DB.sqlite.prepare("SELECT email, status, language, tags, source_page, consent_at, created_at, confirmed_at, unsubscribed_at FROM subscribers ORDER BY email").all().map((r) => ({ ...r }));

// raw posts bytes as text/csv, as a browser upload of the file would.
const raw = (env: Env, path: string, body: Uint8Array) =>
  handleFetch(req(B + path, { method: "POST", auth: PASSWORD, body, headers: { "content-type": "text/csv" } }), env as never, fakeCtx() as never);

function utf16(text: string, bigEndian: boolean): Uint8Array {
  const out = new Uint8Array(2 + text.length * 2);
  out.set(bigEndian ? [0xfe, 0xff] : [0xff, 0xfe]);
  for (let i = 0; i < text.length; i++) {
    const c = text.charCodeAt(i);
    out[2 + i * 2 + (bigEndian ? 1 : 0)] = c & 0xff;
    out[2 + i * 2 + (bigEndian ? 0 : 1)] = c >> 8;
  }
  return out;
}

test("delimiter matrix: comma, semicolon, tab; quoted delimiters respected", async () => {
  for (const d of [",", ";", "\t"]) {
    const env = await readyEnv();
    const other = d === "," ? ";" : ",";
    // The quoted note holds more commas and semicolons than the header has delimiters.
    const note = '"a;b;c;d;e,f,g,h,i ""quoted"""';
    const csv = [`Email${d}${note}${d}Language${d}Tags`, `a@example.com${d}${d}pl${d}"x${other}y"`, `b@example.com${d}${d}${d}`].join("\r\n");
    const p = await (await api(env, "POST", "/api/import/preview", csv)).json();
    assert.equal(p.detected.delimiter, d, JSON.stringify(d));
    assert.deepEqual(p.detected.ignored, [{ header: 'a;b;c;d;e,f,g,h,i "quoted"', code: "not_recognised", reason: "ignored: not a recognised column" }]);
    assert.equal(p.counts.new, 2, JSON.stringify(d));
    await api(env, "POST", "/api/import", { csv, consentSource: "Old list" });
    assert.deepEqual(rows(env).map((r) => [r.email, r.language, r.tags]), [["a@example.com", "pl", "x,y"], ["b@example.com", null, ""]]);
  }
});

test("encodings: UTF-8 with BOM, UTF-16 LE and BE with BOM", async () => {
  const csv = "Email;Groups\r\nzoë@example.com;café\r\nłucja@example.com;news\r\n";
  for (const body of [new TextEncoder().encode("﻿" + csv), utf16("﻿" + csv, false), utf16("﻿" + csv, true)]) {
    const env = await readyEnv();
    const p = await (await raw(env, "/api/import/preview", body)).json();
    assert.equal(p.detected.delimiter, ";");
    assert.deepEqual(p.sample.new.map((e: { email: string }) => e.email), ["łucja@example.com"]);
    assert.deepEqual(p.invalid, [{ line: 2, reason: "invalid_tags", value: "café" }]);
  }
});

test("headerless single-column list imports from line 1", async () => {
  const env = await readyEnv();
  const res = await (await api(env, "POST", "/api/import?consentSource=Paper%20sheet", "one@example.com\ntwo@example.com\n")).json();
  assert.equal(res.imported, 2);
  assert.equal(res.detected.headerless, true);
  assert.equal(res.detected.columns.email, "column 1");
  assert.deepEqual(rows(env).map((r) => r.status), ["pending", "pending"]);
});

test("override mapping through JSON and the query string; bad overrides are 400", async () => {
  const env = await readyEnv();
  const csv = "Who,Where\nmap@example.com,/landing/\n";
  const missing = await api(env, "POST", "/api/import/preview", csv);
  assert.equal((await missing.json()).error, "missing_email_column");
  const p = await (await api(env, "POST", "/api/import/preview", { csv, mapping: { email: "Who", sourcePage: "where" } })).json();
  assert.deepEqual([p.detected.columns.email, p.detected.columns.sourcePage, p.counts.new], ["Who", "Where", 1]);
  const q = encodeURIComponent(JSON.stringify({ email: "Who", sourcePage: "Where" }));
  const run = await (await api(env, "POST", `/api/import?consentSource=x&mapping=${q}`, csv)).json();
  assert.equal(run.imported, 1);
  assert.equal(rows(env)[0].source_page, "/landing/");
  const cases: [string, unknown, string][] = [
    ["/api/import/preview?mapping=%7Bbroken", csv, "invalid_mapping"],
    ["/api/import/preview", { csv, mapping: { email: "Nope" } }, "invalid_mapping"],
    ["/api/import/preview", { csv, mapping: { nickname: "Who" } }, "invalid_mapping"],
    ["/api/import/preview", { csv, dateOrder: "ymd" }, "invalid_date_order"],
    ["/api/import/preview?dateOrder=x", csv, "invalid_date_order"],
  ];
  for (const [path, body, error] of cases) {
    const res = await api(env, "POST", path, body);
    assert.equal(res.status, 400, path);
    assert.equal((await res.json()).error, error);
  }
});

test("dates: sign-up date fills created_at, consent_at and confirmed_at; warnings keep the row", async () => {
  const env = await readyEnv();
  const csv = [
    "email,signup date",
    "iso@example.com,2024-03-04T05:06:07Z",
    "dot@example.com,04.03.2024",
    "bad@example.com,last tuesday",
    "fut@example.com,2999-01-01",
    "none@example.com,",
  ].join("\n");
  const out = await (await api(env, "POST", "/api/import", { csv, consentSource: "Fair", confirmed: true })).json();
  assert.equal(out.imported, 5);
  assert.equal(out.dateWarningCount, 2);
  assert.deepEqual(out.dateWarnings, [
    { line: 4, column: "signup date", value: "last tuesday", reason: "unparseable" },
    { line: 5, column: "signup date", value: "2999-01-01", reason: "future" },
  ]);
  const byEmail = Object.fromEntries(rows(env).map((r) => [r.email, r]));
  assert.equal(byEmail["iso@example.com"].created_at, "2024-03-04T05:06:07.000Z");
  assert.equal(byEmail["iso@example.com"].consent_at, "2024-03-04T05:06:07.000Z");
  assert.equal(byEmail["iso@example.com"].confirmed_at, "2024-03-04T05:06:07.000Z");
  assert.equal(byEmail["dot@example.com"].created_at, "2024-03-04T00:00:00.000Z");
  for (const e of ["bad@example.com", "fut@example.com", "none@example.com"]) {
    assert.ok(Date.now() - Date.parse(byEmail[e].created_at as string) < 60_000, e);
    assert.equal(byEmail[e].consent_at, byEmail[e].created_at);
  }
});

test("ambiguous slash dates: decided by a later row; never decided → 400 until dateOrder", async () => {
  const env = await readyEnv();
  const decided = "email,created\na@example.com,03/04/2024\nb@example.com,25/12/2023\n";
  const p = await (await api(env, "POST", "/api/import/preview", decided)).json();
  assert.deepEqual(p.detected.dateOrder, { signedUpAt: "dmy" });
  assert.equal(p.dateWarningCount, 0);

  const open = "email,created\na@example.com,03/04/2024\nb@example.com,04/05/2024\n";
  const refused = await api(env, "POST", "/api/import/preview", open);
  assert.equal(refused.status, 400);
  const body = await refused.json();
  assert.equal(body.error, "ambiguous_date_format");
  assert.deepEqual(body.columns, ["created"]);
  assert.equal(body.detected.columns.signedUpAt, "created");
  assert.equal((await api(env, "POST", "/api/import", { csv: open, consentSource: "x" })).status, 400);

  const run = await (await api(env, "POST", "/api/import?consentSource=x&dateOrder=mdy", open)).json();
  assert.equal(run.imported, 2);
  assert.deepEqual(rows(env).map((r) => r.created_at), ["2024-03-04T00:00:00.000Z", "2024-04-05T00:00:00.000Z"]);
});

test("only stored dates are decided: an undecidable bounce date does not block", async () => {
  const env = await readyEnv();
  const csv = "email,bounced_at\nx@example.com,03/04/2024\n";
  const p = await (await api(env, "POST", "/api/import/preview", csv)).json();
  assert.deepEqual([p.classCounts.toSuppress, p.dateWarningCount], [1, 0]);
});

test("locales with underscores and semicolon-separated tags", async () => {
  const env = await readyEnv();
  const csv = "email,locale,lists\nloc@example.com,pt_BR,News;Offers\n";
  await api(env, "POST", "/api/import", { csv, consentSource: "x" });
  assert.deepEqual([rows(env)[0].language, rows(env)[0].tags], ["pt-BR", "news,offers"]);
});

test("docs/IMPORT.md's worked example does what the page says", async () => {
  const env = await readyEnv();
  const csv = [
    "E-mail;First name;Status;Date subscribed;Unsubscribed at;Groups",
    'ana@example.com;Ana;Subscribed;03/04/2024;;"Weekly;Offers"',
    "ben@example.com;Ben;Opted out;25/01/2023;02/02/2024;",
    "cid@example.com;Cid;Cleaned;;;",
    "dan@example.com;Dan;;;;news",
  ].join("\n");
  const p = await (await api(env, "POST", "/api/import/preview", { csv, confirmed: true })).json();
  assert.equal(p.detected.delimiter, ";");
  assert.deepEqual(p.detected.dateOrder, { signedUpAt: "dmy", unsubscribedAt: "dmy" });
  assert.deepEqual(p.detected.ignored, [{ header: "First name", code: "name_not_stored", reason: "ignored: this worker does not store names" }]);
  assert.deepEqual(p.classCounts, { toConfirm: 2, toPending: 0, toUnsubscribed: 1, toSuppress: 1, toSuppressExisting: 0, toStrengthen: 0 });
  await api(env, "POST", "/api/import", { csv, confirmed: true, consentSource: "Shop sign-up sheet" });
  const got = rows(env).map((r) => [r.email, r.status, r.tags, r.created_at, r.unsubscribed_at]);
  assert.deepEqual(got.slice(0, 2), [
    ["ana@example.com", "confirmed", "weekly,offers", "2024-04-03T00:00:00.000Z", null],
    ["ben@example.com", "unsubscribed", "", "2023-01-25T00:00:00.000Z", "2024-02-02T00:00:00.000Z"],
  ]);
  assert.deepEqual([got[2][0], got[2][1], got[2][2]], ["dan@example.com", "confirmed", "news"]);
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT reason FROM suppressions WHERE email = 'cid@example.com'").get()?.reason, "hard");
});
