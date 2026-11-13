// Shared fixtures for the GO-125 campaign tests: subscribers seeded as the
// `newsletter` template stores them, and a campaign-ready environment.
import { handleFetch } from "../src/app.ts";
import { PASSWORD, fakeCtx, fakeD1, makeEnv, req } from "./fakes.ts";

export type SubscriberSeed = [id: string, status: string, language: string | null, tags: string];

// seed inserts subscribers as the `newsletter` template would have stored them.
export function seed(d1: ReturnType<typeof fakeD1>, rows: SubscriberSeed[]) {
  const ins = d1.sqlite.prepare(
    "INSERT INTO subscribers (id, email, status, language, tags, consent_text, consent_at, token, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)",
  );
  for (const [id, status, language, tags] of rows) {
    ins.run(id, `${id}@example.com`, status, language, tags, "yes", "2026-01-01", id.padEnd(43, "x"), "2026-01-01", "2026-01-01");
  }
}

// mailEnv is makeEnv plus everything a campaign needs to be sent: a site URL,
// a sender and a (Resend) transport, with two confirmed subscribers.
export function mailEnv(overrides: Record<string, unknown> = {}) {
  const env = makeEnv({
    NEWSLETTER_SITE_URL: "https://example.com",
    NEWSLETTER_MAIL_FROM: "news@example.com",
    NEWSLETTER_MAIL_FROM_NAME: "Example News",
    NEWSLETTER_RESEND_API_KEY: "re_test",
    ...overrides,
  });
  seed(env.NEWSLETTER_DB, [
    ["s1", "confirmed", "en", "news"],
    ["s2", "confirmed", "pl", "offers"],
    ["s3", "pending", "en", "news"],
  ]);
  return env;
}

// fakeFetch replaces globalThis.fetch, recording each request's JSON body.
export function fakeFetch(answer: () => Response = () => new Response("{}", { status: 200 })) {
  const original = globalThis.fetch;
  const calls: { url: string; body: Record<string, unknown> }[] = [];
  globalThis.fetch = (async (url: string, init: RequestInit) => {
    calls.push({ url: String(url), body: JSON.parse(String(init.body)) });
    return answer();
  }) as typeof fetch;
  return { calls, restore: () => void (globalThis.fetch = original) };
}

// call sends an authenticated request to {base}/api/campaigns + path through
// the whole pipeline and returns status, parsed JSON body and headers.
export async function call(env: ReturnType<typeof makeEnv>, method: string, path: string, body?: unknown) {
  const init: RequestInit & { auth: string } = { method, auth: PASSWORD };
  if (body !== undefined) init.body = typeof body === "string" ? body : JSON.stringify(body);
  const res = await handleFetch(req("/newsletter-admin/api/campaigns" + path, init), env as never, fakeCtx() as never);
  const text = await res.text();
  return { status: res.status, body: text ? JSON.parse(text) : null, headers: res.headers };
}
