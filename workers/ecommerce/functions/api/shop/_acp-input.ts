// Reading what an agent sends: items, buyer, fulfilment details, currency,
// payment data. Malformed input is a 400 in the protocol's Error shape with
// the JSONPath of the offending field; input that is well formed but cannot be
// sold (an unknown product, a country the shop does not serve) is not an
// error here — it becomes a message on the session (_acp-pricing).

import type { Env } from "./_env";
import { acpError, type AcpResult } from "./_acp-http";
import type { AcpItem } from "./_acp-session";
import { COUNTRY_RE, CURRENCY_RE, isValidEmail } from "./_lib";
import { getSettingString } from "./_settings";

/** The storefront's own caps: twenty lines, ten copies of one product. */
const MAX_LINES = 20;
const MAX_QUANTITY = 10;

const bad = (param: string, message: string): AcpResult => acpError(400, "invalid_request", "invalid", message, param);
const isObject = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === "object" && !Array.isArray(v);
const text = (v: unknown, max: number): string | undefined => (typeof v === "string" && v.trim() ? v.trim().slice(0, max) : undefined);

export function isResult(value: unknown): value is AcpResult {
  return isObject(value) && typeof value.status === "number" && "body" in value;
}

/** The items to buy. The 2026-04-17 schema names the field `line_items`; its
 *  own examples and the RFC say `items`, and its Item has no `quantity` while
 *  the RFC's does — so both names are read and `quantity` defaults to 1. */
export function readItems(body: Record<string, unknown>): AcpItem[] | AcpResult {
  const raw = body.line_items ?? body.items;
  if (!Array.isArray(raw) || raw.length === 0 || raw.length > MAX_LINES) {
    return bad("$.line_items", `Between 1 and ${MAX_LINES} items are required.`);
  }
  const items: AcpItem[] = [];
  for (const [i, entry] of raw.entries()) {
    const id = isObject(entry) ? text(entry.id, 255) : undefined;
    if (!id) return bad(`$.line_items[${i}].id`, "Every item needs an id.");
    const entryObj = entry as Record<string, unknown>;
    const quantity = entryObj.quantity ?? 1;
    if (!Number.isInteger(quantity) || (quantity as number) < 1 || (quantity as number) > MAX_QUANTITY) {
      return bad(`$.line_items[${i}].quantity`, `The quantity must be a whole number from 1 to ${MAX_QUANTITY}.`);
    }
    const unit = entryObj.unit_amount;
    if (unit !== undefined && (!Number.isInteger(unit) || (unit as number) < 0)) {
      return bad(`$.line_items[${i}].unit_amount`, "unit_amount must be a whole number of minor units.");
    }
    // Two entries naming one product are one line with more copies, as in the
    // storefront's basket.
    const existing = items.find((it) => it.id === id);
    if (existing) existing.quantity = Math.min(MAX_QUANTITY, existing.quantity + (quantity as number));
    else items.push({ id, quantity: quantity as number, ...(unit === undefined ? {} : { unit_amount: unit as number }) });
  }
  return items;
}

/** The buyer, kept to the fields the shop uses. `null` clears (RFC §6.2). */
export function readBuyer(value: unknown): Record<string, string> | null | AcpResult {
  if (value === null) return null;
  if (!isObject(value)) return bad("$.buyer", "buyer must be an object.");
  const email = text(value.email, 254);
  if (!email || !isValidEmail(email)) return bad("$.buyer.email", "A valid buyer email is required.");
  const buyer: Record<string, string> = { email };
  for (const key of ["first_name", "last_name", "full_name", "phone_number"]) {
    const v = text(value[key], 120);
    if (v) buyer[key] = v;
  }
  return buyer;
}

/** Fulfilment details and the country they name — the one piece of tax
 *  evidence an agent can give for a digital product. */
export function readDetails(value: unknown): { details: Record<string, unknown> | null; country: string | null } | AcpResult {
  if (value === null) return { details: null, country: null };
  if (!isObject(value)) return bad("$.fulfillment_details", "fulfillment_details must be an object.");
  const details: Record<string, unknown> = {};
  for (const key of ["name", "phone_number", "email"]) {
    const v = text(value[key], 254);
    if (v) details[key] = v;
  }
  let country: string | null = null;
  if (value.address !== undefined) {
    if (!isObject(value.address)) return bad("$.fulfillment_details.address", "address must be an object.");
    const address: Record<string, string> = {};
    for (const key of ["name", "line_one", "line_two", "city", "state", "postal_code"]) {
      const v = text(value.address[key], 200);
      if (v) address[key] = v;
    }
    const code = typeof value.address.country === "string" ? value.address.country.trim() : "";
    if (!COUNTRY_RE.test(code)) {
      return bad("$.fulfillment_details.address.country", "The country must be a two-letter ISO 3166-1 code.");
    }
    country = code.toUpperCase();
    address.country = country;
    details.address = address;
  }
  return { details, country };
}

/** The request's currency, or the shop's base currency when none is named. */
export async function readCurrency(env: Env, value: unknown): Promise<string | AcpResult> {
  if (value === undefined) return (await getSettingString(env, "currency.base")).toUpperCase();
  if (typeof value !== "string" || !CURRENCY_RE.test(value)) return bad("$.currency", "currency must be an ISO 4217 code.");
  return value.toUpperCase();
}

/** The only fulfilment this shop offers is the digital one. */
export function readSelectedOptions(value: unknown): AcpResult | null {
  if (value === undefined || value === null) return null;
  if (!Array.isArray(value)) return bad("$.selected_fulfillment_options", "selected_fulfillment_options must be an array.");
  for (const [i, option] of value.entries()) {
    if (!isObject(option) || option.option_id !== DIGITAL_OPTION_ID) {
      return bad(`$.selected_fulfillment_options[${i}].option_id`, `The only fulfilment option is "${DIGITAL_OPTION_ID}".`);
    }
  }
  return null;
}

export const DIGITAL_OPTION_ID = "digital";
