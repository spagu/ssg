// Who a publication is by (README of the round, decision 5): the commit's
// author is the signed-in account; the committer is the token's owner, which
// the repository host sets itself.
//
// An `auth` token carries an id and an e-mail, no display name
// (workers/auth/docs/CONTRACT.md), so a person's commit is authored
// "<email> <email>". A service token carries the service account's id and an
// empty e-mail (SERVICE-TOKENS.md): its commit is authored by "service account
// <id>" with an address under the reserved .invalid domain (RFC 2606), which
// no mailbox can ever answer — the history still says which account it was.
// Shared by the panel workers (GO-185), so a service account's commits read
// the same whichever panel made them.

import { sanitizeAuthor } from "./repo-provider";
import type { Author } from "./repo-provider";
import type { Caller } from "./gate";

export const SERVICE_DOMAIN = "service-account.invalid";

// authorOf: the commit author for the caller (sanitizeAuthor strips what
// could forge a header; an unusable e-mail is refused as invalid_author).
export function authorOf(caller: Caller): Author {
  if (caller.kind === "service") {
    const local = caller.user.id.replace(/[^A-Za-z0-9._-]/g, "-").slice(0, 64);
    return sanitizeAuthor({ name: `service account ${caller.user.id}`, email: `${local}@${SERVICE_DOMAIN}` });
  }
  return sanitizeAuthor({ name: caller.user.email, email: caller.user.email });
}
