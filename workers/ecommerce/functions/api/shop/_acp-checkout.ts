// Create, update, retrieve and cancel — the session endpoints that move no
// money. Completion is _acp-complete.ts.

import type { Env } from "./_env";
import { acpError, type AcpResult } from "./_acp-http";
import { isResult, readBuyer, readCurrency, readDetails, readItems, readSelectedOptions } from "./_acp-input";
import { priceSession } from "./_acp-pricing";
import { renderSession } from "./_acp-render";
import {
  cancelPendingOrder,
  casUpdate,
  FINAL,
  insertSession,
  loadSession,
  newSessionId,
  parseJson,
  SESSION_TTL_MS,
  type AcpItem,
  type SessionRow,
} from "./_acp-session";

export interface OpInput {
  request: Request;
  env: Env;
  body: Record<string, unknown>;
  id: string;
  origin: string;
  waitUntil: (p: Promise<unknown>) => void;
}

/** How many times an update re-reads and retries after losing a race. */
const UPDATE_ATTEMPTS = 3;

export const notFound = (): AcpResult => acpError(404, "invalid_request", "not_found", "No such checkout session.");

/** A session that can no longer be changed this way. */
export function stateConflict(row: SessionRow): AcpResult {
  const result = acpError(409, "invalid_request", "invalid_session_state", `The checkout session is ${row.status}.`);
  return row.status === "complete_in_progress" ? { ...result, headers: { "retry-after": "2" } } : result;
}

function readLocale(request: Request, body: Record<string, unknown>): string | null {
  const given = typeof body.locale === "string" ? body.locale : (request.headers.get("accept-language") ?? "").split(",")[0]!;
  const tag = given.trim();
  return /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$/.test(tag) ? tag.slice(0, 35) : null;
}

export async function createSession({ request, env, body, origin }: OpInput): Promise<AcpResult> {
  const items = readItems(body);
  if (isResult(items)) return items;
  const buyer = body.buyer === undefined ? null : readBuyer(body.buyer);
  if (isResult(buyer)) return buyer;
  const place = body.fulfillment_details === undefined ? { details: null, country: null } : readDetails(body.fulfillment_details);
  if (isResult(place)) return place;
  const currency = await readCurrency(env, body.currency);
  if (isResult(currency)) return currency;

  const { state } = await priceSession(env, { items, currency, country: place.country, origin });
  const now = Date.now();
  const row: SessionRow = {
    id: newSessionId(),
    status: state.ready ? "ready_for_payment" : "not_ready_for_payment",
    version: 1,
    currency,
    items_json: JSON.stringify(items),
    buyer_json: buyer ? JSON.stringify(buyer) : null,
    details_json: place.details ? JSON.stringify(place.details) : null,
    country: place.country,
    locale: readLocale(request, body),
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

export async function retrieveSession({ env, id, origin }: OpInput): Promise<AcpResult> {
  const row = await loadSession(env, id);
  if (!row) return notFound();
  return { status: 200, body: await renderSession(env, row, origin) };
}

/** Applies the changes in `body` over the stored session. An absent field is
 *  left alone and `null` clears it (RFC §6.2). The whole read-price-write is
 *  retried when another write landed in between, so no update is lost and
 *  none overwrites another. */
export async function updateSession({ env, body, id, origin }: OpInput): Promise<AcpResult> {
  const options = readSelectedOptions(body.selected_fulfillment_options);
  if (options) return options;
  const items = body.line_items === undefined && body.items === undefined ? null : readItems(body);
  if (isResult(items)) return items;
  const buyer = body.buyer === undefined ? undefined : readBuyer(body.buyer);
  if (isResult(buyer)) return buyer;
  const place = body.fulfillment_details === undefined ? undefined : readDetails(body.fulfillment_details);
  if (isResult(place)) return place;

  for (let attempt = 0; attempt < UPDATE_ATTEMPTS; attempt++) {
    const row = await loadSession(env, id);
    if (!row) return notFound();
    if (FINAL.includes(row.status) || row.status === "complete_in_progress") return stateConflict(row);

    const nextItems = items ?? parseJson<AcpItem[]>(row.items_json, []);
    const country = place === undefined ? row.country : place.country;
    const { state } = await priceSession(env, { items: nextItems, currency: row.currency, country, origin });
    const written = await casUpdate(env, row, {
      status: state.ready ? "ready_for_payment" : "not_ready_for_payment",
      items_json: JSON.stringify(nextItems),
      buyer_json: buyer === undefined ? row.buyer_json : buyer && JSON.stringify(buyer),
      details_json: place === undefined ? row.details_json : place.details && JSON.stringify(place.details),
      country,
      priced_json: JSON.stringify(state),
    });
    if (written) return { status: 200, body: await renderSession(env, (await loadSession(env, id))!, origin) };
  }
  return { ...acpError(409, "invalid_request", "concurrent_update", "The session kept changing; retry the update."), headers: { "retry-after": "1" } };
}

/** 405 once completed, canceled or expired, as the spec says for cancel. */
export async function cancelSession({ env, id, origin }: OpInput): Promise<AcpResult> {
  const row = await loadSession(env, id);
  if (!row) return notFound();
  if (FINAL.includes(row.status)) {
    return acpError(405, "invalid_request", "not_cancelable", `The checkout session is already ${row.status}.`);
  }
  if (row.status === "complete_in_progress" || !(await casUpdate(env, row, { status: "canceled" }))) {
    return stateConflict((await loadSession(env, id))!);
  }
  await cancelPendingOrder(env, row.order_id);
  return { status: 200, body: await renderSession(env, (await loadSession(env, id))!, origin) };
}
