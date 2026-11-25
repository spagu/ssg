// Shared fixtures for the image gate tests (#448): a campaign-ready
// environment from campaign-fixtures, and fetch replaced so no test ever
// makes a real network call.
import { call, mailEnv } from "./campaign-fixtures.ts";

export type Env = ReturnType<typeof mailEnv>;
export const IMG = "https://img.example.net";
export const SITE = "https://example.com";
export const KEY = "a".repeat(64) + ".png";
export const OWN = `${SITE}/newsletter-admin/hooks/a/${KEY}`;

// net replaces fetch: an image URL answers as listed (default 200 image/png);
// an Error is thrown. `onCall` runs before each answer.
export function net(answers: Record<string, { status?: number; type?: string } | Error> = {}, onCall: () => void = () => {}) {
  const original = globalThis.fetch;
  const calls: string[] = [];
  globalThis.fetch = (async (url: string) => {
    calls.push(String(url));
    onCall();
    const a = answers[String(url)] ?? {};
    if (a instanceof Error) throw a;
    return new Response(null, { status: a.status ?? 200, headers: { "content-type": a.type ?? "image/png" } });
  }) as typeof fetch;
  return { calls, restore: () => void (globalThis.fetch = original) };
}

export async function withNet<T>(n: ReturnType<typeof net>, run: () => Promise<T>): Promise<T> {
  try {
    return await run();
  } finally {
    n.restore();
  }
}

export const create = async (env: Env, body: string, bodyFormat = "markdown") => (await call(env, "POST", "", { subject: "S", body, bodyFormat })).body;
export const row = (env: Env, id: string) => ({ ...(env.NEWSLETTER_DB.sqlite.prepare("SELECT status, updated_at FROM campaigns WHERE id = ?").get(id) as object) });
export const schedule = (env: Env, id: string) => call(env, "POST", `/${id}/schedule`, {});
