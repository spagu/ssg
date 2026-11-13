// Which column of an uploaded list is which: the header names that mailing
// tools put in their exports, in ONE table (ALIASES), so supporting another
// layout is a line here and nothing else.
//
// Headers are compared normalised — case, spaces, underscores, hyphens and dots
// ignored — so "E-mail Address", "EMAIL_ADDRESS" and "email.address" are one
// name. Each alias list is in priority order: when two columns claim one role
// (an export with both an opt-in time and a creation time), the earlier alias
// wins and the preview says which column was used and which was not.
//
// Names are recognised on purpose, only to be reported as not stored: this
// worker keeps an address, a language and tags, nothing that identifies a
// person beyond the address (data minimisation).

import type { PlanError } from "./import-plan";

export const ROLES = ["email", "status", "signedUpAt", "unsubscribedAt", "bouncedAt", "language", "tags", "sourcePage"] as const;
export type Role = (typeof ROLES)[number];
export type Mapping = Partial<Record<Role, string | null>>;

export const ALIASES: Record<Role | "name", readonly string[]> = {
  email: ["email", "emailaddress", "mail", "primaryemail", "subscriberemail", "contactemail", "useremail", "emailaddr"],
  status: ["status", "subscriberstatus", "subscriptionstatus", "memberstatus", "contactstatus", "emailstatus", "state"],
  signedUpAt: [
    "subscribedat", "subscribedate", "subscribeddate", "datesubscribed", "subscriptiondate", "subscribedon", "subscriptiontime",
    "optintime", "optindate", "optinat", "optintimestamp",
    "confirmtime", "confirmedat", "confirmationdate", "confirmeddate", "dateconfirmed", "confirmdate", "confirmationtime",
    "signupdate", "signuptime", "signedupat", "signupat", "signedupon", "signedup",
    "createdat", "created", "createddate", "createdon", "datecreated", "creationdate",
    "dateadded", "addedat", "addedon", "added", "joinedat", "joindate", "datejoined", "joined",
    "timestamp", "date",
  ],
  unsubscribedAt: [
    "unsubscribedat", "unsubscribedate", "unsubscribeddate", "unsubscribedon", "unsubscribetime", "unsubtime", "unsubdate",
    "optouttime", "optoutdate", "optedoutat",
  ],
  bouncedAt: ["bouncedat", "bouncedate", "bounceddate", "bouncetime", "bouncedon", "cleanedat", "cleantime", "cleandate", "cleaneddate"],
  language: ["language", "lang", "locale", "languagecode", "preferredlanguage", "emaillanguage"],
  tags: ["tags", "tag", "groups", "group", "segments", "segment", "lists", "list", "labels"],
  sourcePage: ["sourcepage", "signupsource", "source", "signupurl", "signuppage", "sourceurl", "referrer", "referer", "referringurl"],
  name: [
    "name", "fullname", "firstname", "lastname", "fname", "lname", "givenname", "familyname", "surname",
    "middlename", "displayname", "subscribername", "contactname",
  ],
};

export const NAMES_NOT_STORED = "ignored: this worker does not store names";
export const NOT_RECOGNISED = "ignored: not a recognised column";

export type IgnoredCode = "name_not_stored" | "used_elsewhere" | "not_recognised";

export interface Detected {
  delimiter: string;
  headerless: boolean; // true: line 1 is already an address, columns are "column 1", "column 2", …
  columns: Record<Role, string | null>; // role → the header used, null when the file has none
  headers: string[]; // every header in file order ("column N" for a headerless file)
  // code is for the panel to branch on, reason is the English text to show:
  // name_not_stored | used_elsewhere (another column of the same kind won) | not_recognised
  ignored: { header: string; code: IgnoredCode; reason: string }[];
  ambiguous: { role: Role; chosen: string; alsoMatched: string[] }[];
}
export interface Columns {
  index: Record<Role, number>; // -1 = not in the file
  detected: Detected;
}

export const normaliseHeader = (h: string): string => h.trim().toLowerCase().replace(/[\s_.-]+/g, "");

const badMapping = (detail: string): PlanError => ({ error: "invalid_mapping", detail, status: 400 });

// parseMapping validates the request's override: an object of role → header
// (null or "" = "do not use any column for this role").
export function parseMapping(raw: unknown): Mapping | PlanError {
  if (raw === undefined || raw === null) return {};
  if (typeof raw !== "object" || Array.isArray(raw)) return badMapping('send {"email": "<header>", …}');
  const out: Mapping = {};
  for (const [role, header] of Object.entries(raw)) {
    if (!(ROLES as readonly string[]).includes(role)) return badMapping(`unknown role "${role}"; roles are ${ROLES.join(", ")}`);
    if (header !== null && typeof header !== "string") return badMapping(`the column for ${role} must be a header name or null`);
    out[role as Role] = header;
  }
  return out;
}

// findHeader matches the admin's header name exactly first, then normalised.
function findHeader(headers: string[], name: string): number {
  const exact = headers.indexOf(name);
  return exact >= 0 ? exact : headers.map(normaliseHeader).indexOf(normaliseHeader(name));
}

// detectColumns assigns a column to each role: the override first, then the
// alias table for the roles it left open, never one column to two roles.
export function detectColumns(headers: string[], mapping: Mapping, delimiter: string, headerless: boolean): Columns | PlanError {
  const index = Object.fromEntries(ROLES.map((r) => [r, -1])) as Record<Role, number>;
  const used = new Set<number>();
  for (const [role, name] of Object.entries(mapping) as [Role, string | null][]) {
    if (!name) continue;
    const i = findHeader(headers, name);
    if (i < 0) return badMapping(`no column "${name}" for ${role}`);
    if (used.has(i)) return badMapping(`column "${name}" is mapped to two roles`);
    index[role] = i;
    used.add(i);
  }
  const norm = headers.map(normaliseHeader);
  const ambiguous: Detected["ambiguous"] = [];
  for (const role of ROLES) {
    if (role in mapping) continue;
    const hits = norm
      .map((h, i) => ({ i, rank: ALIASES[role].indexOf(h) }))
      .filter((m) => m.rank >= 0 && !used.has(m.i))
      .sort((a, b) => a.rank - b.rank || a.i - b.i);
    if (!hits.length) continue;
    index[role] = hits[0].i;
    used.add(hits[0].i);
    if (hits.length > 1) ambiguous.push({ role, chosen: headers[hits[0].i], alsoMatched: hits.slice(1).map((m) => headers[m.i]) });
  }
  // A list without a header row has its address in the first column.
  // (Its made-up "column N" headers match no alias, so only an override can
  // have taken the email role or column 1 already.)
  if (headerless && !("email" in mapping) && !used.has(0)) (index.email = 0), used.add(0);
  if (index.email < 0) return { error: "missing_email_column", detail: "the header row must name an email column", status: 400 };

  const loser = (i: number) => ambiguous.find((a) => a.alsoMatched.includes(headers[i]));
  const ignored = headers.flatMap((header, i): Detected["ignored"] => {
    if (used.has(i)) return [];
    const lost = loser(i);
    if (ALIASES.name.includes(norm[i])) return [{ header, code: "name_not_stored", reason: NAMES_NOT_STORED }];
    if (lost) return [{ header, code: "used_elsewhere", reason: `ignored: "${lost.chosen}" was used for ${lost.role}` }];
    return [{ header, code: "not_recognised", reason: NOT_RECOGNISED }];
  });
  const columns = Object.fromEntries(ROLES.map((r) => [r, index[r] < 0 ? null : headers[index[r]]])) as Record<Role, string | null>;
  return { index, detected: { delimiter, headerless, headers, columns, ignored, ambiguous } };
}
