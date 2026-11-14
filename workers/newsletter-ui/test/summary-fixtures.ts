// A database for the module summary's tests (GO-148): one of everything the
// summary counts, each row carrying recognisable personal data the summary
// must never repeat, next to rows it must NOT count.
import type { DatabaseSync } from "node:sqlite";
import { ensureSchema } from "../src/schema.ts";
import { fakeD1, makeEnv, withSubscribers } from "./fakes.ts";

// PII: what must not appear in a serialized summary (compared lower-case).
export const PII = ["pii-", "piimarker", "pii-subject", "pii-error", "@", "/pii-page/"];

// A configuration with nothing wrong: transport, sender, site URL, queue.
export const CONFIGURED = {
  NEWSLETTER_RESEND_API_KEY: "re_test_key",
  NEWSLETTER_MAIL_FROM: "news@example.com",
  NEWSLETTER_SITE_URL: "https://example.com",
};

const ago = (days: number) => new Date(Date.now() - days * 86_400_000).toISOString();

// What the seeded database holds, as the summary should count it.
export const EXPECTED = { confirmed: 2, pending: 3, neverMailed: 1, inProgress: 3, pausedByEngine: 1, uncertain: 3, deadLetters: 2, suppressed: 2 };

// seedSummary fills a database whose schema is in place.
export function seedSummary(sqlite: DatabaseSync): void {
  const person = sqlite.prepare(
    `INSERT INTO subscribers (id, email, status, language, source_page, tags, consent_text, consent_at, token,
       created_at, updated_at, confirmed_at, unsubscribed_at, confirm_sent_at, ip_hash, ua_hash)
     VALUES (?, ?, ?, 'de', '/pii-page/', 'PII-TAG', 'PII-CONSENT Alice Piimarker', ?, ?, ?, ?, NULL, ?, ?, 'PII-IPHASH', 'PII-UAHASH')`,
  );
  const people: [string, string, number, string | null, string | null][] = [
    ["s1", "confirmed", 0, null, null],
    ["s2", "confirmed", 2, null, null],
    ["s3", "pending", 0, null, null], // never mailed: counted
    ["s4", "pending", 1, null, ago(1)], // mailed: not counted
    ["s5", "pending", 0, null, null], // suppressed: not counted
    ["s6", "unsubscribed", 40, ago(3), null], // outside the 30-day series
  ];
  for (const [id, status, days, unsubscribedAt, sentAt] of people) {
    person.run(id, `pii-${id}@example.com`, status, ago(days), `PII-TOKEN-${id}`, ago(days), ago(days), unsubscribedAt, sentAt);
  }
  const suppression = sqlite.prepare("INSERT INTO suppressions (email_hash, email, reason, created_at, updated_at) VALUES (?, ?, 'hard', ?, ?)");
  suppression.run("hash-s5", "pii-s5@example.com", ago(1), ago(1));
  suppression.run("hash-erased", null, ago(1), ago(1));

  const campaign = sqlite.prepare("INSERT INTO campaigns (id, subject, status, started_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)");
  const plan = sqlite.prepare("INSERT INTO send_plans (campaign_id, error, created_at, updated_at) VALUES (?, ?, ?, ?)");
  const send = sqlite.prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, claimed_at) VALUES (?, ?, ?, ?)");
  for (const [id, status, error] of [
    ["c1", "paused", "render failed for pii-err@example.com (PII-ERROR)"], // paused by the engine
    ["c2", "paused", null], // paused by a person
    ["c3", "sending", null],
    ["c4", "sent", null],
  ] as const) {
    campaign.run(id, `PII-SUBJECT ${id}`, status, ago(1), ago(2), ago(1));
    plan.run(id, error, ago(1), ago(1));
  }
  send.run("c3", "s1", "uncertain", ago(0));
  send.run("c3", "s2", "sent", ago(0));
  send.run("c4", "s1", "uncertain", ago(1));
  send.run("c4", "s2", "uncertain", ago(1));
  const dead = sqlite.prepare("INSERT INTO send_dead_letters (id, campaign_id, job, attempts, received_at) VALUES (?, 'c3', ?, 5, ?)");
  dead.run("d1", JSON.stringify({ to: "pii-dead@example.com" }), ago(0));
  dead.run("d2", "{}", ago(0));
}

// seededEnv: a configured worker on a seeded database (sqlite: a file to share).
export async function seededEnv(overrides: Record<string, unknown> = {}, sqlite?: DatabaseSync) {
  const env = makeEnv({ NEWSLETTER_DB: withSubscribers(fakeD1(sqlite)), ...CONFIGURED, ...overrides });
  await ensureSchema(env as never);
  seedSummary(env.NEWSLETTER_DB.sqlite);
  return env;
}
