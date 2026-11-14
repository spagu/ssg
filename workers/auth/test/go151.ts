// Helpers for the GO-151 tests: a worker with every login provider on, the
// start → provider → callback round trip, and accounts with linked identities.
import { onRequestPost as start } from "../functions/api/auth/oauth/[provider]/start.ts";
import { onRequestGet as callbackGet, onRequestPost as callbackPost } from "../functions/api/auth/oauth/[provider]/callback.ts";
import { identityInsert } from "../functions/api/auth/_provider-identities.ts";
import { ensureSchema } from "../functions/api/auth/_schema.ts";
import { createUser } from "../functions/api/auth/_users.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { Browser, T0, call, makeEnv } from "./fakes.ts";
import { FakeProviders, PROVIDER_ENV } from "./providers-fake.ts";
import type { Person, Pid } from "./providers-fake.ts";

export { start, callbackGet, callbackPost };

export const providerEnv = (over: Partial<Env> = {}) => makeEnv({ ...PROVIDER_ENV, ...over } as Partial<Env>);

// fakeProviders installs the fakes; call .restore() in a finally / after.
export const fakeProviders = () => new FakeProviders().install();

export async function startAt(env: Env, browser: Browser, provider: string, fields: Record<string, string> = {}): Promise<Response> {
  return call(start, env, `/api/auth/oauth/${provider}/start`, { form: { purpose: "signin", ...fields }, browser, params: { provider } });
}

// answer delivers the provider's answer to the callback as the browser would
// (Apple: its cross-site form POST, bounced to a GET).
export async function answer(env: Env, browser: Browser, provider: string, back: { path: string; form?: Record<string, string> }): Promise<Response> {
  let path = back.path;
  if (back.form) {
    const bounce = await call(callbackPost, env, path, { form: back.form, csrf: false, headers: { origin: "https://appleid.apple.com", "sec-fetch-site": "cross-site" }, params: { provider } });
    if (bounce.status !== 303) return bounce;
    path = bounce.headers.get("location") as string;
  }
  return call(callbackGet, env, path, { browser, csrf: false, params: { provider } });
}

// roundTrip: start, consent as `person`, deliver the answer.
export async function roundTrip(env: Env, fake: FakeProviders, browser: Browser, provider: Pid, person: Person, fields: Record<string, string> = {}) {
  const started = await startAt(env, browser, provider, fields);
  const back = await fake.authorize(started.headers.get("location") as string, person);
  const res = await answer(env, browser, provider, back);
  return { started, back, res, location: res.headers.get("location") ?? "" };
}

// providerUser: an account that signs in only through `provider` as `sub`.
export async function providerUser(env: Env, email: string, provider: string, sub: string): Promise<string> {
  await ensureSchema(env);
  const id = (await createUser(env, { email, name: "Pat", passwordHash: null }, T0)) as string;
  await identityInsert(env, id, provider, { subject: sub, email, emailVerified: true, name: "" }, T0).run();
  return id;
}

export const count = (env: Env, sql: string): number =>
  ((env.AUTH_DB as unknown as { sqlite: { prepare(s: string): { get(): unknown } } }).sqlite.prepare(sql).get() as { n: number }).n;

export const events = (env: Env, kind?: string): { kind: string; user_id: string | null; method: string | null; detail: Record<string, unknown> }[] =>
  (env.AUTH_DB as unknown as { sqlite: { prepare(s: string): { all(...a: unknown[]): Record<string, unknown>[] } } }).sqlite
    .prepare(`SELECT kind, user_id, method, detail FROM auth_events ${kind ? "WHERE kind = ?" : ""} ORDER BY at, rowid`)
    .all(...(kind ? [kind] : []))
    .map((r) => ({ kind: r.kind as string, user_id: r.user_id as string | null, method: r.method as string | null, detail: JSON.parse(r.detail as string) }));

export const errorOf = (location: string): string | null => new URL(location, "https://example.com").searchParams.get("error");

// before runs `fn` once, just before the first statement matching `re` executes.
export function before(env: Env, re: RegExp, fn: () => void) {
  const db = env.AUTH_DB as unknown as { prepare(sql: string): unknown };
  const real = db.prepare.bind(db);
  let done = false;
  db.prepare = (sql: string) => {
    const stmt = real(sql) as Record<string, unknown>;
    if (done || !re.test(sql)) return stmt;
    const wrap = (s: Record<string, unknown>): Record<string, unknown> => ({
      ...s,
      bind: (...a: unknown[]) => wrap((s.bind as (...a: unknown[]) => Record<string, unknown>)(...a)),
      ...Object.fromEntries(["first", "run", "all"].map((m) => [m, (...a: unknown[]) => ((done = true), fn(), (s[m] as (...a: unknown[]) => unknown)(...a))])),
    });
    return wrap(stmt);
  };
}
