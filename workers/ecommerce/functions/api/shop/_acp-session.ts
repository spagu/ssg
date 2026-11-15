// An ACP checkout session as a row, and the only ways it may change. The
// storage itself — compare-and-swap on `version`, expiry, why D1 and not a
// Durable Object — is shared with UCP in _agent-session.ts; this file holds
// ACP's status words and its id format.

import type { Env } from "./_env";
import { sessionStore, type AgentSessionPatch, type AgentSessionRow } from "./_agent-session";
import { newId } from "./_lib";

export { cancelPendingOrder, parseJson } from "./_agent-session";

export type AcpStatus =
  | "not_ready_for_payment"
  | "ready_for_payment"
  | "complete_in_progress"
  | "completed"
  | "canceled"
  | "expired";

/** States nothing may change any more. */
export const FINAL: AcpStatus[] = ["completed", "canceled", "expired"];

/** One requested item: the feed's item id (the shop's SKU) and a quantity. */
export interface AcpItem {
  id: string;
  quantity: number;
  /** What the agent believed the unit price was, if it said. */
  unit_amount?: number;
}

export type SessionRow = AgentSessionRow<AcpStatus>;

/** The columns a write may set. Names come from this list, never from input. */
export type SessionPatch = AgentSessionPatch<AcpStatus>;

/** How long an agent has to finish a session. */
export const SESSION_TTL_MS = 6 * 3600 * 1000;

export function newSessionId(): string {
  return `cs_${newId().replaceAll("-", "")}`;
}

const store = sessionStore<AcpStatus>("acp_sessions", {
  expired: "expired",
  // A completion in progress is never expired under its own feet: it either
  // finishes or is recovered (_acp-complete).
  isOpen: (status) => !FINAL.includes(status) && status !== "complete_in_progress",
});

export const insertSession = (env: Env, row: SessionRow): Promise<void> => store.insert(env, row);

/** Writes `patch` only if nobody else wrote since `row` was read. False means
 *  the caller lost the race and must re-read. */
export const casUpdate = (env: Env, row: SessionRow, patch: SessionPatch): Promise<boolean> => store.casUpdate(env, row, patch);

/** The session, with an overdue one moved to `expired` on the way. */
export const loadSession = (env: Env, id: string): Promise<SessionRow | null> => store.load(env, id);

/** Links the order a completion wrote (_acp-complete). */
export const attachOrder = (env: Env, row: SessionRow, orderId: string, orderKey: string): Promise<void> =>
  store.attachOrder(env, row, orderId, orderKey);
