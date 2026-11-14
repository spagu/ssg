// The route table of GO-141: who may call each new route. Generated from the
// files, like GO-133's and GO-134's: a GO-141 route file without a line here
// fails the first test.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { Browser, call, makeEnv, useClock } from "./fakes.ts";
import { signedIn } from "./seed.ts";
import { account, secretFor } from "./service.ts";
import { parseSecret } from "../functions/api/auth/_service-tokens.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const ROOT = new URL("../functions/api/auth/", import.meta.url).pathname;
type Ids = { account: string; token: string };
type Who = "machine" | "auth:admin";

const ROUTES: Record<string, { who: Who; params?: (i: Ids) => Record<string, string>; json?: () => unknown }> = {
  "GET admin/service-accounts/index.ts": { who: "auth:admin" },
  "POST admin/service-accounts/index.ts": { who: "auth:admin", json: () => ({ name: "new-robot" }) },
  "GET admin/service-accounts/[id]/index.ts": { who: "auth:admin", params: (i) => ({ id: i.account }) },
  "PUT admin/service-accounts/[id]/index.ts": { who: "auth:admin", params: (i) => ({ id: i.account }), json: () => ({ description: "d" }) },
  "DELETE admin/service-accounts/[id]/index.ts": { who: "auth:admin", params: (i) => ({ id: i.account }) },
  "GET admin/service-accounts/[id]/tokens/index.ts": { who: "auth:admin", params: (i) => ({ id: i.account }) },
  "POST admin/service-accounts/[id]/tokens/index.ts": { who: "auth:admin", params: (i) => ({ id: i.account }), json: () => ({}) },
  "DELETE admin/service-accounts/[id]/tokens/[tokenId].ts": { who: "auth:admin", params: (i) => ({ id: i.account, tokenId: i.token }) },
  // The exchange: a machine with a secret, no session and no CSRF header (token.ts says why).
  "POST token.ts": { who: "machine" },
};

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((n) => {
    const p = join(dir, n);
    if (statSync(p).isDirectory()) return files(p);
    return n.startsWith("_") ? [] : [relative(ROOT, p)];
  });
}
const OWN = (f: string) => f === "token.ts" || f.startsWith("admin/service-accounts/");
const urlOf = (file: string, params: Record<string, string>) =>
  "/api/auth/" + file.replace(/(\/index)?\.ts$/, "").replace(/\[(\w+)\]/g, (_, k) => params[k]);

async function fixture() {
  useClock();
  const env = makeEnv();
  const owner = await signedIn(env, "owner@example.com", "owner");
  const id = await account(env, owner.browser, []);
  const secret = await secretFor(env, owner.browser, id);
  return { env, ids: { account: id, token: (parseSecret(secret) as { id: string }).id } as Ids, secret };
}
const holder = async (env: Env, lacking: boolean): Promise<Browser> =>
  (await signedIn(env, `${lacking ? "lacking" : "holding"}@example.com`, lacking ? ["newsletter:read"] : ["auth:admin"])).browser;

test("GO-141 route table: every handler in the GO-141 files has exactly one line", async () => {
  const found: string[] = [];
  for (const file of files(ROOT).filter(OWN)) {
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
  test(`GO-141 route table: ${key} — ${spec.who}`, async () => {
    const mod = await import(join(ROOT, file));
    const handler = mod[`onRequest${method[0]}${method.slice(1).toLowerCase()}`];
    const { env, ids, secret } = await fixture();
    if (spec.who === "machine") {
      const none = await call(handler, env, urlOf(file, {}), { method, csrf: false });
      assert.equal(none.status, 401);
      const cross = await call(handler, env, urlOf(file, {}), { method, csrf: false, headers: { authorization: `Bearer ${secret}`, origin: "https://evil.example.com" } });
      assert.equal(cross.status, 200, "no CSRF check: the secret is not ambient");
      return;
    }
    const params = spec.params?.(ids) ?? {};
    const json = spec.json?.();
    const run = (browser?: Browser, extra = {}) => call(handler, env, urlOf(file, params), { method, browser, params, ...(json !== undefined ? { json } : {}), ...extra });
    const none = await run();
    assert.deepEqual([none.status, await none.json()], [401, { error: "not_authenticated" }]);
    const without = await run(await holder(env, true));
    assert.deepEqual([without.status, await without.json()], [403, { error: "forbidden", permission: "auth:admin" }]);
    const holding = await holder(env, false);
    if (method !== "GET") {
      const noHeader = await call(handler, env, urlOf(file, params), { method, browser: holding, params, csrf: false, json: json ?? {}, headers: { origin: "https://example.com" } });
      assert.deepEqual([noHeader.status, (await noHeader.json()).error], [403, "csrf_check_failed"], `${key} without x-ssg-auth`);
      const cross = await call(handler, env, urlOf(file, params), { method, browser: holding, params, json: json ?? {}, headers: { origin: "https://evil.example.com" } });
      assert.equal(cross.status, 403, `${key} from another origin`);
    }
    const ok = await run(holding);
    assert.ok(ok.status >= 200 && ok.status < 300, `${key}: ${ok.status} ${await ok.clone().text()}`);
  });
}
