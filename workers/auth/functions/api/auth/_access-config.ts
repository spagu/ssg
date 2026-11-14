// Cloudflare Access as a way to sign in (GO-139): its variables, read and
// checked. A leading underscore keeps this file out of the Pages route table.
//
// Access answers "this person may enter" — it vouches for who the person is
// at the identity provider the team uses. This worker then decides what the
// person may DO from its own roles, like after any other sign-in. The method
// id is "access" (stored with the session; AUTH_ROLE_METHODS may name it).
//
//   AUTH_ACCESS_TEAM            the team name (<team>.cloudflareaccess.com), or
//                               that host; nothing else — the host whose keys
//                               are trusted comes from here only, never from a
//                               request or a token
//   AUTH_ACCESS_AUD             the Application Audience (AUD) tag of the Access
//                               application that covers /auth/access
//   AUTH_ACCESS_LINK_BY_EMAIL   "on": an Access identity not linked yet may enter
//                               the EXISTING account with the same address (off
//                               by default — docs/ACCESS.md says why)
//   AUTH_ACCESS_AUTO_CREATE     "on": an unknown Access identity gets an account
//   AUTH_ACCESS_DEFAULT_ROLE    … with this role only (required with it; never
//                               owner, *, or a sensitive permission —
//                               _access-provision.ts checks against the database)
//   AUTH_ACCESS_ALLOWED_DOMAINS like every provider's (_provider-policy.ts)
//
// TEAM and AUD go together: both → the method exists; neither → it does not
// (its routes answer 404, as if absent); one alone, or a value that is not
// usable → misconfigured: no button, and its routes answer 503 naming what to
// fix. Never opened by a half configuration.

import type { Env } from "./_lib";

export const ACCESS_METHOD = "access";
export const ACCESS_NAME = "Cloudflare Access"; // a product name, never translated
export const ACCESS_HEADER = "cf-access-jwt-assertion";
export const ACCESS_COOKIE_NAME = "CF_Authorization";

export interface AccessSetup {
  host: string; // <team>.cloudflareaccess.com
  aud: string;
  linkByEmail: boolean;
  autoCreate: boolean;
  defaultRole: string; // "" unless autoCreate
}

export type AccessLookup = { kind: "enabled"; setup: AccessSetup } | { kind: "absent" } | { kind: "misconfigured"; problem: string };

const TEAM_LABEL = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;
const TEAM_SUFFIX = ".cloudflareaccess.com";
// An AUD tag is 64 hex characters today; anything of token characters is
// accepted so a longer tag would still work, nothing with spaces or quotes.
const AUD = /^[A-Za-z0-9_-]{16,256}$/;

// teamHost: "myteam", "myteam.cloudflareaccess.com" or
// "https://myteam.cloudflareaccess.com/" → "myteam.cloudflareaccess.com";
// "" for anything else (another domain is never trusted for keys).
export function teamHost(raw: string): string {
  let v = raw.trim().toLowerCase().replace(/^https:\/\//, "").replace(/\/$/, "");
  if (v.endsWith(TEAM_SUFFIX)) v = v.slice(0, -TEAM_SUFFIX.length);
  return TEAM_LABEL.test(v) ? v + TEAM_SUFFIX : "";
}

// onOff reads a switch: "on" → true, unset/"off" → false, anything else null.
function onOff(raw: string | undefined): boolean | null {
  const v = (raw ?? "").trim().toLowerCase();
  if (v === "on") return true;
  return v === "" || v === "off" ? false : null;
}

export function accessLookup(env: Env): AccessLookup {
  const team = (env.AUTH_ACCESS_TEAM ?? "").trim();
  const aud = (env.AUTH_ACCESS_AUD ?? "").trim();
  if (!team && !aud) return { kind: "absent" };
  const bad = (problem: string): AccessLookup => ({ kind: "misconfigured", problem });
  if (!team) return bad("AUTH_ACCESS_AUD is set but AUTH_ACCESS_TEAM is not: set both (the team name and the application's AUD tag) or neither");
  if (!aud) return bad("AUTH_ACCESS_TEAM is set but AUTH_ACCESS_AUD is not: set both (the team name and the application's AUD tag) or neither");
  const host = teamHost(team);
  if (!host) return bad("AUTH_ACCESS_TEAM must be your team name, e.g. \"myteam\" for myteam.cloudflareaccess.com");
  if (!AUD.test(aud)) return bad("AUTH_ACCESS_AUD must be the Application Audience (AUD) tag copied from the Access application");
  const linkByEmail = onOff(env.AUTH_ACCESS_LINK_BY_EMAIL);
  const autoCreate = onOff(env.AUTH_ACCESS_AUTO_CREATE);
  if (linkByEmail === null) return bad("AUTH_ACCESS_LINK_BY_EMAIL must be \"on\" or \"off\"");
  if (autoCreate === null) return bad("AUTH_ACCESS_AUTO_CREATE must be \"on\" or \"off\"");
  const defaultRole = autoCreate ? (env.AUTH_ACCESS_DEFAULT_ROLE ?? "").trim() : "";
  if (autoCreate && !defaultRole) return bad("AUTH_ACCESS_AUTO_CREATE=on needs AUTH_ACCESS_DEFAULT_ROLE, the name of the one role new accounts get");
  return { kind: "enabled", setup: { host, aud, linkByEmail, autoCreate, defaultRole } };
}

export const accessIssuer = (s: AccessSetup): string => `https://${s.host}`;

// accessLogoutUrl: where a browser signs out of Access — built from the
// configuration only, so the logout answer is never an open redirect.
export const accessLogoutUrl = (s: AccessSetup): string => `https://${s.host}/cdn-cgi/access/logout`;

// configuredLogoutUrl: the same for a site with Access enabled, else null.
export function configuredLogoutUrl(env: Env): string | null {
  const l = accessLookup(env);
  return l.kind === "enabled" ? accessLogoutUrl(l.setup) : null;
}
