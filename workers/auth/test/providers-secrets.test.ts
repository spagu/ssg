// GO-151, nothing secret leaks: after sign-ins, invitations, links and
// refusals with every provider, no client secret, Apple key, provider access
// or ID token, authorization code or PKCE verifier appears in an answer body,
// a header (cookies aside — they hold our own session), a log line or any
// column of any table.
import { test } from "node:test";
import assert from "node:assert/strict";
import { jwksCache } from "../functions/api/auth/_provider-oidc.ts";
import { Browser, call, useClock } from "./fakes.ts";
import { ALL, PROVIDER_ENV, SECRETS } from "./providers-fake.ts";
import { invite, tokenOf } from "./go134.ts";
import { loggedIn, signedIn } from "./seed.ts";
import { answer, fakeProviders, providerEnv, providerUser, start } from "./go151.ts";

test("secrets and provider tokens appear in no answer, log or table", async () => {
  useClock();
  jwksCache.clear();
  const env = providerEnv();
  const fake = fakeProviders();
  const logs: string[] = [];
  const saved = { log: console.log, warn: console.warn, error: console.error };
  for (const k of ["log", "warn", "error"] as const) console[k] = (...a: unknown[]) => logs.push(a.map(String).join(" "));
  const answers: string[] = [];
  const keep = async (res: Response) => {
    const headers = [...res.headers].filter(([k]) => k !== "set-cookie").map(([k, v]) => `${k}: ${v}`);
    answers.push(headers.join("\n"), await res.clone().text());
    return res;
  };
  try {
    const boss = await signedIn(env, "boss@example.com", "owner");
    const { browser } = await loggedIn(env, "linker@example.com");
    for (const [i, p] of ALL.entries()) {
      await providerUser(env, `user-${p}@example.com`, p, String(100 + i));
      const token = tokenOf((await invite(env, boss.browser, { email: `inv-${p}@example.com`, delivery: "link" })).body.link);
      const runs: [Browser, Record<string, string>, string][] = [
        [new Browser(), { purpose: "signin" }, String(100 + i)],
        [new Browser(), { purpose: "invite", token }, String(200 + i)],
        [browser, { purpose: "link" }, String(300 + i)],
        [new Browser(), { purpose: "signin" }, String(400 + i)], // not invited
      ];
      for (const [b, fields, sub] of runs) {
        const s = await keep(await call(start, env, `/api/auth/oauth/${p}/start`, { form: fields, browser: b, params: { provider: p } }));
        const back = await fake.authorize(s.headers.get("location") as string, { sub, email: `x${sub}@example.com`, verified: true });
        await keep(await answer(env, b, p, back));
      }
      fake.tweak = { forge: "none" };
      const s = await call(start, env, `/api/auth/oauth/${p}/start`, { form: { purpose: "signin" }, browser, params: { provider: p } });
      await keep(await answer(env, browser, p, await fake.authorize(s.headers.get("location") as string, { sub: "1" })));
      fake.tweak = {};
    }
  } finally {
    Object.assign(console, saved);
    fake.restore();
  }
  // Everything secret the exchanges carried.
  const secret = new Set<string>([...SECRETS, PROVIDER_ENV.AUTH_APPLE_PRIVATE_KEY.split("\n")[1]]);
  for (const c of fake.calls) {
    const f = new URLSearchParams(c.body);
    for (const k of ["code", "code_verifier", "client_secret"]) if (f.get(k)) secret.add(f.get(k) as string);
    if (c.headers.authorization) secret.add(c.headers.authorization.replace(/^Bearer /, ""));
  }
  for (const t of fake.tokens.keys()) secret.add(t);
  for (const t of fake.idTokens) secret.add(t.split(".")[2] || t);
  assert.ok(secret.size > 40, `collected ${secret.size}`);
  type Row = Record<string, unknown>;
  const db = (env as unknown as { AUTH_DB: { sqlite: { prepare(s: string): { all(): Row[] } } } }).AUTH_DB.sqlite;
  const tables = db.prepare("SELECT name FROM sqlite_master WHERE type = 'table'").all().map((r: Row) => r.name as string);
  const stored = tables.flatMap((t: string) => db.prepare(`SELECT * FROM "${t}"`).all().map((r: Row) => JSON.stringify(r)));
  const haystacks: [string, string[]][] = [["answers", answers], ["logs", logs], ["tables", stored]];
  for (const [where, texts] of haystacks) {
    for (const s of secret) {
      // An authorization code is only ever in the provider's own redirect to us
      // (and Apple's bounce, which the scan does not record).
      assert.ok(!texts.some((x) => x.includes(s)), `${where} contain ${s.slice(0, 12)}…`);
    }
  }
  assert.ok(logs.length > 0, "the forged tokens were logged — by reason only");
  assert.ok(stored.some((r) => r.includes('"kind":"signin_refused"')));
});
