// What an Access sign-in is, as an identity of this worker (GO-139). A leading
// underscore keeps this file out of the Pages route table.
//
// The identity is the pair ("access", <Access sub>) — like a login provider's.
// The address matters for one thing only: finding an open invitation (or,
// when the administrator opted in, an existing account) for a person Access
// has not been linked to yet. Access asserts the address its identity
// provider verified, so it counts as verified — but only in a form that can
// not be mistaken for another address:
//   - compared after the worker's one normalisation (normEmail: trimmed,
//     lower-cased), the same as every stored address;
//   - printable ASCII only. Lower-casing maps some non-ASCII letters onto
//     ASCII ones (U+212A KELVIN SIGN → "k"), so "Kate@example.com" would
//     otherwise land on kate@example.com; a Unicode domain (IDN) is not
//     matched to its punycode form either. Such an address matches nothing —
//     the account links Access from its account page instead;
//   - nothing else is folded: plus-tags, dots and a trailing dot after the
//     domain make a DIFFERENT address, as they may well be a different mailbox.

import type { AccessPerson } from "./_access-jwt";
import { looksLikeEmail, normEmail } from "./_lib";
import type { Identity } from "./_provider";

// matchableEmail: the address to compare with stored ones, or "" when this
// address may not be matched to anything.
export function matchableEmail(raw: string): string {
  if (!/^[\x21-\x7e]+$/.test(raw)) return "";
  const e = normEmail(raw);
  return looksLikeEmail(e) ? e : "";
}

// identityOf: emailVerified means "may be matched", the address shown is the
// matchable one or, failing that, the given one (for display and the audit
// trail only).
export function identityOf(p: AccessPerson): Identity {
  const matchable = matchableEmail(p.email);
  return { subject: p.subject, email: matchable || normEmail(p.email).slice(0, 254), emailVerified: matchable !== "", name: "" };
}
