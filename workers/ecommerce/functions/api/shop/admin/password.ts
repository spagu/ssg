// POST /api/shop/admin/password — change your own password.
//
// Separate from the users endpoints on purpose. Those are an owner managing
// other people; this is anyone managing themselves, and it asks for the current
// password first — which the owner-driven reset does not, because an owner
// resetting a colleague's password does not know it.
//
// Asking for the current one is what stops a borrowed session becoming a
// permanent one: someone who sits down at an unlocked laptop can read the
// orders, and should not also be able to lock the owner out of the shop.

import { authMode, type Env } from "../_env";
import { audit } from "../_audit";
import { endAllSessions, hashPassword, verifyPassword } from "../_auth";
import { fail, json, readJson } from "../_lib";
import type { AdminUserRow } from "../_types";
import type { AdminData } from "./_middleware";
import { MIN_PASSWORD } from "./users/index";

interface Body {
  currentPassword?: string;
  newPassword?: string;
}

export const onRequestPost: PagesFunction<Env, string, AdminData> = async ({ request, env, data }) => {
  if (authMode(env) !== "jwt") {
    // Under Cloudflare Access the password is the identity provider's business,
    // and this shop has none to change.
    return fail(request, "not_found", "This shop signs in through Cloudflare Access.", 404);
  }

  const body = await readJson<Body>(request);
  if (!body) return fail(request, "invalid_body", "The request body could not be read.", 400);

  const current = typeof body.currentPassword === "string" ? body.currentPassword : "";
  const next = typeof body.newPassword === "string" ? body.newPassword : "";

  if (next.length < MIN_PASSWORD) {
    return fail(
      request,
      "weak_password",
      `A password needs at least ${MIN_PASSWORD} characters. A short sentence is easier to remember and harder to guess than a word with symbols in it.`,
      422,
    );
  }
  if (next === current) {
    return fail(request, "unchanged", "That is the password you are already using.", 422);
  }

  const user = await env.SHOP_DB.prepare(`SELECT * FROM admin_users WHERE id = ?`)
    .bind(data.admin.sub)
    .first<AdminUserRow>();
  if (!user) return fail(request, "not_found", "No such account.", 404);

  if (!(await verifyPassword(current, user.pass_hash))) {
    await audit(env, `admin:${user.id}`, "admin.password_failed", user.id, { email: user.email_lc });
    return fail(request, "invalid_credentials", "That is not your current password.", 403);
  }

  await env.SHOP_DB.prepare(`UPDATE admin_users SET pass_hash = ? WHERE id = ?`)
    .bind(await hashPassword(next), user.id)
    .run();

  // Every session, including this one. Changing a password because you think
  // somebody else has it is pointless if their session survives it; the panel
  // signs you back in.
  await endAllSessions(env, user.id);
  await audit(env, `admin:${user.id}`, "admin.password_changed", user.id, { email: user.email_lc });

  return json({
    ok: true,
    message: "Password changed. Every session has been signed out, including this one.",
  });
};
