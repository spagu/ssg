// GO-141 end to end: an owner creates a service account and a token in
// `auth`, a script exchanges it, and the JWT is accepted by web-push's notify
// and republish-trigger — with the right permission only. Skipped (with the
// reason) when those templates are not next to this one.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { KEYS, T0, makeEnv, useClock } from "./fakes.ts";
import { signedIn } from "./seed.ts";
import { account, secretFor, swap } from "./service.ts";

const WEB_PUSH = new URL("../../web-push/functions/api/push/notify.ts", import.meta.url);
const REPUBLISH = new URL("../../republish-trigger/functions/api/republish/index.ts", import.meta.url);
const skip = existsSync(WEB_PUSH) && existsSync(REPUBLISH) ? false : "workers/web-push or workers/republish-trigger is not next to workers/auth";

const consumer = (prefix: string) => ({
  [`${prefix}_AUTH_ISSUER`]: "https://example.com/api/auth",
  [`${prefix}_AUTH_PUBLIC_KEY`]: KEYS.pub,
  [`${prefix}_AUTH_LOGIN_URL`]: "/auth/login",
});
const kv = () => {
  const m = new Map<string, string>();
  return { get: async (k: string) => m.get(k) ?? null, put: async (k: string, v: string) => void m.set(k, v) };
};
const pages = (request: Request, env: object) => ({ request, env, params: {}, data: {}, waitUntil: () => {}, next: async () => new Response() }) as never;

// jwtFor gives a service account `perms` through one role and, when given,
// `sensitive` through a second role. They are kept apart on purpose: a role
// holding a sensitive permission is one people may use only with a second
// factor, and such a role gives a machine nothing at all without an owner's
// allowSensitive (_service-policy.ts) — so mixing them in one role would also
// withhold the ordinary permissions these tests exercise.
async function jwtFor(perms: string[], tokenList?: string[], sensitive: string[] = []) {
  useClock(Date.now()); // the consumers check exp against the real clock
  const env = makeEnv();
  const owner = await signedIn(env, "owner@example.com", "owner");
  const id = await account(env, owner.browser, perms);
  if (sensitive.length) {
    const role = (await createRole(env, { name: "svc-sensitive", description: "", permissions: sensitive }, T0)) as Role;
    await addGrant(env, role.id, { userId: id }, "test", T0);
  }
  const secret = await secretFor(env, owner.browser, id, tokenList ? { permissions: tokenList } : {});
  const res = await swap(env, `Bearer ${secret}`);
  assert.equal(res.status, 200);
  return (await res.json()).accessToken as string;
}

test("e2e: account + token → exchange → web-push notify and republish-trigger accept it", { skip }, async (t) => {
  const { onRequestPost: notify } = await import(WEB_PUSH.href);
  const { onRequestPost: republish } = await import(REPUBLISH.href);
  const feed = `<feed xmlns="http://www.w3.org/2005/Atom"><entry><title>One</title><link href="/p/1/"/><id>1</id></entry></feed>`;
  const dispatched: string[] = [];
  t.mock.method(globalThis, "fetch", async (url: string) => {
    if (url.startsWith("https://api.github.com/")) return (dispatched.push(url), new Response(null, { status: 204 }));
    return new Response(feed, { headers: { "content-type": "application/atom+xml" } });
  });
  const jwt = await jwtFor(["web-push:notify", "republish-trigger:run"], undefined, ["auth:admin"]);
  const authorization = `Bearer ${jwt}`;

  const pushEnv = { PUSH_DB: {}, PUSH_KV: kv(), PUSH_SUBJECT: "mailto:news@example.com", PUSH_VAPID_PUBLIC_KEY: "p", PUSH_VAPID_PRIVATE_KEY: "q", ...consumer("PUSH") };
  const pushed = await notify(pages(new Request("https://example.com/api/push/notify", { method: "POST", headers: { authorization } }), pushEnv));
  assert.deepEqual([pushed.status, await pushed.json()], [200, { sent: 0, removed: 0, items: [], primed: "1" }]);

  const repEnv = { REPUBLISH_PROVIDER: "github", REPUBLISH_TOKEN: "ghp_x", REPUBLISH_REPO: "owner/site", REPUBLISH_WORKFLOW: "deploy.yml", ...consumer("REPUBLISH") };
  const run = await republish(pages(new Request("https://example.com/api/republish", { method: "POST", headers: { authorization } }), repEnv));
  assert.deepEqual([run.status, await run.json()], [202, { ok: true, provider: "github", ref: "main" }]);
  assert.equal(dispatched.length, 1);
});

test("e2e: a token narrowed to web-push:notify is refused by republish-trigger with 403", { skip }, async () => {
  const { onRequestPost: republish } = await import(REPUBLISH.href);
  const jwt = await jwtFor(["web-push:notify", "republish-trigger:run"], ["web-push:notify"]);
  const repEnv = { REPUBLISH_PROVIDER: "github", REPUBLISH_TOKEN: "ghp_x", REPUBLISH_KEY: "legacy-key", ...consumer("REPUBLISH") };
  const res = await republish(pages(new Request("https://example.com/api/republish", { method: "POST", headers: { authorization: `Bearer ${jwt}` } }), repEnv));
  assert.deepEqual([res.status, await res.json()], [403, { error: "forbidden", permission: "republish-trigger:run" }]);
});

test("e2e: the sensitive auth:admin never reached the machine's token", { skip }, async () => {
  const jwt = await jwtFor(["web-push:notify"], undefined, ["auth:admin"]);
  assert.deepEqual(JSON.parse(Buffer.from(jwt.split(".")[1], "base64url").toString()).perms, ["web-push:notify"]);
});
