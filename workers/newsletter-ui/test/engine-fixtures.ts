// Fixtures for the send engine (GO-126) and the tickets built on it: a list
// of subscribers, a campaign ready to go, a scripted mailer, and pump() — the
// cron and the queue run until nothing is left to do.
import { planTick } from "../src/engine-plan.ts";
import { SEND_JOB } from "../src/engine-rate.ts";
import { sendBatch } from "../src/engine-send.ts";
import type { Opener } from "../src/engine-deliver.ts";
import { handleDeadLetters } from "../src/engine-status.ts";
import { handleQueue } from "../src/jobs.ts";
import type { MailMessage, MailResult } from "../src/mail/_transport.ts";
import { ensureSchema } from "../src/schema.ts";
import { fakeBatch, makeEnv } from "./fakes.ts";
import type { FakeD1 } from "./fakes.ts";

export type EngineEnv = ReturnType<typeof makeEnv> & { NEWSLETTER_QUEUE: { sent: { body: unknown; options?: unknown }[] } };

// engineEnv: every mail setting, the schema, and `n` confirmed subscribers
// sub-00001 … (language en; consent 2026-01-01).
export async function engineEnv(n = 0, overrides: Record<string, unknown> = {}): Promise<EngineEnv> {
  const env = makeEnv({
    NEWSLETTER_SITE_URL: "https://example.com",
    NEWSLETTER_MAIL_FROM: "news@example.com",
    NEWSLETTER_RESEND_API_KEY: "re_test",
    ...overrides,
  }) as EngineEnv;
  await ensureSchema(env as never);
  addSubscribers(env.NEWSLETTER_DB, n);
  return env;
}

export const subId = (i: number) => `sub-${String(i).padStart(5, "0")}`;

// addSubscribers inserts subscribers [from, from + n) as the `newsletter` template stores them.
export function addSubscribers(d1: FakeD1, n: number, from = 1, fields: { status?: string; language?: string; consentAt?: string } = {}) {
  const ins = d1.sqlite.prepare(
    `INSERT INTO subscribers (id, email, status, language, tags, consent_text, consent_at, token, created_at, updated_at)
     VALUES (?, ?, ?, ?, '', 'yes', ?, ?, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z')`,
  );
  d1.sqlite.exec("BEGIN");
  for (let i = from; i < from + n; i++) {
    const id = subId(i);
    ins.run(id, `${id}@example.com`, fields.status ?? "confirmed", fields.language ?? "en", fields.consentAt ?? "2026-01-01T00:00:00.000Z", `tok-${id}`.padEnd(43, "x"));
  }
  d1.sqlite.exec("COMMIT");
}

// addCampaign writes a campaign straight into the table (default: scheduled, due now).
export function addCampaign(d1: FakeD1, fields: Record<string, string | null> = {}): string {
  const id = fields.id ?? crypto.randomUUID();
  const row: Record<string, string | null> = {
    id,
    subject: "Hello {{language}}",
    body: "Read [more]({{site_url}}/blog/).",
    segment: '{"languages":[],"tags":[],"tagMode":"any"}',
    status: "scheduled",
    scheduled_at: "2026-01-01T00:00:00.000Z",
    created_at: "2026-01-01T00:00:00.000Z",
    updated_at: "2026-01-01T00:00:00.000Z",
    ...fields,
  };
  const cols = Object.keys(row);
  d1.sqlite.prepare(`INSERT INTO campaigns (${cols.join(", ")}) VALUES (${cols.map(() => "?").join(", ")})`).run(...cols.map((c) => row[c]));
  return id;
}

// fakeMailer records every message and session. `answer` scripts the result
// per message (by index across the whole mailer); throwing from it is a
// transport crash.
export function fakeMailer(answer: (m: MailMessage, i: number) => MailResult | Promise<MailResult> = () => ({ ok: true })) {
  const sent: MailMessage[] = [];
  const stats = { opens: 0, closes: 0, calls: 0 };
  const open: Opener = async () => {
    stats.opens++;
    return {
      ok: true,
      mailer: {
        async send(m: MailMessage) {
          const r = await answer(m, stats.calls++);
          if (r.ok) sent.push(m);
          return r;
        },
        async close() {
          stats.closes++;
        },
      },
    };
  };
  return { open, sent, stats };
}

// perPerson counts accepted messages by recipient.
export const perPerson = (sent: MailMessage[]) => {
  const m = new Map<string, number>();
  for (const s of sent) m.set(s.to, (m.get(s.to) ?? 0) + 1);
  return m;
};

export interface PumpOptions {
  open?: Opener;
  maxRounds?: number;
  advance?: () => void; // called before every round, e.g. mock.timers.tick(60_000)
  maxRetries?: number; // as in wrangler.toml; then the dead-letter consumer gets it
}

// pump runs the cron, then delivers every queued job (one per invocation, as
// wrangler.toml configures) until a round ends with an empty queue. Retried
// jobs go back on the queue, delays are ignored (pass `advance` to move a
// mocked clock); after maxRetries a job goes to the dead-letter consumer.
export async function pump(env: EngineEnv, o: PumpOptions = {}) {
  const q = env.NEWSLETTER_QUEUE;
  const handlers = { [SEND_JOB]: (job: never, e: never) => sendBatch(job, e, o.open) };
  const attempts = new Map<unknown, number>();
  const stats = { rounds: 0, delivered: 0, retried: 0, deadLettered: 0 };
  for (; stats.rounds < (o.maxRounds ?? 1000); stats.rounds++) {
    o.advance?.();
    await planTick(env as never);
    if (q.sent.length === 0) break;
    const pending = q.sent.splice(0);
    for (const { body } of pending) {
      const batch = fakeBatch([body]);
      const n = (attempts.get(body) ?? 0) + 1;
      batch.messages[0].attempts = n;
      await handleQueue(batch as never, env as never, handlers as never);
      stats.delivered++;
      if (batch.messages[0].retried) {
        if (n > (o.maxRetries ?? 5)) {
          stats.deadLettered++;
          await handleDeadLetters(fakeBatch([body], "ssg-newsletter-send-dlq") as never, env as never);
        } else {
          stats.retried++;
          attempts.set(body, n);
          q.sent.push({ body });
        }
      }
    }
  }
  return stats;
}

export const campaignRow = (d1: FakeD1, id: string) => d1.sqlite.prepare("SELECT * FROM campaigns WHERE id = ?").get(id) as Record<string, unknown>;

export const sendRows = (d1: FakeD1, id: string) =>
  d1.sqlite.prepare("SELECT * FROM campaign_sends WHERE campaign_id = ? ORDER BY subscriber_id").all(id) as Record<string, unknown>[];

// truth is what campaign_sends says, by status.
export function truth(d1: FakeD1, id: string): Record<string, number> {
  const out: Record<string, number> = {};
  for (const r of d1.sqlite.prepare("SELECT status, COUNT(*) AS n FROM campaign_sends WHERE campaign_id = ? GROUP BY status").all(id)) {
    out[r.status as string] = Number(r.n);
  }
  return out;
}

// sendJobs are the queued send jobs (bodies), oldest first.
export const sendJobs = (env: EngineEnv) => env.NEWSLETTER_QUEUE.sent.map((m) => m.body as Record<string, string>);

// started: n subscribers, one due campaign, the first cron tick run; the
// queued jobs are taken off the queue and returned (push them back to pump).
export async function started(n: number, overrides: Record<string, unknown> = {}, fields: Record<string, string | null> = {}) {
  const env = await engineEnv(n, overrides);
  const id = addCampaign(env.NEWSLETTER_DB, fields);
  await planTick(env as never);
  const jobs = sendJobs(env);
  env.NEWSLETTER_QUEUE.sent.length = 0;
  return { env, id, jobs };
}

// runJob delivers one job to the consumer with the given mailer.
export const runJob = (env: EngineEnv, job: unknown, open: unknown) => sendBatch(job as never, env as never, open as never);

export const batchState = (env: EngineEnv, id: string) =>
  env.NEWSLETTER_DB.sqlite.prepare("SELECT status FROM send_batches WHERE campaign_id = ? ORDER BY after_id").all(id).map((r) => r.status);

export const planError = (env: EngineEnv, id: string) =>
  (env.NEWSLETTER_DB.sqlite.prepare("SELECT error FROM send_plans WHERE campaign_id = ?").get(id) as { error: string | null }).error;
