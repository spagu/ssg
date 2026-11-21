// Who an external issuer's verified token names (#435). A leading underscore
// keeps this file out of the Pages route table.
//
// The identity is the pair ("jwt:<issuer>", <subject>), like a login
// provider's:
//   - AUTH_JWT_IDENTITY_CLAIM=sub (default): the subject is `sub`, the
//     issuer's stable id for the person. The address, when there is one,
//     only finds an open invitation or makes a new account;
//   - AUTH_JWT_IDENTITY_CLAIM=email: the subject IS the address, so the token
//     must carry a usable one, and an existing account with exactly that
//     address is the person's (the administrator chose the address as the
//     identity).
// An address counts only when the issuer says it verified it
// (`email_verified: true`, or the string "true") — unless
// AUTH_JWT_EMAIL_VERIFIED=off — and only in a form that cannot be mistaken
// for another address (matchableEmail, _access-identity.ts: printable ASCII,
// the worker's one normalisation).

import { matchableEmail } from "./_access-identity";
import type { Claims } from "./_external-jwt";
import type { ExternalSetup } from "./_external-config";
import { normEmail } from "./_lib";
import type { Identity } from "./_provider";
import { truthy } from "./_provider-oidc";

const MAX_ROLES = 200;

// externalIdentity: the identity, or why the token names nobody usable.
export function externalIdentity(c: Claims, s: ExternalSetup): Identity | { problem: "no subject" | "no verified email" } {
  const raw = typeof c.email === "string" ? c.email.trim() : "";
  const matchable = matchableEmail(raw);
  const usable = matchable !== "" && (!s.emailVerified || truthy(c.email_verified));
  if (s.identityClaim === "email") {
    return usable ? { subject: matchable, email: matchable, emailVerified: true, name: "" } : { problem: "no verified email" };
  }
  if (typeof c.sub !== "string" || !c.sub || c.sub.length > 255) return { problem: "no subject" };
  return { subject: c.sub, email: matchable || normEmail(raw).slice(0, 254), emailVerified: usable, name: "" };
}

// rolesClaim reads the person's external roles: an array of names, or one
// space-separated string (the OAuth `scope` form); [] for anything else.
export function rolesClaim(c: Claims, name: string): string[] {
  const v = c[name];
  const raw = Array.isArray(v) ? v : typeof v === "string" ? v.split(" ") : [];
  const out = new Set<string>();
  for (const r of raw.slice(0, MAX_ROLES)) if (typeof r === "string" && r.trim() && r.length <= 256) out.add(r.trim());
  return [...out];
}
