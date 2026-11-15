// Reading what the platform sends on create and update (checkout.json with
// its `ucp_request` annotations): line items, buyer, context and — only for
// the buyer's country — the fulfillment destinations.
//
// A body the protocol cannot accept (no line items, a quantity that is not a
// whole number, a buyer that is not an object) is a 400 protocol error with
// the JSONPath of the field. A basket that is well formed but cannot be sold
// is not an error here: it becomes `messages` on the session (_ucp-pricing).

import { ucpError, type UcpResult } from "./_ucp-http";
import type { UcpItem } from "./_ucp-session";
import { COUNTRY_RE, CURRENCY_RE, isValidEmail } from "./_lib";

/** The storefront's own caps: twenty lines, ten copies of one product. */
const MAX_LINES = 20;
const MAX_QUANTITY = 10;

const bad = (path: string, content: string): UcpResult => ucpError(400, "invalid_request", content, { path });
const isObject = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === "object" && !Array.isArray(v);
const text = (v: unknown, max: number): string | undefined => (typeof v === "string" && v.trim() ? v.trim().slice(0, max) : undefined);

export function isResult(value: unknown): value is UcpResult {
  return isObject(value) && typeof value.status === "number" && "body" in value;
}

/** `line_items[]`, each `{item: {id}, quantity}` (line_item.json). Two lines
 *  naming one product are one line with more copies, as in the storefront's
 *  basket. A line's own `id` (sent back on update) is not needed: lines are
 *  matched by product. */
export function readLineItems(body: Record<string, unknown>): UcpItem[] | UcpResult {
  const raw = body.line_items;
  if (!Array.isArray(raw) || raw.length === 0 || raw.length > MAX_LINES) {
    return bad("$.line_items", `Between 1 and ${MAX_LINES} line items are required.`);
  }
  const items: UcpItem[] = [];
  for (const [i, entry] of raw.entries()) {
    const id = isObject(entry) && isObject(entry.item) ? text(entry.item.id, 255) : undefined;
    if (!id) return bad(`$.line_items[${i}].item.id`, "Every line item needs item.id.");
    const quantity = (entry as Record<string, unknown>).quantity ?? 1;
    if (!Number.isInteger(quantity) || (quantity as number) < 1 || (quantity as number) > MAX_QUANTITY) {
      return bad(`$.line_items[${i}].quantity`, `The quantity must be a whole number from 1 to ${MAX_QUANTITY}.`);
    }
    const existing = items.find((it) => it.id === id);
    if (existing) existing.quantity = Math.min(MAX_QUANTITY, existing.quantity + (quantity as number));
    else items.push({ id, quantity: quantity as number });
  }
  return items;
}

/** The buyer (buyer.json), kept to the fields the shop uses. Absent is null:
 *  an update replaces the whole session (checkout-rest.md, "Update Buyer
 *  Info"), so a buyer left out of a PUT is a buyer cleared. */
export function readBuyer(value: unknown): Record<string, string> | null | UcpResult {
  if (value === undefined || value === null) return null;
  if (!isObject(value)) return bad("$.buyer", "buyer must be an object.");
  const buyer: Record<string, string> = {};
  const email = text(value.email, 254);
  if (email !== undefined) {
    if (!isValidEmail(email)) return bad("$.buyer.email", "buyer.email is not a valid address.");
    buyer.email = email;
  }
  for (const key of ["first_name", "last_name", "phone_number"]) {
    const v = text(value[key], 120);
    if (v) buyer[key] = v;
  }
  return buyer;
}

/** The buyer's country, the one piece of tax evidence the platform gives for
 *  a download: the selected (or first) fulfillment destination's
 *  `address_country`, else `context.address_country`. Only ISO 3166-1
 *  alpha-2 is read; the protocol also allows alpha-3 or a country name "for
 *  backward compatibility", which a tax rate cannot be looked up by. */
export function readCountry(body: Record<string, unknown>): { country: string | null; source?: "fulfillment" | "context" } {
  const methods = isObject(body.fulfillment) && Array.isArray(body.fulfillment.methods) ? body.fulfillment.methods : [];
  for (const method of methods) {
    if (!isObject(method) || !Array.isArray(method.destinations)) continue;
    const destinations = method.destinations.filter(isObject);
    const chosen = destinations.find((d) => d.id !== undefined && d.id === method.selected_destination_id) ?? destinations[0];
    const code = alpha2(chosen?.address_country);
    if (code) return { country: code, source: "fulfillment" };
  }
  const hint = alpha2(isObject(body.context) ? body.context.address_country : undefined);
  return hint ? { country: hint, source: "context" } : { country: null };
}

export function alpha2(value: unknown): string | null {
  return typeof value === "string" && COUNTRY_RE.test(value.trim()) ? value.trim().toUpperCase() : null;
}

/** context.language, else the first Accept-Language tag (context.json says
 *  the field overrides the header). */
export function readLanguage(request: Request, body: Record<string, unknown>): string | null {
  const given = isObject(body.context) && typeof body.context.language === "string"
    ? body.context.language
    : (request.headers.get("accept-language") ?? "").split(",")[0]!;
  const tag = given.trim();
  return /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$/.test(tag) ? tag.slice(0, 35) : null;
}

/** context.currency, a preference only: the shop decides (checkout.json,
 *  `currency`), falling back to its base currency. */
export function readCurrencyHint(body: Record<string, unknown>): string | null {
  const value = isObject(body.context) ? body.context.currency : undefined;
  return typeof value === "string" && CURRENCY_RE.test(value) ? value.toUpperCase() : null;
}
