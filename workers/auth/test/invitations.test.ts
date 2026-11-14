// Invitations (GO-134): create, list, the escalation rule, mail vs link,
// replace, withdraw, send again.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestDelete as revoke } from "../functions/api/auth/admin/invitations/[id]/index.ts";
import { onRequestPost as resend } from "../functions/api/auth/admin/invitations/[id]/resend.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { createGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { sha256hex } from "../functions/api/auth/_lib.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { T0, call, makeEnv, useClock } from "./fakes.ts";
import { acceptWith, captureMail, createInvite, invite, linkIn, listInvites, rows, tokenOf, withMail } from "./go134.ts";
import { roleId, seedUser, signedIn } from "./seed.ts";

const role = async (env: ReturnType<typeof makeEnv>, name: string, permissions: string[]) =>
  (await createRole(env, { name, description: "", permissions }, T0)) as Role;

test("invite without mail: 201 with the one-time link; only its hash is stored; listed as pending", async () => {
  useClock();
  const env = makeEnv();
  const { browser, id } = await signedIn(env, "boss@example.com", ["auth:invite"]);
  const { status, body } = await invite(env, browser, { email: " New@Example.com ", name: "Nina" });
  assert.equal(status, 201);
  assert.equal(body.delivery, "link");
  const url = new URL(body.link);
  assert.equal(url.origin + url.pathname, "https://example.com/auth/invite");
  const token = tokenOf(body.link);
  assert.match(token, /^[A-Za-z0-9_-]{43}$/);
  assert.equal(url.searchParams.get("lang"), "en");
  assert.deepEqual(
    { email: body.invitation.email, name: body.invitation.name, status: body.invitation.status, by: body.invitation.invitedBy, byEmail: body.invitation.invitedByEmail },
    { email: "new@example.com", name: "Nina", status: "pending", by: id, byEmail: "boss@example.com" },
  );
  assert.equal(body.invitation.expiresAt, new Date(T0 + 72 * 3600_000).toISOString());
  const [row] = rows(env, "invitations");
  assert.equal(row.token_hash, await sha256hex(token));
  assert.ok(!JSON.stringify(rows(env, "invitations")).includes(token), "the token itself is nowhere in the table");
  const list = await (await call(listInvites, env, "/api/auth/admin/invitations", { browser })).json();
  assert.deepEqual(list.items.map((i: { status: string }) => i.status), ["pending"]);
  assert.equal(list.nextCursor, null);
});

test("invite with mail: sent in the invitee's language, the link only in the mail; default language = the inviter's page language", async () => {
  useClock();
  const env = withMail({ AUTH_INVITE_HOURS: "24" });
  const mail = captureMail();
  try {
    const { browser } = await signedIn(env, "boss@example.com", ["auth:invite"]);
    const pl = await invite(env, browser, { email: "ola@example.com", lang: "pl" });
    assert.equal(pl.status, 201);
    assert.deepEqual(Object.keys(pl.body).sort(), ["delivery", "invitation"]);
    assert.equal(pl.body.delivery, "mail");
    assert.equal(mail.sent[0].to, "ola@example.com");
    assert.equal(mail.sent[0].subject, "Zaproszenie do example.com");
    assert.match(mail.sent[0].text, /^Ada zaprasza Cię/);
    assert.match(mail.sent[0].text, /24 godzin po wysłaniu/);
    const link = linkIn(mail.sent[0].text);
    assert.equal(new URL(link).searchParams.get("lang"), "pl");
    assert.equal((await acceptWith(env, tokenOf(link))).status, 201, "the mailed link works");
    env.AUTH_DB.sqlite.exec("UPDATE users SET display_name = '' WHERE email = 'boss@example.com'");
    const de = await call(createInvite, env, "/api/auth/admin/invitations", { json: { email: "dora@example.com" }, browser, headers: { "accept-language": "de-AT,de;q=0.9,en;q=0.5" } });
    assert.equal(de.status, 201);
    assert.equal(mail.sent[1].subject, "Einladung zu example.com");
    assert.match(mail.sent[1].text, /^boss@example\.com hat Sie/, "no name: the inviter's address");
    assert.equal((await de.json()).invitation.lang, "de");
  } finally {
    mail.restore();
  }
});

test("invite: the inviter cannot hand out more than they hold — roles and groups alike; * can hand out anything", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await signedIn(env, "lead@example.com", ["auth:invite", "newsletter:read", "comments:*"]);
  const reader = await role(env, "reader", ["newsletter:read"]);
  const sender = await role(env, "sender", ["newsletter:read", "newsletter:send"]);
  const mods = await role(env, "mods", ["comments:moderate"]);
  const group = (await createGroup(env, "Senders", "", T0)) as Group;
  await addGrant(env, sender.id, { groupId: group.id }, "x", T0);
  const refused = await invite(env, browser, { email: "a@example.com", roleIds: [reader.id, sender.id] });
  assert.equal(refused.status, 403);
  assert.deepEqual([refused.body.error, refused.body.permissions], ["escalation", ["newsletter:send"]]);
  const viaGroup = await invite(env, browser, { email: "a@example.com", groupIds: [group.id] });
  assert.deepEqual([viaGroup.status, viaGroup.body.permissions], [403, ["newsletter:send"]]);
  const owner = await invite(env, browser, { email: "a@example.com", roleIds: [roleId(env as never, "owner")] });
  assert.deepEqual([owner.status, owner.body.permissions], [403, ["*"]]);
  assert.equal((await invite(env, browser, { email: "a@example.com", roleIds: [reader.id, mods.id, reader.id] })).status, 201, "covered (comments:* covers comments:moderate); duplicates collapse");
  const top = await signedIn(env, "top@example.com", "owner");
  const asOwner = await invite(env, top.browser, { email: "b@example.com", roleIds: [roleId(env as never, "owner")], groupIds: [group.id] });
  assert.equal(asOwner.status, 201);
  assert.equal(rows(env, "invitations").length, 2, "refusals stored nothing");
});

test("invite: input checks, unknown roles and groups, an address that has an account", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await signedIn(env, "boss@example.com", ["auth:invite"]);
  await seedUser(env, "taken@example.com");
  const cases: [Record<string, unknown>, number, string][] = [
    [{ email: "not an address" }, 400, "invalid_request"],
    [{ email: "a@example.com", name: "x".repeat(201) }, 400, "invalid_request"],
    [{ email: "a@example.com", name: 7 }, 400, "invalid_request"],
    [{ email: "a@example.com", roleIds: "r" }, 400, "invalid_request"],
    [{ email: "a@example.com", groupIds: [""] }, 400, "invalid_request"],
    [{ email: "a@example.com", roleIds: Array.from({ length: 21 }, (_, i) => `r${i}`) }, 400, "invalid_request"],
    [{ email: "a@example.com", lang: "xx" }, 400, "invalid_request"],
    [{ email: "a@example.com", delivery: "pigeon" }, 400, "invalid_request"],
    [{ email: "a@example.com", delivery: "mail" }, 409, "mail_not_configured"],
    [{ email: "Taken@example.com" }, 409, "account_exists"],
    [{ email: "a@example.com", roleIds: ["nope"] }, 404, "role_not_found"],
    [{ email: "a@example.com", groupIds: ["nope"] }, 404, "group_not_found"],
  ];
  for (const [json, status, error] of cases) {
    const r = await invite(env, browser, json);
    assert.deepEqual([r.status, r.body.error], [status, error], JSON.stringify(json));
  }
  const raw = await call(createInvite, env, "/api/auth/admin/invitations", { method: "POST", browser, headers: { "content-type": "application/json" }, json: undefined });
  assert.equal(raw.status, 400);
  assert.equal(rows(env, "invitations").length, 0);
});

test("invite: a broken transport is an error up front; delivery: link still works; a refused mail is 502 and the invitation stays", async () => {
  useClock();
  const broken = makeEnv({ AUTH_RESEND_API_KEY: "re_x" }); // no sender
  const b = await signedIn(broken, "boss@example.com", ["auth:invite"]);
  const r = await invite(broken, b.browser, { email: "a@example.com" });
  assert.deepEqual([r.status, r.body.error], [503, "mail_misconfigured"]);
  assert.match(r.body.detail, /AUTH_MAIL_FROM/);
  assert.equal((await invite(broken, b.browser, { email: "a@example.com", delivery: "link" })).body.delivery, "link");
  const smtp25 = makeEnv({ AUTH_SMTP_HOST: "smtp.example.com", AUTH_SMTP_PORT: "25", AUTH_MAIL_FROM: "a@example.com" });
  const s = await signedIn(smtp25, "boss@example.com", ["auth:invite"]);
  assert.match((await invite(smtp25, s.browser, { email: "a@example.com" })).body.detail, /port 25/);

  const env = withMail();
  const mail = captureMail();
  mail.status = 403;
  const warn = console.warn;
  const logged: string[] = [];
  console.warn = (...a: unknown[]) => void logged.push(a.join(" "));
  try {
    const { browser } = await signedIn(env, "boss@example.com", ["auth:invite"]);
    const failed = await invite(env, browser, { email: "c@example.com" });
    assert.deepEqual([failed.status, failed.body.error, failed.body.temporary], [502, "mail_failed", false]);
    assert.equal(rows(env, "invitations")[0].revoked_at, null, "kept: resend or withdraw it");
    assert.ok(!logged.join("\n").includes(linkIn(mail.sent[0].text)), "the link is never logged");
  } finally {
    console.warn = warn;
    mail.restore();
  }
});

test("invite again: the earlier invitation is replaced and its link stops working", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await signedIn(env, "boss@example.com", ["auth:invite"]);
  const first = await invite(env, browser, { email: "a@example.com" });
  const second = await invite(env, browser, { email: "a@example.com" });
  assert.equal((await acceptWith(env, tokenOf(first.body.link))).status, 410);
  const list = (await (await call(listInvites, env, "/api/auth/admin/invitations", { browser })).json()).items;
  const byId = new Map(list.map((i: { id: string }) => [i.id, i]));
  assert.deepEqual([(byId.get(first.body.invitation.id) as { status: string; revokeReason: string }).status, (byId.get(first.body.invitation.id) as { revokeReason: string }).revokeReason], ["revoked", "replaced"]);
  assert.equal((await acceptWith(env, tokenOf(second.body.link))).status, 201);
});

test("withdraw: the link dies; again is fine; accepted is 409; unknown is 404", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await signedIn(env, "boss@example.com", ["auth:invite"]);
  const a = await invite(env, browser, { email: "a@example.com" });
  const del = (id: string) => call(revoke, env, `/api/auth/admin/invitations/${id}`, { method: "DELETE", browser, params: { id } });
  assert.deepEqual(await (await del(a.body.invitation.id)).json(), { ok: true });
  assert.equal((await del(a.body.invitation.id)).status, 200);
  assert.equal((await acceptWith(env, tokenOf(a.body.link))).status, 410);
  const b = await invite(env, browser, { email: "b@example.com" });
  await acceptWith(env, tokenOf(b.body.link));
  const states = (await (await call(listInvites, env, "/api/auth/admin/invitations", { browser })).json()).items.map((i: { status: string }) => i.status);
  assert.deepEqual(states.sort(), ["accepted", "revoked"]);
  const accepted = await del(b.body.invitation.id);
  assert.deepEqual([accepted.status, (await accepted.json()).error], [409, "invitation_accepted"]);
  assert.equal((await del("nope")).status, 404);
});
