// GET  /api/shop/admin/users — who can get in
// POST /api/shop/admin/users — add someone
//
// Two roles, and the difference between them is the answer to one question:
// can this person change what the shop is or what it charges?
//
//   owner  everything, including money, settings, VAT rates and these accounts
//   staff  the day's work — reading orders, resending a download, taking a note
//
// Staff exists so that the person who answers the email does not need the
// credentials that could change the seller's VAT number. Every endpoint that
// could is already behind requireOwner; this screen is where the distinction
// becomes visible.
//
// A password hash never leaves this file's queries, and the panel has no screen
// that shows one. Reading somebody's hash is the first step to cracking it
// offline at leisure.

import type { Env } from "../../_env";
import { audit } from "../../_audit";
import { hashPassword, requireOwner } from "../../_auth";
import { fail, isValidEmail, json, newId, nowISO, readJson, str } from "../../_lib";
import type { AdminUserRow } from "../../_types";
import type { AdminData } from "../_middleware";

/** Long enough that a list of common passwords is not a threat, short enough
 *  that a person will use a passphrase rather than write one down. */
export const MIN_PASSWORD = 12;

/** What a user looks like from outside: everything except the hash. */
export function publicUser(row: AdminUserRow): Record<string, unknown> {
  return {
    id: row.id,
    email: row.email_lc,
    name: row.name,
    role: row.role,
    disabled: row.disabled === 1,
    createdAt: row.created_at,
    lastLoginAt: row.last_login_at,
  };
}

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env, data }) => {
  // Staff can see that colleagues exist — the audit log names them anyway — but
  // only an owner can change anything here.
  const { results } = await env.SHOP_DB.prepare(
    `SELECT * FROM admin_users ORDER BY role, email_lc`,
  ).all<AdminUserRow>();
  const users = results ?? [];

  return json({
    users: users.map(publicUser),
    you: data.admin.sub,
    // An owner who is the only one has nobody to unlock them if they lose their
    // password, which is worth saying before it happens rather than after.
    owners: users.filter((u) => u.role === "owner" && u.disabled !== 1).length,
    canManage: data.admin.role === "owner",
    minPasswordLength: MIN_PASSWORD,
    requestOrigin: new URL(request.url).origin,
  });
};

interface NewUserBody {
  email?: string;
  name?: string;
  password?: string;
  role?: string;
}

export const onRequestPost: PagesFunction<Env, string, AdminData> = async ({ request, env, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;

  const body = await readJson<NewUserBody>(request);
  if (!body) return fail(request, "invalid_body", "The request body could not be read.", 400);

  const email = str(body.email, 254).toLowerCase();
  if (!isValidEmail(email)) return fail(request, "invalid_email", "A valid email address is required.", 422);

  const password = typeof body.password === "string" ? body.password : "";
  if (password.length < MIN_PASSWORD) {
    return fail(
      request,
      "weak_password",
      `A password needs at least ${MIN_PASSWORD} characters. A short sentence is easier to remember and harder to guess than a word with symbols in it.`,
      422,
    );
  }

  const role = body.role === "owner" ? "owner" : "staff";

  const existing = await env.SHOP_DB.prepare(`SELECT id FROM admin_users WHERE email_lc = ?`)
    .bind(email)
    .first<{ id: string }>();
  if (existing) return fail(request, "email_taken", "Someone already signs in with that address.", 409);

  const id = newId();
  await env.SHOP_DB.prepare(
    `INSERT INTO admin_users (id, email_lc, pass_hash, role, created_at, last_login_at, name, disabled)
     VALUES (?, ?, ?, ?, ?, NULL, ?, 0)`,
  )
    .bind(id, email, await hashPassword(password), role, nowISO(), str(body.name, 120) || null)
    .run();

  // The password is not in the entry, and neither is its length: an audit log
  // is read by more people than it is written by.
  await audit(env, `admin:${data.admin.sub}`, "admin.create", id, { email, role });

  const created = await env.SHOP_DB.prepare(`SELECT * FROM admin_users WHERE id = ?`)
    .bind(id)
    .first<AdminUserRow>();
  return json({ user: publicUser(created!) }, 201);
};
