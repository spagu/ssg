// Fixtures for the confirmation-mail tests: an environment ready to mail,
// pending rows as an import leaves them, a Resend stand-in, and a POST
// through the whole pipeline.
import { handleFetch } from "../src/app.ts";
import { ensureSchema } from "../src/schema.ts";
import { PASSWORD, fakeCtx, makeEnv, req } from "./fakes.ts";
import type { FakeD1 } from "./fakes.ts";

export type Env = ReturnType<typeof makeEnv>;
export const NOW = Date.parse("2026-11-13T10:00:10.000Z");

export async function confirmEnv(overrides: Record<string, unknown> = {}): Promise<Env> {
  const env = makeEnv({
    NEWSLETTER_SITE_URL: "https://example.com",
    NEWSLETTER_MAIL_FROM: "news@example.com",
    NEWSLETTER_RESEND_API_KEY: "re_test",
    ...overrides,
  });
  await ensureSchema(env as never);
  return env;
}

// add inserts a subscriber as an import leaves it (pending, never mailed).
export function add(d1: FakeD1, id: string, fields: { status?: string; language?: string | null; sentAt?: string | null; consentAt?: string } = {}) {
  d1.sqlite
    .prepare(
      `INSERT INTO subscribers (id, email, status, language, tags, consent_text, consent_at, token, created_at, updated_at, confirm_sent_at)
       VALUES (?, ?, ?, ?, '', 'import: shop customers', ?, ?, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', ?)`,
    )
    .run(id, `${id}@example.com`, fields.status ?? "pending", fields.language ?? null, fields.consentAt ?? "2026-01-01T00:00:00.000Z", `tok-${id}`.padEnd(43, "x"), fields.sentAt ?? null);
}

export const sentAt = (env: Env, id: string) =>
  (env.NEWSLETTER_DB.sqlite.prepare("SELECT confirm_sent_at FROM subscribers WHERE id = ?").get(id) as { confirm_sent_at: string | null }).confirm_sent_at;
export const used = (env: Env) => Number((env.NEWSLETTER_DB.sqlite.prepare("SELECT COALESCE(SUM(used), 0) AS n FROM send_rate").get() as { n: number }).n);

// mailbox replaces fetch with a Resend stand-in; `refuse` maps an address to
// the HTTP status the provider answers for it.
export function mailbox(refuse: Record<string, number> = {}) {
  const original = globalThis.fetch;
  const mails: { to: string; subject: string; text: string; from: string }[] = [];
  globalThis.fetch = (async (_url: string, init: RequestInit) => {
    const m = JSON.parse(String(init.body));
    const status = refuse[m.to] ?? 200;
    if (status === 200) mails.push(m);
    return new Response("{}", { status });
  }) as typeof fetch;
  return { mails, restore: () => void (globalThis.fetch = original) };
}

export async function post(env: Env, path: string, body?: unknown) {
  const init: RequestInit & { auth: string } = { method: "POST", auth: PASSWORD };
  if (body !== undefined) init.body = typeof body === "string" ? body : JSON.stringify(body);
  const res = await handleFetch(req("/newsletter-admin/api/subscribers" + path, init), env as never, fakeCtx() as never);
  return { status: res.status, body: await res.json(), headers: res.headers };
}
