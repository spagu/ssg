// GO-140: step-up — sensitive changes need a second factor from the last
// AUTH_STEP_UP_MINUTES when the account has one: sign-in methods and factors,
// a service token with allowSensitive, handing out `*`, ending another
// account's sessions. Required, honoured after POST /api/auth/mfa/step-up,
// and gone again when it is old.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as removeMethod } from "../functions/api/auth/methods/[id].ts";
import { onRequestPost as grant } from "../functions/api/auth/admin/grants/index.ts";
import { onRequestPost as addMember } from "../functions/api/auth/admin/groups/[id]/members/index.ts";
import { onRequestPost as invite } from "../functions/api/auth/admin/invitations/index.ts";
import { onRequestPost as revoke } from "../functions/api/auth/admin/users/[id]/revoke-sessions.ts";
import { onRequestPost as tokens } from "../functions/api/auth/admin/service-accounts/[id]/tokens/index.ts";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { onRequestPost as start } from "../functions/api/auth/oauth/[provider]/start.ts";
import { onRequestGet as stepUpPage } from "../functions/auth/mfa/step-up.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { createGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { requireStepUp, stepUpNeeded } from "../functions/api/auth/_mfa-step-up.ts";
import { T0, call, useClock } from "./fakes.ts";
import { MFA_KEY, code, enrolTotp, mfaEnv, signIn, stepUp } from "./mfa.ts";
import { roleId, seedUser } from "./seed.ts";
import { account, newToken } from "./service.ts";
import { providerEnv } from "./go151.ts";

// boss: an owner with an authenticator app, in a session that passed it at
// enrolment; the clock is then moved past the step-up window.
async function boss(env = mfaEnv()) {
  const clock = useClock();
  const id = await seedUser(env, "boss@example.com");
  await addGrant(env, roleId(env as never, "owner"), { userId: id }, "t", T0);
  const { browser } = await signIn(env, "boss@example.com");
  const { secret } = await enrolTotp(env, browser, clock.now);
  clock.advance(11 * 60_000);
  await call(refresh, env, "/api/auth/refresh", { method: "POST", browser }); // the access token lives 10 minutes
  const up = async () => {
    const r = await call(stepUp, env, "/api/auth/mfa/step-up", { json: { code: await code(secret, clock.now) }, browser });
    assert.equal(r.status, 200, await r.clone().text());
    clock.advance(30_000);
  };
  return { clock, env, id, browser, secret, up };
}

const refused = async (res: Response) => assert.deepEqual([res.status, (await res.json()).error], [403, "step_up_required"]);

test("each sensitive admin action: refused without a recent second factor, done after a step-up", async () => {
  const b = await boss();
  const other = await seedUser(b.env, "eva@example.com");
  const ownerRole = roleId(b.env as never, "owner");
  const group = (await createGroup(b.env, "Owners", "", T0)) as Group;
  await addGrant(b.env, ownerRole, { groupId: group.id }, "t", T0);
  const svc = await account(b.env, b.browser, ["newsletter:erase"]);
  const actions: [string, () => Promise<Response>][] = [
    ["grant owner", () => call(grant, b.env, "/api/auth/admin/grants", { json: { roleId: ownerRole, userId: other }, browser: b.browser })],
    ["join a group holding *", () => call(addMember, b.env, `/api/auth/admin/groups/${group.id}/members`, { json: { userId: other }, browser: b.browser, params: { id: group.id } })],
    ["invite with owner", () => call(invite, b.env, "/api/auth/admin/invitations", { json: { email: "new@example.com", roleIds: [ownerRole], delivery: "link" }, browser: b.browser })],
    ["end another account's sessions", () => call(revoke, b.env, `/api/auth/admin/users/${other}/revoke-sessions`, { method: "POST", browser: b.browser, params: { id: other } })],
    ["a token with allowSensitive", () => call(tokens, b.env, `/api/auth/admin/service-accounts/${svc}/tokens`, { json: { allowSensitive: ["newsletter:erase"] }, browser: b.browser, params: { id: svc } })],
  ];
  for (const [name, act] of actions) {
    const res = await act();
    assert.equal(res.status, 403, name);
    assert.deepEqual(await res.json(), { error: "step_up_required", stepUpUrl: "/auth/mfa/step-up", minutes: 10 }, name);
  }
  await b.up();
  for (const [name, act] of actions) assert.ok((await act()).status < 300, `${name} after the step-up`);
  // Without anything sensitive in them the same routes never ask.
  b.clock.advance(11 * 60_000);
  await call(refresh, b.env, "/api/auth/refresh", { method: "POST", browser: b.browser });
  assert.equal((await newToken(b.env, b.browser, svc, {})).status, 201, "a plain token");
  const self = await call(revoke, b.env, `/api/auth/admin/users/${b.id}/revoke-sessions`, { method: "POST", browser: b.browser, params: { id: b.id } });
  assert.equal(self.status, 200, "ending one's own sessions");
});

test("sign-in methods: removing one needs a step-up (a form goes to the step-up page); linking a provider too", async () => {
  const env = providerEnv({ AUTH_MFA_KEY: MFA_KEY } as never);
  const b = await boss(env);
  await refused(await call(removeMethod, env, "/api/auth/methods/password", { method: "POST", browser: b.browser, params: { id: "password" }, json: {} }));
  const form = await call(removeMethod, env, "/api/auth/methods/password", { form: { a: "b" }, browser: b.browser, params: { id: "password" } });
  assert.equal(form.headers.get("location"), "/auth/mfa/step-up?next=%2Fauth%2Faccount");
  const link = await call(start, env, "/api/auth/oauth/google/start", { json: { purpose: "link" }, browser: b.browser, params: { provider: "google" } });
  await refused(link);
  const linkForm = await call(start, env, "/api/auth/oauth/google/start", { form: { purpose: "link" }, browser: b.browser, params: { provider: "google" } });
  assert.equal(linkForm.headers.get("location"), "/auth/mfa/step-up?next=%2Fauth%2Faccount");
  await b.up();
  // A step-up stands in for a recent sign-in: this session is 11 minutes old.
  const linked = await call(start, env, "/api/auth/oauth/google/start", { json: { purpose: "link" }, browser: b.browser, params: { provider: "google" } });
  assert.equal(linked.status, 200);
  const last = await call(removeMethod, env, "/api/auth/methods/password", { method: "POST", browser: b.browser, params: { id: "password" }, json: {} });
  assert.deepEqual([last.status, (await last.json()).error], [409, "method_last"], "past the gate, the usual rules");
});

test("the window: a step-up counts AUTH_STEP_UP_MINUTES, then is asked again; an account without a factor is never asked", async () => {
  const b = await boss(mfaEnv({ AUTH_STEP_UP_MINUTES: "2" }));
  const ctx = { env: b.env, now: b.clock.now } as never;
  const holder = async () => ({ user: { id: b.id }, session: { id: JSON.parse(Buffer.from((b.browser.jar.get("__Host-ssg-auth") as string).split(".")[1], "base64url").toString()).sid } });
  assert.equal(await stepUpNeeded(ctx, await holder()), true);
  await b.up();
  assert.equal(await stepUpNeeded({ env: b.env, now: b.clock.now } as never, await holder()), false);
  b.clock.advance(3 * 60_000);
  assert.ok(await requireStepUp({ env: b.env, now: b.clock.now } as never, await holder()));
  assert.equal(await requireStepUp({ env: b.env, now: b.clock.now } as never, { user: { id: "no-factor" }, session: { id: "s" } }), null);
});

test("the step-up route: wrong codes, the lockout, forms, the account without a factor, and the page", async () => {
  const b = await boss();
  const bad = await call(stepUp, b.env, "/api/auth/mfa/step-up", { json: { code: "000000" }, browser: b.browser });
  assert.deepEqual([bad.status, (await bad.json()).error], [401, "invalid_code"]);
  const badForm = await call(stepUp, b.env, "/api/auth/mfa/step-up", { form: { backupCode: "AAAA-AAAA-AAAA-AAAA", next: "/auth/account" }, browser: b.browser });
  assert.equal(badForm.headers.get("location"), "/auth/mfa/step-up?next=%2Fauth%2Faccount&error=invalid_code&use=backup");
  for (let i = 0; i < 4; i++) await call(stepUp, b.env, "/api/auth/mfa/step-up", { json: { code: "000000" }, browser: b.browser });
  const locked = await call(stepUp, b.env, "/api/auth/mfa/step-up", { json: { code: await code(b.secret, b.clock.now) }, browser: b.browser });
  assert.equal(locked.status, 429);
  assert.ok(locked.headers.get("retry-after"));
  b.clock.advance(61_000);
  const ok = await call(stepUp, b.env, "/api/auth/mfa/step-up", { form: { code: await code(b.secret, b.clock.now), next: "/auth/account" }, browser: b.browser });
  assert.equal(ok.headers.get("location"), "/auth/account");
  await seedUser(b.env, "eva@example.com");
  const eva = await signIn(b.env, "eva@example.com");
  const none = await call(stepUp, b.env, "/api/auth/mfa/step-up", { json: { code: "123456" }, browser: eva.browser });
  assert.deepEqual([none.status, (await none.json()).error], [409, "no_factor"]);
  assert.equal((await call(stepUp, b.env, "/api/auth/mfa/step-up", { json: { code: "1" } })).status, 401);
  const page = await call(stepUpPage, b.env, "/auth/mfa/step-up?next=/auth/account", { browser: eva.browser });
  assert.equal(page.headers.get("location"), "/auth/account", "nothing to step up with: on to next");
  const shown = await call(stepUpPage, b.env, "/auth/mfa/step-up?next=/auth/account&error=invalid_code", { browser: b.browser });
  const html = await shown.text();
  assert.match(html, /<form class="stack" method="post" action="\/api\/auth\/mfa\/step-up"><input type="hidden" name="next" value="\/auth\/account">/);
  assert.match(html, /aria-invalid="true"/);
  assert.equal((await call(stepUpPage, b.env, "/auth/mfa/step-up")).status, 303, "not signed in");
});
