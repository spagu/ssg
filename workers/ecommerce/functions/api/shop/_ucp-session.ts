// A UCP checkout session as a row (ucp_sessions, migration 6). Storage,
// compare-and-swap and expiry are shared with ACP (_agent-session.ts); this
// file holds UCP's status words (checkout.md, "Status Values") and what the
// row's JSON columns carry.

import type { Env } from "./_env";
import { sessionStore, type AgentSessionPatch, type AgentSessionRow } from "./_agent-session";
import { newId } from "./_lib";

export { cancelPendingOrder, parseJson } from "./_agent-session";

/** `requires_escalation` is in the protocol but never set here: everything
 *  this shop needs can be given through the API. */
export type UcpStatus = "incomplete" | "ready_for_complete" | "complete_in_progress" | "completed" | "canceled";

/** States nothing may change any more. An expired session is `canceled`: the
 *  protocol has no separate word for it ("Checkout session is invalid or
 *  expired"). */
export const FINAL: UcpStatus[] = ["completed", "canceled"];

/** One requested item: the feed's item id (the shop's SKU) and a quantity. */
export interface UcpItem {
  id: string;
  quantity: number;
}

/** What details_json holds: the inputs that are not items or buyer. */
export interface UcpDetails {
  /** BCP 47 tag from context.language or Accept-Language. */
  language?: string;
  /** Where the country came from, for the order's tax evidence. */
  countrySource?: "fulfillment" | "context" | "billing";
  /** The instrument Google sent on complete, without its credential — echoed
   *  in later reads as the protocol's examples do. */
  instrument?: Record<string, unknown>;
}

export type UcpSessionRow = AgentSessionRow<UcpStatus>;
export type UcpSessionPatch = AgentSessionPatch<UcpStatus>;

/** The protocol's default lifetime (checkout.json, `expires_at`). */
export const SESSION_TTL_MS = 6 * 3600 * 1000;

export function newSessionId(): string {
  return `chk_${newId().replaceAll("-", "")}`;
}

/** Line item ids are positions, stable for a given list of items: the
 *  platform sends them back on update, and the order uses the same ones. */
export const lineId = (index: number): string => `li_${index + 1}`;

const store = sessionStore<UcpStatus>("ucp_sessions", {
  expired: "canceled",
  isOpen: (status) => !FINAL.includes(status) && status !== "complete_in_progress",
});

export const insertSession = (env: Env, row: UcpSessionRow): Promise<void> => store.insert(env, row);
export const casUpdate = (env: Env, row: UcpSessionRow, patch: UcpSessionPatch): Promise<boolean> =>
  store.casUpdate(env, row, patch);
export const loadSession = (env: Env, id: string): Promise<UcpSessionRow | null> => store.load(env, id);
export const attachOrder = (env: Env, row: UcpSessionRow, orderId: string, orderKey: string): Promise<void> =>
  store.attachOrder(env, row, orderId, orderKey);

/** The session that made an order, for its order events. */
export async function sessionForOrder(env: Env, orderId: string): Promise<UcpSessionRow | null> {
  return env.SHOP_DB.prepare(`SELECT * FROM ucp_sessions WHERE order_id = ?`).bind(orderId).first<UcpSessionRow>();
}
