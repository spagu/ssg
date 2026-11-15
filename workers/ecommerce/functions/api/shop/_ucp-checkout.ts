// Create, get, update and cancel — the UCP operations that move no money
// (checkout.md, "Operations"). Completion is _ucp-complete.ts.

import type { AgentOpInput } from "./_agent-common";
import { getSettingString } from "./_settings";
import { errorMessage, errorResponse, notFound, ucpError, type UcpMessage, type UcpResult } from "./_ucp-http";
import { isResult, readBuyer, readCountry, readCurrencyHint, readLanguage, readLineItems } from "./_ucp-input";
import { isItemError, priceUcp, type UcpPriced } from "./_ucp-pricing";
import { renderSession } from "./_ucp-render";
import {
  cancelPendingOrder,
  casUpdate,
  FINAL,
  insertSession,
  loadSession,
  newSessionId,
  parseJson,
  SESSION_TTL_MS,
  type UcpDetails,
  type UcpSessionRow,
  type UcpStatus,
} from "./_ucp-session";

/** How many times an update re-reads and retries after losing a race. */
const UPDATE_ATTEMPTS = 3;

/** A session that can no longer be changed this way: a protocol conflict. */
export function stateConflict(row: UcpSessionRow): UcpResult {
  const result = ucpError(409, "invalid_state", `The checkout session is ${row.status}.`);
  return row.status === "complete_in_progress" ? { ...result, headers: { "retry-after": "2" } } : result;
}

/** The buyer's email is the one thing a download cannot be delivered
 *  without; Google collects it on the "full hydration" update. */
export function withBuyer(state: UcpPriced, buyer: Record<string, string> | null): { state: UcpPriced; status: UcpStatus } {
  const messages: UcpMessage[] = [...state.messages];
  if (!buyer?.email) messages.push(errorMessage("missing_buyer_info", "The buyer's email is needed to deliver the download.", "recoverable", "$.buyer.email"));
  const ready = state.ready && Boolean(buyer?.email);
  return { state: { ...state, ready, messages }, status: ready ? "ready_for_complete" : "incomplete" };
}

export async function createSession({ request, env, body, origin }: AgentOpInput): Promise<UcpResult> {
  const items = readLineItems(body);
  if (isResult(items)) return items;
  const buyer = readBuyer(body.buyer);
  if (isResult(buyer)) return buyer;
  const place = readCountry(body);

  // context.currency is a preference; the shop decides, so a currency it does
  // not sell in falls back to its base one rather than failing the basket.
  const base = (await getSettingString(env, "currency.base")).toUpperCase();
  const hint = readCurrencyHint(body);
  let currency = hint ?? base;
  let priced = await priceUcp(env, { items, currency, country: place.country, origin });
  if (priced.error?.error === "price_not_available" && currency !== base) {
    currency = base;
    priced = await priceUcp(env, { items, currency, country: place.country, origin });
  }
  // Nothing in the basket can be sold: no session is created (checkout.md,
  // "Error Handling": unrecoverable, ucp.status "error").
  if (priced.error && isItemError(priced.error)) {
    return errorResponse(priced.state.messages.map((m) => ({ ...m, severity: "unrecoverable" })), `${origin}/`);
  }

  const { state, status } = withBuyer(priced.state, buyer);
  const details: UcpDetails = { ...(place.source ? { countrySource: place.source } : {}) };
  const now = Date.now();
  const row: UcpSessionRow = {
    id: newSessionId(),
    status,
    version: 1,
    currency,
    items_json: JSON.stringify(items),
    buyer_json: buyer ? JSON.stringify(buyer) : null,
    details_json: JSON.stringify(details),
    country: place.country,
    locale: readLanguage(request, body),
    priced_json: JSON.stringify(state),
    order_id: null,
    order_key: null,
    created_at: new Date(now).toISOString(),
    updated_at: new Date(now).toISOString(),
    expires_at: new Date(now + SESSION_TTL_MS).toISOString(),
  };
  await insertSession(env, row);
  return { status: 201, body: await renderSession(env, row, origin) };
}

export async function getSession({ env, id, origin }: AgentOpInput): Promise<UcpResult> {
  const row = await loadSession(env, id);
  if (!row) return notFound(origin);
  return { status: 200, body: await renderSession(env, row, origin) };
}

/** PUT replaces the session (checkout-rest.md: "Each PUT replaces the entire
 *  session"): line items are required, and a buyer or a destination left
 *  out is cleared. The read-price-write is retried when another write landed
 *  in between, so no update is lost and none overwrites another. */
export async function updateSession({ request, env, body, id, origin }: AgentOpInput): Promise<UcpResult> {
  const items = readLineItems(body);
  if (isResult(items)) return items;
  const buyer = readBuyer(body.buyer);
  if (isResult(buyer)) return buyer;
  const place = readCountry(body);

  for (let attempt = 0; attempt < UPDATE_ATTEMPTS; attempt++) {
    const row = await loadSession(env, id);
    if (!row) return notFound(origin);
    if (FINAL.includes(row.status) || row.status === "complete_in_progress") return stateConflict(row);

    const priced = await priceUcp(env, { items, currency: row.currency, country: place.country, origin });
    const { state, status } = withBuyer(priced.state, buyer);
    const details = parseJson<UcpDetails>(row.details_json, {});
    const written = await casUpdate(env, row, {
      status,
      items_json: JSON.stringify(items),
      buyer_json: buyer ? JSON.stringify(buyer) : null,
      details_json: JSON.stringify({ ...details, countrySource: place.source, language: readLanguage(request, body) ?? undefined }),
      country: place.country,
      priced_json: JSON.stringify(state),
    });
    if (written) return { status: 200, body: await renderSession(env, (await loadSession(env, id))!, origin) };
  }
  return { ...ucpError(409, "concurrent_update", "The session kept changing; retry the update."), headers: { "retry-after": "1" } };
}

/** Cancelling a canceled session answers it as it is; a completed one, or
 *  one being paid for right now, cannot be canceled. */
export async function cancelSession({ env, id, origin }: AgentOpInput): Promise<UcpResult> {
  const row = await loadSession(env, id);
  if (!row) return notFound(origin);
  if (row.status === "canceled") return { status: 200, body: await renderSession(env, row, origin) };
  if (row.status === "completed" || row.status === "complete_in_progress" || !(await casUpdate(env, row, { status: "canceled" }))) {
    return stateConflict((await loadSession(env, id))!);
  }
  await cancelPendingOrder(env, row.order_id);
  return { status: 200, body: await renderSession(env, (await loadSession(env, id))!, origin) };
}
