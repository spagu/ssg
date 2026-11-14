// Invented data for the admin panel's preview (GO-138; dev-server.mjs adds
// it to demo-seed.ts's): about forty people, groups, built-in and custom
// roles, open invitations, service accounts with tokens, sessions started
// different ways — some with a second factor —, ninety days of audit events
// and a Cloudflare Access group mapping. Only example.com addresses.
import { createMapping } from "../functions/api/auth/_access-groups.ts";
import { emitEvent } from "../functions/api/auth/_audit.ts";
import type { EventKind } from "../functions/api/auth/_audit.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { addMember, createGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { createInvitation } from "../functions/api/auth/_invites.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { markSessionFactor } from "../functions/api/auth/_mfa-policy.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { createServiceAccount } from "../functions/api/auth/_service-accounts.ts";
import type { ServiceAccount } from "../functions/api/auth/_service-accounts.ts";
import { createToken, revokeToken } from "../functions/api/auth/_service-tokens.ts";
import { createSession } from "../functions/api/auth/_sessions.ts";
import { createUser } from "../functions/api/auth/_users.ts";

const DAY = 86_400_000;
const PEOPLE = [
  "Amara Okafor", "Bruno Silva", "Chen Wei", "Dagny Larsen", "Elif Yılmaz", "Farid Haddad", "Greta Novak", "Hiroshi Tanaka", "Ines Moreau", "Jonas Berg",
  "Kavya Iyer", "Liam O'Brien", "Marta Kowalska", "Nikos Pappas", "Olusegun Adeyemi", "Priya Nair", "Quentin Roux", "Rosa Martínez", "Sven Lindqvist", "Tamar Cohen",
  "Uma Desai", "Viktor Horvat", "Wen Li", "Xavier Dubois", "Yara Haddad", "Zofia Wiśniewska", "Aarav Mehta", "Beatriz Costa", "Ciarán Walsh", "Dilnoza Karimova",
  "Emre Demir", "Fatima Zahra", "Gustavo Rocha", "Hanna Virtanen", "Ivo Petrov", "Jana Dvořák", "Kenji Sato", "Lucía Fernández",
];
const slug = (name: string) => name.toLowerCase().normalize("NFD").replace(/[^a-z ]/g, "").trim().replace(/ +/g, ".");
const roleOf = async (env: Env, name: string) => ((await env.AUTH_DB.prepare("SELECT id FROM roles WHERE name = ?").bind(name).first<{ id: string }>()) as { id: string }).id;

export async function seedAdminDemo(env: Env, now: number, ownerId: string): Promise<void> {
  const ids: string[] = [];
  for (const [i, name] of PEOPLE.entries()) {
    const id = (await createUser(env, { email: `${slug(name)}@example.com`, name, passwordHash: null }, now - (120 - i * 3) * DAY)) as string;
    ids.push(id);
    if (i % 4 !== 3) await env.AUTH_DB.prepare("UPDATE users SET last_login_at = ? WHERE id = ?").bind(new Date(now - (i % 9) * 3_600_000 * 7).toISOString(), id).run();
  }
  await env.AUTH_DB.prepare("UPDATE users SET status = 'blocked' WHERE id IN (?, ?)").bind(ids[7], ids[22]).run();
  const support = (await createRole(env, { name: "support-desk", description: "Reads the moderation queue and the subscriber list to answer readers.", permissions: ["comments:read", "newsletter:read"] }, now - 60 * DAY)) as Role;
  const accounts = (await createRole(env, { name: "account-admins", description: "Looks after accounts and invitations.", permissions: ["auth:admin", "auth:invite"] }, now - 50 * DAY)) as Role;
  const editors = (await createGroup(env, "Editors", "Write and send the newsletter.", now - 70 * DAY)) as Group;
  const desk = (await createGroup(env, "Support desk", "Answer readers' questions.", now - 40 * DAY)) as Group;
  await addGrant(env, await roleOf(env, "newsletter-editor"), { groupId: editors.id }, ownerId, now - 70 * DAY);
  await addGrant(env, support.id, { groupId: desk.id }, ownerId, now - 40 * DAY);
  for (const i of [0, 1, 2, 3, 4, 5]) await addMember(env, editors.id, ids[i], ownerId, now - 60 * DAY);
  for (const i of [6, 8, 9, 10, 11, 12, 13, 14]) await addMember(env, desk.id, ids[i], ownerId, now - 30 * DAY);
  const mods = (await env.AUTH_DB.prepare("SELECT id FROM groups WHERE name = 'Moderators'").first<{ id: string }>()) as { id: string };
  for (const i of [15, 16, 17, 18]) await addMember(env, mods.id, ids[i], ownerId, now - 20 * DAY);
  await addGrant(env, accounts.id, { userId: ids[19] }, ownerId, now - 45 * DAY);
  await addGrant(env, await roleOf(env, "owner"), { userId: ids[20] }, ownerId, now - 100 * DAY);
  await addGrant(env, await roleOf(env, "newsletter-viewer"), { userId: ids[21] }, ownerId, now - 10 * DAY);
  for (const [email, name] of [["new.writer@example.com", "Noa Writer"], ["guest.editor@example.com", ""], ["intern@example.com", "Ola Intern"], ["contractor@example.com", "Sam Contractor"]]) {
    await createInvitation(env, { email, name, roleIds: [], groupIds: [desk.id], lang: "en", invitedBy: ownerId }, now - 2 * DAY);
  }
  // Sessions started different ways; some passed a second factor.
  const methods = ["password", "github", "google", "access", "password", "microsoft"];
  const req = new Request("https://example.com/");
  for (let i = 0; i < 18; i++) {
    const { session } = await createSession(env, ids[i], req, 14 * 86_400, now - (i + 1) * 3_600_000 * 5, methods[i % methods.length]);
    await env.AUTH_DB.prepare("UPDATE sessions SET last_used_at = ? WHERE id = ?").bind(new Date(now - i * 1_800_000).toISOString(), session.id).run();
    if (i % 3 === 0) await markSessionFactor(env, session.id, i % 2 ? "passkey" : "totp", now - i * 1_800_000);
  }
  const ci = (await createServiceAccount(env, "ci-deploy", "GitHub Actions after a deploy.", ownerId, now - 80 * DAY)) as ServiceAccount;
  const cron = (await createServiceAccount(env, "nightly-cron", "The nightly retention purge.", ownerId, now - 30 * DAY)) as ServiceAccount;
  await addGrant(env, support.id, { userId: ci.id }, ownerId, now - 80 * DAY);
  await createToken(env, ci.id, { name: "GitHub Actions", days: 90, permissions: ["newsletter:read"], allowSensitive: [] }, ownerId, now - 20 * DAY);
  const old = await createToken(env, ci.id, { name: "Old runner", days: 90, permissions: null, allowSensitive: [] }, ownerId, now - 85 * DAY);
  await revokeToken(env, ci.id, old.token.id, ownerId, now - 21 * DAY);
  await createToken(env, cron.id, { name: "Purge", days: 365, permissions: null, allowSensitive: [] }, ownerId, now - 30 * DAY);
  await createMapping(env, "editorial-team", editors.id, ownerId, now - 25 * DAY);
  // Ninety days of audit events.
  const kinds: [EventKind, string | null][] = [["signin", "password"], ["signin", "github"], ["signin_refused", "google"], ["mfa_passed", null], ["mfa_failed", null], ["grant_added", null], ["member_added", null], ["invitation_sent", null], ["session_revoked", null], ["role_changed", null]];
  for (let d = 0; d < 90; d++) {
    for (let k = 0; k < 2 + (d % 3); k++) {
      const [kind, method] = kinds[(d * 7 + k) % kinds.length];
      const who = ids[(d + k * 5) % ids.length];
      const admin = ["grant_added", "member_added", "invitation_sent", "session_revoked", "role_changed"].includes(kind);
      const detail = admin ? { by: ownerId, ...(kind === "grant_added" ? { role: "support-desk" } : kind === "member_added" ? { group: "Support desk" } : kind === "role_changed" ? { role: "support-desk" } : {}) } : kind === "signin_refused" ? { reason: "not_invited" } : {};
      await emitEvent(env, { kind, userId: kind === "role_changed" ? null : who, method, detail }, now - d * DAY - k * 3_600_000);
    }
  }
}
