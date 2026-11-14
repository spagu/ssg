// Invented data for the local preview (dev-server.mjs): an owner, an editor,
// roles, a group, an open invitation and a reset link. Only example.com
// addresses; the passwords are printed by the dev server.
import { addGrant } from "../functions/api/auth/_grants.ts";
import { addMember, createGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { createInvitation } from "../functions/api/auth/_invites.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { hashPassword } from "../functions/api/auth/_password.ts";
import { createReset } from "../functions/api/auth/_resets.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { ensureSchema } from "../functions/api/auth/_schema.ts";
import { createUser } from "../functions/api/auth/_users.ts";

export const DEMO_PASSWORD = "demo password 1";

// The panels the preview pretends to have: what AUTH_KNOWN_PERMISSIONS would
// hold on a site running the newsletter and the comments.
export const DEMO_KNOWN = JSON.stringify([
  {
    id: "newsletter-ui",
    title: { en: "Newsletter", pl: "Newsletter", hi: "न्यूज़लेटर", de: "Newsletter", fr: "Newsletter" },
    panel: "/newsletter-admin/",
    permissions: [
      { name: "newsletter:read", description: "View subscribers, campaigns and statistics." },
      { name: "newsletter:edit", description: "Write and edit campaign drafts." },
      { name: "newsletter:send", description: "Send a campaign to the list." },
    ],
  },
  { id: "comments", title: { en: "Comments", pl: "Komentarze", hi: "टिप्पणियाँ", de: "Kommentare", fr: "Commentaires" }, permissions: [{ name: "comments:read", description: "See the moderation queue." }, { name: "comments:moderate", description: "Approve comments and mark spam." }] },
]);

export async function seedDemo(env: Env, now: number): Promise<{ inviteToken: string; resetToken: string }> {
  await ensureSchema(env);
  const hash = await hashPassword(DEMO_PASSWORD, 10_000);
  const owner = (await createUser(env, { email: "owner@example.com", name: "Olga Owner", passwordHash: hash }, now - 40 * 86_400_000)) as string;
  const editor = (await createUser(env, { email: "editor@example.com", name: "Eda Editor", passwordHash: hash }, now - 9 * 86_400_000)) as string;
  const ownerRole = (await env.AUTH_DB.prepare("SELECT id FROM roles WHERE name = 'owner'").first<{ id: string }>()) as { id: string };
  await addGrant(env, ownerRole.id, { userId: owner }, "demo", now);
  const editorRole = (await env.AUTH_DB.prepare("SELECT id FROM roles WHERE name = 'newsletter-editor'").first<{ id: string }>()) as { id: string };
  await addGrant(env, editorRole.id, { userId: editor }, owner, now);
  const mods = (await createGroup(env, "Moderators", "People who look after the comments.", now)) as Group;
  const modRole = (await env.AUTH_DB.prepare("SELECT id FROM roles WHERE name = 'comments-moderator'").first<{ id: string }>()) as { id: string };
  await addGrant(env, modRole.id, { groupId: mods.id }, owner, now);
  await addMember(env, mods.id, editor, owner, now);
  const inviter = (await createRole(env, { name: "inviter", description: "Invites new people.", permissions: ["auth:invite"] }, now)) as Role;
  await addGrant(env, inviter.id, { userId: editor }, owner, now);
  const { token: inviteToken } = await createInvitation(env, { email: "new.person@example.com", name: "Noor", roleIds: [editorRole.id], groupIds: [mods.id], lang: "en", invitedBy: owner }, now);
  const { token: resetToken } = await createReset(env, editor, owner, now);
  return { inviteToken, resetToken };
}
