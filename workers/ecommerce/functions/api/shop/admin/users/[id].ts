// PATCH  /api/shop/admin/users/:id — name, role, suspension, password
// DELETE /api/shop/admin/users/:id — remove an account
//
// The rules here are all one rule: a shop must not be able to lock its owner
// out of it. Every refusal below is a way that could otherwise happen — the
// last owner demoting themselves, suspending themselves, or being deleted by
// somebody who then loses their own password.

import type { Env } from "../../_env";
import { audit } from "../../_audit";
import { endAllSessions, hashPassword, requireOwner } from "../../_auth";
import { fail, json, nowISO, readJson, str } from "../../_lib";
import type { AdminUserRow } from "../../_types";
import type { AdminData } from "../_middleware";
import { MIN_PASSWORD, publicUser } from "./index";

/** How many owners could still sign in if this one stopped being able to. */
async function otherActiveOwners(env: Env, exceptId: string): Promise<number> {
  const row = await env.SHOP_DB.prepare(
    `SELECT COUNT(*) AS n FROM admin_users WHERE role = 'owner' AND disabled = 0 AND id != ?`,
  )
    .bind(exceptId)
    .first<{ n: number }>();
  return row?.n ?? 0;
}

interface PatchBody {
  name?: string;
  role?: string;
  disabled?: boolean;
  password?: string;
}

export const onRequestPatch: PagesFunction<Env, string, AdminData> = async ({ request, env, params, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;

  const id = String(params.id ?? "");
  const user = await env.SHOP_DB.prepare(`SELECT * FROM admin_users WHERE id = ?`)
    .bind(id)
    .first<AdminUserRow>();
  if (!user) return fail(request, "not_found", "No such account.", 404);

  const body = await readJson<PatchBody>(request);
  if (!body) return fail(request, "invalid_body", "The request body could not be read.", 400);

  const role = body.role === undefined ? user.role : body.role === "owner" ? "owner" : "staff";
  const disabled = body.disabled === undefined ? user.disabled === 1 : Boolean(body.disabled);

  // The shop must keep at least one owner who can sign in.
  const losingThisOwner = user.role === "owner" && (role !== "owner" || disabled);
  if (losingThisOwner && (await otherActiveOwners(env, user.id)) === 0) {
    return fail(
      request,
      "last_owner",
      "This is the only owner who can sign in. Make someone else an owner first.",
      409,
    );
  }

  const changed: string[] = [];
  if (body.name !== undefined) changed.push("name");
  if (role !== user.role) changed.push("role");
  if (disabled !== (user.disabled === 1)) changed.push(disabled ? "suspended" : "restored");

  await env.SHOP_DB.prepare(`UPDATE admin_users SET name = ?, role = ?, disabled = ? WHERE id = ?`)
    .bind(
      body.name === undefined ? user.name : str(body.name, 120) || null,
      role,
      disabled ? 1 : 0,
      id,
    )
    .run();

  // A password change, and a suspension, both end that person's sessions: a
  // password you have changed must not leave a session open somewhere, and a
  // suspension that waits for a token to expire is not a suspension.
  if (typeof body.password === "string" && body.password.length > 0) {
    if (body.password.length < MIN_PASSWORD) {
      return fail(request, "weak_password", `A password needs at least ${MIN_PASSWORD} characters.`, 422);
    }
    await env.SHOP_DB.prepare(`UPDATE admin_users SET pass_hash = ? WHERE id = ?`)
      .bind(await hashPassword(body.password), id)
      .run();
    await endAllSessions(env, id);
    changed.push("password");
  } else if (disabled) {
    await endAllSessions(env, id);
  }

  await audit(env, `admin:${data.admin.sub}`, "admin.update", id, {
    email: user.email_lc,
    changed,
  });

  const updated = await env.SHOP_DB.prepare(`SELECT * FROM admin_users WHERE id = ?`)
    .bind(id)
    .first<AdminUserRow>();
  return json({ user: publicUser(updated!) });
};

export const onRequestDelete: PagesFunction<Env, string, AdminData> = async ({ request, env, params, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;

  const id = String(params.id ?? "");
  const user = await env.SHOP_DB.prepare(`SELECT * FROM admin_users WHERE id = ?`)
    .bind(id)
    .first<AdminUserRow>();
  if (!user) return fail(request, "not_found", "No such account.", 404);

  if (user.role === "owner" && (await otherActiveOwners(env, user.id)) === 0) {
    return fail(request, "last_owner", "This is the only owner. The shop would have nobody who can get in.", 409);
  }

  // Deleting yourself is allowed — an owner handing the shop over should not
  // have to keep an account they no longer want — but the session goes with it,
  // so it cannot be done by accident and then half-undone.
  await env.SHOP_DB.batch([
    env.SHOP_DB.prepare(`DELETE FROM admin_sessions WHERE admin_id = ?`).bind(id),
    env.SHOP_DB.prepare(`DELETE FROM admin_users WHERE id = ?`).bind(id),
  ]);

  // The audit entries this person left behind stay, and still name them. That
  // is the point of a log: "who did this" has to keep having an answer after
  // they have gone.
  await audit(env, `admin:${data.admin.sub}`, "admin.delete", id, {
    email: user.email_lc,
    role: user.role,
    self: id === data.admin.sub,
  });

  return json({ ok: true, deletedAt: nowISO() });
};
