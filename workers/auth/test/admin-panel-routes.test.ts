// The route table of GO-138: every new admin route answers 401 without a
// session, 403 naming auth:admin to an account without it, refuses a request
// without the CSRF defence when it changes state, and succeeds with it.
// Generated from the files like GO-133's and GO-141's: a GO-138 route file
// without a line here fails the first test.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { Browser, call, makeEnv, useClock } from "./fakes.ts";
import { signedIn } from "./seed.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const ROOT = new URL("../functions/api/auth/", import.meta.url).pathname;
type Ids = { member: string; session: string };

const ROUTES: Record<string, { params?: (i: Ids) => Record<string, string>; json?: () => unknown }> = {
  "GET admin/users/index.ts": {},
  "GET admin/users/[id]/index.ts": { params: (i) => ({ id: i.member }) },
  "DELETE admin/users/[id]/index.ts": { params: (i) => ({ id: i.member }) },
  "POST admin/users/[id]/status.ts": { params: (i) => ({ id: i.member }), json: () => ({ status: "blocked" }) },
  "GET admin/sessions/index.ts": {},
  "DELETE admin/sessions/[id].ts": { params: (i) => ({ id: i.session }) },
  "POST admin/sessions/revoke-all.ts": { json: () => ({ confirm: "revoke-all" }) },
  "GET admin/events/index.ts": {},
  "GET admin/events/export.ts": {},
  "GET admin/status.ts": {},
  "GET admin/matrix.ts": {},
  "GET module/summary.ts": {},
};
const OWN = new Set(Object.keys(ROUTES).map((k) => k.split(" ")[1]));

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((n) => {
    const p = join(dir, n);
    if (statSync(p).isDirectory()) return files(p);
    return n.startsWith("_") ? [] : [relative(ROOT, p)];
  });
}
// A new file under admin/sessions/ or admin/events/ must get a line here too.
const isOwn = (f: string) => OWN.has(f) || f.startsWith("admin/sessions/") || f.startsWith("admin/events/");
const urlOf = (file: string, params: Record<string, string>) => "/api/auth/" + file.replace(/(\/index)?\.ts$/, "").replace(/\[(\w+)\]/g, (_, k) => params[k]);

async function fixture() {
  useClock();
  const env = makeEnv();
  const member = await signedIn(env, "member@example.com", ["newsletter:read"]);
  const session = (env.AUTH_DB.sqlite.prepare("SELECT id FROM sessions WHERE user_id = ?").get(member.id) as { id: string }).id;
  return { env, ids: { member: member.id, session } as Ids };
}
const holder = async (env: Env, lacking: boolean): Promise<Browser> =>
  (await signedIn(env, `${lacking ? "lacking" : "holding"}@example.com`, lacking ? ["newsletter:read"] : ["auth:admin", "newsletter:read"])).browser;

test("GO-138 route table: every handler in the GO-138 files has exactly one line", async () => {
  const found: string[] = [];
  for (const file of files(ROOT).filter(isOwn)) {
    const mod = await import(join(ROOT, file));
    for (const name of Object.keys(mod)) {
      const m = /^onRequest(Get|Post|Put|Delete|Patch)$/.exec(name);
      if (m) found.push(`${m[1].toUpperCase()} ${file}`);
    }
  }
  assert.deepEqual(found.sort(), Object.keys(ROUTES).sort());
});

for (const [key, spec] of Object.entries(ROUTES)) {
  const [method, file] = key.split(" ");
  test(`GO-138 route table: ${key} — auth:admin`, async () => {
    const mod = await import(join(ROOT, file));
    const handler = mod[`onRequest${method[0]}${method.slice(1).toLowerCase()}`];
    const { env, ids } = await fixture();
    const params = spec.params?.(ids) ?? {};
    const json = spec.json?.();
    const run = (browser?: Browser) => call(handler, env, urlOf(file, params), { method, browser, params, ...(json !== undefined ? { json } : {}) });
    const none = await run();
    assert.deepEqual([none.status, await none.json()], [401, { error: "not_authenticated" }]);
    const without = await run(await holder(env, true));
    assert.deepEqual([without.status, await without.json()], [403, { error: "forbidden", permission: "auth:admin" }]);
    const holding = await holder(env, false);
    if (method !== "GET") {
      const noHeader = await call(handler, env, urlOf(file, params), { method, browser: holding, params, csrf: false, json: json ?? {}, headers: { origin: "https://example.com" } });
      assert.deepEqual([noHeader.status, (await noHeader.json()).error], [403, "csrf_check_failed"]);
      const cross = await call(handler, env, urlOf(file, params), { method, browser: holding, params, json: json ?? {}, headers: { origin: "https://evil.example.com" } });
      assert.equal(cross.status, 403);
    }
    const ok = await run(holding);
    assert.ok(ok.status >= 200 && ok.status < 300, `${key}: ${ok.status} ${await ok.clone().text()}`);
  });
}
