// The route table of GO-134: who may call each new route. Generated from the
// files, like GO-133's (admin-routes.test.ts): a GO-134 route file without a
// line here fails the first test.
import { test } from "node:test";
import assert from "node:assert/strict";
import { join } from "node:path";
import { Browser, call, makeEnv, useClock } from "./fakes.ts";
import { invite, tokenOf } from "./go134.ts";
import { seedUser, signedIn } from "./seed.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const ROOT = new URL("../functions/api/auth/", import.meta.url).pathname;
type Ids = { invitation: string; member: string };
type Who = "public" | "session" | "auth:invite" | "auth:admin";

// One line per method and file: who may call it, and a request that succeeds.
const ROUTES: Record<string, { who: Who; params?: (i: Ids) => Record<string, string>; json?: (i: Ids) => unknown; csrf: boolean }> = {
  "GET admin/invitations/index.ts": { who: "auth:invite", csrf: false },
  "POST admin/invitations/index.ts": { who: "auth:invite", json: () => ({ email: "x@example.com" }), csrf: true },
  "DELETE admin/invitations/[id]/index.ts": { who: "auth:invite", params: (i) => ({ id: i.invitation }), csrf: true },
  "POST admin/invitations/[id]/resend.ts": { who: "auth:invite", params: (i) => ({ id: i.invitation }), csrf: true },
  "POST admin/users/[id]/reset-link.ts": { who: "auth:admin", params: (i) => ({ id: i.member }), csrf: true },
  "GET sessions/index.ts": { who: "session", csrf: false },
  // Ending a session needs the caller's own session id: covered in own-sessions.test.ts.
  "DELETE sessions/[id].ts": { who: "session", csrf: true },
  "POST sessions/[id].ts": { who: "session", csrf: true },
  // Public: they work without a session, each guarded by its own token, limit or cookie.
  "POST invite.ts": { who: "public", csrf: true },
  "POST reset.ts": { who: "public", csrf: true },
  "POST forgot.ts": { who: "public", csrf: true },
  "GET resume.ts": { who: "public", csrf: false },
};

const files = Object.keys(ROUTES).map((k) => k.split(" ")[1]);
const urlOf = (file: string, params: Record<string, string>) =>
  "/api/auth/" + file.replace(/(\/index)?\.ts$/, "").replace(/\[(\w+)\]/g, (_, k) => params[k]);

async function fixture() {
  useClock();
  const env = makeEnv();
  const member = await seedUser(env, "member@example.com");
  const owner = await signedIn(env, "owner@example.com", "owner");
  const inv = await invite(env, owner.browser, { email: "invited@example.com" });
  return { env, ids: { invitation: inv.body.invitation.id, member } as Ids, token: tokenOf(inv.body.link) };
}

async function holder(env: Env, who: Who, lacking: boolean): Promise<Browser> {
  const perms = who === "auth:invite" || who === "auth:admin" ? (lacking ? ["newsletter:read"] : [who]) : ["newsletter:read"];
  return (await signedIn(env, `${lacking ? "lacking" : "holding"}@example.com`, perms)).browser;
}

test("GO-134 route table: every handler in the GO-134 files has exactly one line", async () => {
  const found: string[] = [];
  for (const file of new Set(files)) {
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
  test(`GO-134 route table: ${key} — ${spec.who}`, async () => {
    const mod = await import(join(ROOT, file));
    const handler = mod[`onRequest${method[0]}${method.slice(1).toLowerCase()}`];
    if (spec.who === "auth:invite" || spec.who === "auth:admin") {
      const { env, ids } = await fixture();
      const params = spec.params?.(ids) ?? {};
      const json = spec.json?.(ids);
      const run = (browser?: Browser, extra = {}) => call(handler, env, urlOf(file, params), { method, browser, params, ...(json !== undefined ? { json } : {}), ...extra });
      const none = await run();
      assert.deepEqual([none.status, await none.json()], [401, { error: "not_authenticated" }]);
      const without = await run(await holder(env, spec.who, true));
      assert.deepEqual([without.status, await without.json()], [403, { error: "forbidden", permission: spec.who }]);
      const holding = await holder(env, spec.who, false);
      const ok = await run(holding);
      assert.ok(ok.status >= 200 && ok.status < 300, `${key}: ${ok.status} ${await ok.clone().text()}`);
      if (spec.csrf) {
        const noHeader = await call(handler, env, urlOf(file, params), { method, browser: holding, params, csrf: false, json: json ?? {}, headers: { origin: "https://example.com" } });
        assert.equal(noHeader.status, 403, `${key} without x-ssg-auth`);
        const cross = await call(handler, env, urlOf(file, params), { method, browser: holding, params, json: json ?? {}, headers: { origin: "https://evil.example.com" } });
        assert.equal(cross.status, 403, `${key} from another origin`);
      }
      return;
    }
    if (spec.who === "session" && method === "GET") {
      const { env } = await fixture();
      assert.equal((await call(handler, env, urlOf(file, {}))).status, 401);
      assert.equal((await call(handler, env, urlOf(file, {}), { browser: await holder(env, "session", true) })).status, 200);
      return;
    }
    if (spec.csrf) {
      // Public or session POST/DELETE: refused from another origin before any work.
      const { env } = await fixture();
      const cross = await call(handler, env, urlOf(file, { id: "x" }), { method, params: { id: "x" }, json: {}, headers: { origin: "https://evil.example.com" } });
      assert.equal(cross.status, 403, `${key} from another origin`);
      return;
    }
    const { env } = await fixture();
    assert.equal((await call(handler, env, urlOf(file, {}), { csrf: false })).status, 303, "resume without a session: to the login page");
  });
}
