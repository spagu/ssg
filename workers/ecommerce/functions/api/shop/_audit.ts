// Who did what. Every mutation from the panel and every state change an order
// goes through leaves a row here.
//
// No personal data in the detail: an order id, an amount, a reason. The log is
// for answering "why is this order refunded" a year later, not for profiling
// buyers (S-30).

import type { Env } from "./_env";
import { newId, nowISO } from "./_lib";

export interface AuditRow {
  id: string;
  actor: string;
  action: string;
  subject: string | null;
  detail_json: string | null;
  created_at: string;
}

export async function audit(
  env: Env,
  actor: string,
  action: string,
  subject?: string | null,
  detail?: Record<string, unknown>,
): Promise<void> {
  await env.SHOP_DB.prepare(
    `INSERT INTO audit_log (id, actor, action, subject, detail_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
  )
    .bind(newId(), actor, action, subject ?? null, detailJson(detail), nowISO())
    .run();
}

/** Serialises the detail, or records that it could not be.
 *
 *  An audit entry is written alongside the action it describes, so a detail
 *  that cannot be stringified — a value carrying a cycle, a BigInt — must not
 *  be able to undo the refund it was recording. */
function detailJson(detail?: Record<string, unknown>): string | null {
  if (!detail) return null;
  try {
    return JSON.stringify(detail);
  } catch {
    return JSON.stringify({ note: "the detail for this entry could not be recorded" });
  }
}
