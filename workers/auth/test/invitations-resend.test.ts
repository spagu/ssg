// Sending an invitation again (GO-134): a new link kills the old one; the one
// resending is checked like the one who invited.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as resend } from "../functions/api/auth/admin/invitations/[id]/resend.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { createGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { T0, call, makeEnv, useClock } from "./fakes.ts";
import { acceptWith, captureMail, invite, listInvites, tokenOf, withMail } from "./go134.ts";
import { seedUser, signedIn } from "./seed.ts";

const role = async (env: ReturnType<typeof makeEnv>, name: string, permissions: string[]) =>
  (await createRole(env, { name, description: "", permissions }, T0)) as Role;


test("send again: a new link, the old one dead, the expiry renewed — also for an expired invitation", async () => {
  const clock = useClock();
  const env = makeEnv();
  const { browser } = await signedIn(env, "boss@example.com", ["auth:invite"]);
  const a = await invite(env, browser, { email: "a@example.com" });
  const id = a.body.invitation.id;
  const again = (json?: unknown, b = browser) => call(resend, env, `/api/auth/admin/invitations/${id}/resend`, { method: "POST", browser: b, params: { id }, ...(json ? { json } : {}) });
  // Expired (the clock stays: the inviter's access token must stay valid).
  env.AUTH_DB.sqlite.prepare("UPDATE invitations SET expires_at = ?").run(new Date(clock.now - 1).toISOString());
  const listed = (await (await call(listInvites, env, "/api/auth/admin/invitations", { browser })).json()).items[0];
  assert.equal(listed.status, "expired");
  const res = await again();
  assert.equal(res.status, 200);
  const body = await res.json();
  assert.equal(body.invitation.status, "pending");
  assert.equal(body.invitation.expiresAt, new Date(clock.now + 72 * 3600_000).toISOString());
  assert.notEqual(tokenOf(body.link), tokenOf(a.body.link));
  assert.equal((await acceptWith(env, tokenOf(a.body.link))).status, 410, "the old link is dead");
  assert.equal((await (await again({ delivery: "pigeon" })).json()).error, "invalid_request");
  assert.equal((await acceptWith(env, tokenOf(body.link))).status, 201);
  assert.equal((await (await again()).json()).error, "invitation_closed");
  assert.equal((await call(resend, env, "/api/auth/admin/invitations/x/resend", { method: "POST", browser, params: { id: "x" } })).status, 404);
});

test("send again: refused when the address got an account, or when the one resending holds less than the invitation hands out", async () => {
  useClock();
  const env = makeEnv();
  const top = await signedIn(env, "top@example.com", "owner");
  const small = await signedIn(env, "small@example.com", ["auth:invite"]);
  const sender = await role(env, "sender", ["newsletter:send"]);
  const gone = await role(env, "gone", ["newsletter:read"]);
  const group = (await createGroup(env, "G", "", T0)) as Group;
  await addGrant(env, sender.id, { groupId: group.id }, "x", T0);
  const a = await invite(env, top.browser, { email: "a@example.com", roleIds: [sender.id, gone.id], groupIds: [group.id] });
  await env.AUTH_DB.prepare("DELETE FROM roles WHERE id = ?").bind(gone.id).run();
  const id = a.body.invitation.id;
  const bySmall = await call(resend, env, `/api/auth/admin/invitations/${id}/resend`, { method: "POST", browser: small.browser, params: { id } });
  assert.deepEqual([bySmall.status, (await bySmall.json()).permissions], [403, ["newsletter:send"]]);
  await seedUser(env, "a@example.com");
  const taken = await call(resend, env, `/api/auth/admin/invitations/${id}/resend`, { method: "POST", browser: top.browser, params: { id } });
  assert.deepEqual([taken.status, (await taken.json()).error], [409, "account_exists"]);
});

test("send again by mail; a lost race to the renewal is invitation_closed", async () => {
  useClock();
  const env = withMail();
  const mail = captureMail();
  try {
    const { browser } = await signedIn(env, "boss@example.com", ["auth:invite"]);
    const a = await invite(env, browser, { email: "a@example.com", delivery: "link" });
    const id = a.body.invitation.id;
    const res = await call(resend, env, `/api/auth/admin/invitations/${id}/resend`, { method: "POST", browser, params: { id } });
    assert.deepEqual(await res.json().then((b) => [b.delivery, b.link]), ["mail", undefined]);
    assert.equal(mail.sent.length, 1);
    mail.status = 500;
    const failed = await call(resend, env, `/api/auth/admin/invitations/${id}/resend`, { method: "POST", browser, params: { id } });
    assert.deepEqual(await failed.json().then((b) => [failed.status, b.error, b.temporary]), [502, "mail_failed", true]);
    // Accepted between the check and the renewal: the UPDATE matches nothing.
    const real = env.AUTH_DB.prepare;
    env.AUTH_DB.prepare = ((sql: string) => (sql.startsWith("UPDATE invitations SET token_hash") ? real("UPDATE invitations SET token_hash = ?, expires_at = ? WHERE id = ? AND 0") : real(sql))) as never;
    const raced = await call(resend, env, `/api/auth/admin/invitations/${id}/resend`, { method: "POST", browser, params: { id }, json: { delivery: "link" } });
    env.AUTH_DB.prepare = real;
    assert.deepEqual([raced.status, (await raced.json()).error], [409, "invitation_closed"]);
  } finally {
    mail.restore();
  }
});
