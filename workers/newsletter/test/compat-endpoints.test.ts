// Backward-compatibility suite, part 1: every endpoint of this template answers
// exactly as the 1.9.7 release did — status, headers, JSON, CSV (header row and
// cells), the HTML pages byte for byte (by hash), the rows left in
// `subscribers`, and every request sent to Turnstile and to a mail provider.
// The expected values are compat-golden.json, produced by running the same
// scenarios against that release. Run from the repository root:
//   node --import ./workers/newsletter/test/resolve.mjs --test 'workers/newsletter/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { loadHandlers } from "./compat-harness.ts";
import { CHANGED, SCENARIOS, runScenarios } from "./compat-scenarios.ts";

const golden = JSON.parse(readFileSync(new URL("./compat-golden.json", import.meta.url), "utf8")).scenarios;
const handlers = await loadHandlers(new URL("../functions/api/newsletter/", import.meta.url).href);
const now = await runScenarios(handlers, SCENARIOS);
const changed = await runScenarios(handlers, CHANGED);

test("compat: the golden file covers every scenario and nothing else", () => {
  assert.deepEqual(Object.keys(golden).sort(), Object.keys(SCENARIOS).sort());
});

for (const name of Object.keys(SCENARIOS)) {
  test(`compat: ${name} — answers and stored rows as in 1.9.7`, () => {
    const want = golden[name];
    const got = now[name];
    assert.equal(got.calls.length, want.calls.length);
    want.calls.forEach((w: { step: string }, i: number) => assert.deepEqual(got.calls[i], w, `step "${w.step}"`));
    assert.deepEqual(got.rows, want.rows);
  });
}

test("compat: the CSV export keeps its header row and cell rules", () => {
  const csv = (now["admin endpoint"].calls as { step: string; body: string }[]).find((c) => c.step === "CSV all")?.body ?? "";
  const [header, ...lines] = csv.split("\r\n");
  assert.equal(header, "﻿email,status,language,tags,source_page,consent_text,consent_at,created_at,confirmed_at,unsubscribed_at,unsubscribe_url");
  assert.match(csv, /,'=1\+1,/); // a formula is defused with an apostrophe
  assert.match(csv, /"He said ""yes"",\nthen left"/); // RFC 4180 quoting
  assert.equal(lines.at(-1), ""); // CRLF-terminated
});

test("compat: a Resend-only or MailChannels-only install sends the confirmation byte for byte as before", () => {
  const sent = (name: string, step: string) =>
    (now[name].calls as { step: string; fetches: { url: string; body: string; headers: [string, string][] }[] }[]).find((c) => c.step === step)?.fetches[1];
  const resend = sent("double opt-in via Resend", "new → pending, mail");
  assert.equal(resend?.url, "https://api.resend.com/emails");
  assert.deepEqual(resend?.headers, [["authorization", "Bearer re_test"], ["content-type", "application/json"]]);
  assert.equal(
    resend?.body,
    '{"from":"Example News <news@example.com>","to":"reader@example.com","subject":"Confirm your newsletter subscription",' +
      `"text":${JSON.stringify(JSON.parse(resend?.body ?? "{}").text)}}`,
  );
  const mc = sent("double opt-in via MailChannels", "with sender name");
  assert.equal(mc?.url, "https://api.mailchannels.net/tx/v1/send");
  assert.deepEqual(mc?.headers, [["x-api-key", "mc_test"], ["content-type", "application/json"]]);
  assert.match(mc?.body ?? "", /^\{"personalizations":\[\{"to":\[\{"email":"named@example\.com"\}\]\}\],"from":\{"email":"news@example\.com","name":"Example News"\},"subject":"[^"]+","content":\[\{"type":"text\/plain","value":"[^"]+"\}\]\}$/);
});

// The two deliberate changes of 1.9.8 (CHANGELOG "Changed"). On 1.9.7 both of
// these sign-ups were confirmed at once: an SMTP setting meant nothing then.
test("changed on purpose: a sender plus an SMTP host is double opt-in through SMTP", () => {
  const o = changed["SMTP host with a sender"];
  assert.equal((o.calls[0] as { body: string }).body, '{"ok":true,"confirm":true}');
  assert.equal(o.rows[0].status, "pending");
  assert.equal(o.rows[0].confirmed_at, null);
});

test("changed on purpose: mail settings that can never send answer not_configured (and store nothing)", () => {
  const o = changed["SMTP on port 25 with a sender"];
  const call = o.calls[0] as { status: number; body: string; fetches: unknown[] };
  assert.equal(call.status, 503);
  assert.equal(call.body, '{"error":"not_configured"}');
  assert.deepEqual(call.fetches, []);
  assert.deepEqual(o.rows, []);
  assert.match(o.logs.join("\n"), /sign-up refused.*port 25/);
});
