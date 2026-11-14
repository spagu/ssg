// Service accounts (GO-141): the data layer. A leading underscore keeps this
// file out of the Pages route table. Why they live in their own table — and
// why that keeps them out of the login, invitations, resets and the owner
// invariant by construction — is in _service-schema.ts.

import { iso } from "./_lib";
import type { Env } from "./_lib";
import { isOwnerViolation, isUnique } from "./_rbac-schema";

export interface ServiceAccount {
  id: string;
  name: string;
  description: string;
  status: "active" | "disabled";
  createdBy: string | null;
  createdAt: string;
  updatedAt: string;
}

interface Row {
  id: string;
  name: string;
  description: string;
  status: "active" | "disabled";
  created_by: string | null;
  created_at: string;
  updated_at: string;
}

// SERVICE_NAME: a slug, like a role's — it appears in scripts and logs.
export const SERVICE_NAME = /^[a-z][a-z0-9-]{0,63}$/;

const shape = (r: Row): ServiceAccount => ({
  id: r.id,
  name: r.name,
  description: r.description,
  status: r.status,
  createdBy: r.created_by,
  createdAt: r.created_at,
  updatedAt: r.updated_at,
});

export async function listServiceAccounts(env: Env): Promise<ServiceAccount[]> {
  const { results } = await env.AUTH_DB.prepare("SELECT * FROM service_accounts ORDER BY name").all<Row>();
  return results.map(shape);
}

export async function getServiceAccount(env: Env, id: string): Promise<ServiceAccount | null> {
  const row = await env.AUTH_DB.prepare("SELECT * FROM service_accounts WHERE id = ?").bind(id).first<Row>();
  return row ? shape(row) : null;
}

// createServiceAccount inserts one account; "name_taken" when the name is used.
export async function createServiceAccount(env: Env, name: string, description: string, by: string, now: number): Promise<ServiceAccount | "name_taken"> {
  const id = crypto.randomUUID();
  try {
    await env.AUTH_DB.prepare(
      "INSERT INTO service_accounts (id, name, description, status, created_by, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?, ?)",
    )
      .bind(id, name, description, by, iso(now), iso(now))
      .run();
  } catch (e) {
    if (isUnique(e)) return "name_taken";
    throw e;
  }
  return (await getServiceAccount(env, id)) as ServiceAccount;
}

export interface ServiceChange {
  name?: string;
  description?: string;
  status?: ServiceAccount["status"];
}

// updateServiceAccount changes what the patch names. Disabling an account
// makes every exchange of its tokens fail at once; access tokens already
// issued live out their few minutes (the README says how many).
export async function updateServiceAccount(env: Env, id: string, patch: ServiceChange, now: number): Promise<ServiceAccount | "not_found" | "name_taken"> {
  const before = await getServiceAccount(env, id);
  if (!before) return "not_found";
  try {
    await env.AUTH_DB.prepare("UPDATE service_accounts SET name = ?, description = ?, status = ?, updated_at = ? WHERE id = ?")
      .bind(patch.name ?? before.name, patch.description ?? before.description, patch.status ?? before.status, iso(now), id)
      .run();
  } catch (e) {
    if (isUnique(e)) return "name_taken";
    throw e;
  }
  return (await getServiceAccount(env, id)) as ServiceAccount;
}

// deleteServiceAccount removes the account with its grants and tokens, in one
// batch. Its grants never hold up the owner invariant, so the trigger only
// refuses when the invariant was already broken — then nothing is deleted.
export async function deleteServiceAccount(env: Env, id: string): Promise<"ok" | "not_found" | "owner_required"> {
  const db = env.AUTH_DB;
  try {
    const results = await db.batch([
      db.prepare("DELETE FROM grants WHERE user_id = ? AND EXISTS (SELECT 1 FROM service_accounts WHERE id = ?)").bind(id, id),
      db.prepare("DELETE FROM service_tokens WHERE service_account_id = ?").bind(id),
      db.prepare("DELETE FROM service_accounts WHERE id = ?").bind(id),
    ]);
    return results[2].meta.changes === 1 ? "ok" : "not_found";
  } catch (e) {
    if (isOwnerViolation(e)) return "owner_required";
    throw e;
  }
}
