// No answer of this worker carries a secret's value (README of the round,
// decision 8; GO-178). A value inside a secret key — `jwt_secret`,
// `mddb.api_key`, `workers[].auth.token`, every entry of `server_users`, every
// header of a notification — is answered as {set, reference}: whether it is
// set, and the environment reference it is written as, or null when it is
// anything else (a literal pasted in by hand is never echoed). Keys the schema
// does not know are left out: nothing says they are not secrets.

import { ENV_WHOLE } from "./overlay-validate";
import { child, elementOf, secretFormatOf, shapeOf } from "./schema";
import type { SchemaNode, SecretFormat } from "./schema";

export interface SecretState { set: boolean; reference: string | null }

// STRICT: the reference forms this worker shows and accepts — the whole value
// a reference; "login:$NAME"; or for a header an optional scheme word before
// one ("Bearer $TOKEN"). ssg accepts more for `inline` (any text holding a
// reference), but text around a reference may itself be a credential.
const REF = String(ENV_WHOLE.source).slice(1, -1);
const STRICT: Record<SecretFormat, RegExp> = {
  whole: ENV_WHOLE,
  login: new RegExp(`^[^:\\s]{1,64}:${REF}$`),
  inline: new RegExp(`^(?:[A-Za-z][A-Za-z0-9-]{0,30} )?${REF}$`),
};

// strictReference: is `s` a reference in the strict form of `format`?
export const strictReference = (format: SecretFormat, s: string): boolean => STRICT[format].test(s);

// secretState: what may be said about a value inside a secret key.
export function secretState(format: SecretFormat, v: unknown): SecretState {
  const set = v !== null && v !== undefined && v !== "";
  return { set, reference: typeof v === "string" && strictReference(format, v) ? v : null };
}

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

// redact returns `v` (described by `n`) with every value inside a secret key
// replaced by its SecretState and every key the schema does not know dropped.
export function redact(v: unknown, n: SchemaNode): unknown {
  const format = secretFormatOf(n);
  if (format && !Array.isArray(v) && !isObject(v)) return secretState(format, v);
  const s = shapeOf(v, [n, n.alternate, n]);
  if (s.type === "any" && !format) return v; // free-form (variables.*): no secret key inside
  if (Array.isArray(v)) return v.map((e) => redact(e, elementOf(s, e)));
  if (!isObject(v)) return v;
  const out: Record<string, unknown> = {};
  for (const [k, e] of Object.entries(v)) {
    const c = child(s, k);
    if (c) out[k] = redact(e, c);
  }
  return out;
}
