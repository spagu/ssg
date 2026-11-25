// The campaign state machine: every allowed move in one table. Anything not in
// it is refused (the API answers 409 invalid_transition).
//
//   draft ──schedule──▶ scheduled ──(engine)──▶ sending ──(engine)──▶ sent
//     ▲                   │  │                   │  ▲
//     └────unschedule─────┘  │                 pause resume
//                            │                   ▼  │
//                            │                  paused
//                            └──cancel──▶ cancelled ◀──cancel── sending, paused
//
// Only `draft` is editable. `sent` and `cancelled` are final. The API moves
// draft ⇄ scheduled, sending ⇄ paused and anything live → cancelled; the send
// engine moves scheduled → sending and sending → sent.

import type { Env } from "./lib";

export const STATUSES = ["draft", "scheduled", "sending", "paused", "cancelled", "sent"] as const;
export type Status = (typeof STATUSES)[number];

export const TRANSITIONS: Record<Status, readonly Status[]> = {
  draft: ["scheduled"],
  scheduled: ["draft", "sending", "cancelled"],
  sending: ["paused", "cancelled", "sent"],
  paused: ["sending", "cancelled"],
  cancelled: [],
  sent: [],
};

export const isStatus = (s: unknown): s is Status => typeof s === "string" && (STATUSES as readonly string[]).includes(s);

export function canTransition(from: string, to: string): boolean {
  return isStatus(from) && isStatus(to) && TRANSITIONS[from].includes(to);
}

// The timestamps a move sets, so every caller records them the same way:
// scheduling sets scheduled_at, going back to draft clears it, the first move
// to sending stamps started_at, and the final states stamp finished_at.
function stamps(to: Status): string {
  switch (to) {
    case "scheduled":
      return ", scheduled_at = ?4";
    case "draft":
      return ", scheduled_at = NULL";
    case "sending":
      return ", started_at = COALESCE(started_at, ?3)";
    case "sent":
    case "cancelled":
      return ", finished_at = ?3";
    default:
      return "";
  }
}

// transition moves campaign `id` from `from` to `to` and reports whether THIS
// call made the move. It is one UPDATE guarded by "AND status = from", so of
// two concurrent callers (two cron ticks, an admin clicking twice) exactly one
// wins; the other gets false and must not act. A move the table does not allow
// returns false without touching the database. `scheduledAt` (ISO 8601,
// default now) is used only for → scheduled. `unchangedSince` (#448) adds
// "AND updated_at = it": a move decided on content checked a while ago (the
// image gate probes the network for seconds) fails if the draft was edited
// in the meantime, instead of scheduling content nobody checked.
export async function transition(env: Env, id: string, from: Status, to: Status, scheduledAt?: string, unchangedSince?: string): Promise<boolean> {
  if (!canTransition(from, to)) return false;
  const now = new Date().toISOString();
  const binds = [to, from, now, scheduledAt ?? now, id];
  if (unchangedSince !== undefined) binds.push(unchangedSince);
  const r = await env.NEWSLETTER_DB.prepare(
    `UPDATE campaigns SET status = ?1, updated_at = ?3${stamps(to)} WHERE id = ?5 AND status = ?2${unchangedSince === undefined ? "" : " AND updated_at = ?6"}`,
  )
    .bind(...binds)
    .run();
  return r.meta.changes === 1;
}
