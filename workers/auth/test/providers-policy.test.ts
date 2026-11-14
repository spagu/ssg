// GO-151, the per-role method policy (AUTH_ROLE_METHODS): a role counts only
// in sessions started with one of its methods — at sign-in and at every
// refresh — the pages say which roles are left out and why, and a broken
// policy fails closed.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as panels } from "../functions/auth/index.ts";
import { onRequestGet as account } from "../functions/auth/account.ts";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { policyView, rolePolicy } from "../functions/api/auth/_provider-policy.ts";
import { jwksCache } from "../functions/api/auth/_provider-oidc.ts";
import { b64urlDecode } from "../functions/api/auth/_lib.ts";
import { Browser, call, useClock } from "./fakes.ts";
import { pageOf } from "./go134.ts";
import { grantPerms, roleId, signedIn } from "./seed.ts";
import { fakeProviders, providerEnv, roundTrip } from "./go151.ts";
import { identityInsert } from "../functions/api/auth/_provider-identities.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const perms = (b: Browser): string[] => {
  const jwt = b.jar.get("__Host-ssg-auth") as string;
  return JSON.parse(new TextDecoder().decode(b64urlDecode(jwt.split(".")[1]) as Uint8Array)).perms;
};

async function owner(policy: string) {
  const clock = useClock();
  jwksCache.clear();
  const env = providerEnv({ AUTH_ROLE_METHODS: policy } as Partial<Env>);
  const { browser, id } = await signedIn(env, "boss@example.com", "owner");
  await grantPerms(env, id, ["newsletter:read"]);
  await identityInsert(env, id, "google", { subject: "g-boss", email: "boss@example.com", emailVerified: true, name: "" }, clock.now).run();
  // A new token with the role granted after the sign-in.
  await call(refresh, env, "/api/auth/refresh", { method: "POST", browser });
  return { clock, env, browser, id };
}

test("policy: owner only through Google — a password session leaves the owner role out, says why, and so does every refresh", async () => {
  const { clock, env, browser } = await owner(JSON.stringify({ owner: ["google"], unknown: ["github"] }));
  assert.deepEqual(perms(browser), ["newsletter:read"], "the owner's * is left out, the other role stays");
  for (const [handler, path] of [[account, "/auth/account"], [panels, "/auth/"]] as const) {
    const { html } = await pageOf(handler, env, path, browser);
    assert.ok(html.includes("Some permissions are left out of this session"), path);
    assert.ok(html.includes("Role owner: only in sessions started with Google. This session was started with Password."), path);
  }
  clock.advance(11 * 60_000);
  const res = await call(refresh, env, "/api/auth/refresh", { method: "POST", browser });
  assert.equal(res.status, 200);
  assert.deepEqual(perms(browser), ["newsletter:read"], "a refresh applies the policy too");

  const fake = fakeProviders();
  try {
    const g = new Browser();
    await roundTrip(env, fake, g, "google", { sub: "g-boss" });
    assert.deepEqual(perms(g), ["*", "newsletter:read"]);
    assert.ok(!(await pageOf(account, env, "/auth/account", g)).html.includes("Some permissions are left out"));
  } finally {
    fake.restore();
  }
});

test("policy: a permission given by an allowed role stays; roles not listed allow every method", async () => {
  const { env, browser, id } = await owner(JSON.stringify({ "test-role-strict": ["github"] }));
  const strict = env.AUTH_DB.sqlite.prepare("SELECT id FROM roles WHERE name LIKE 'test-role-%'").get() as { id: string };
  env.AUTH_DB.sqlite.exec(`UPDATE roles SET name = 'test-role-strict' WHERE id = '${strict.id}'`);
  const view = await policyView(env, id, "password");
  assert.deepEqual(view.withheld, [{ role: "test-role-strict", methods: ["github"] }]);
  assert.deepEqual(view.perms, ["*"], "newsletter:read came only from the strict role");
  await addGrant(env, roleId(env as never, "newsletter-viewer"), { userId: id }, "t", 0);
  assert.deepEqual((await policyView(env, id, "password")).perms, ["*", "newsletter:read"], "another role still gives it");
  assert.ok(browser);
});

test("policy: a broken AUTH_ROLE_METHODS fails closed — no role counts — and the pages say so", async () => {
  for (const bad of ["{not json", "[]", '"owner"', JSON.stringify({ owner: "google" }), JSON.stringify({ owner: [1] }), "null"]) {
    const env = providerEnv({ AUTH_ROLE_METHODS: bad } as Partial<Env>);
    assert.equal(rolePolicy(env), "invalid", bad);
  }
  const errors: string[] = [];
  const real = console.error;
  console.error = (m: string) => errors.push(m);
  try {
    const { env, browser } = await owner("{oops");
    assert.deepEqual(perms(browser), []);
    assert.ok((await pageOf(account, env, "/auth/account", browser)).html.includes("The site&#39;s sign-in rules are set up wrongly"));
    assert.ok(errors.some((e) => e.includes("AUTH_ROLE_METHODS")));
  } finally {
    console.error = real;
  }
  assert.equal(rolePolicy(providerEnv({ AUTH_ROLE_METHODS: "  " } as Partial<Env>)), null);
});
