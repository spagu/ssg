// What the signed-in account may see and do, as pure functions over the
// token's permissions (the shell's data block). The panel HIDES what the
// account cannot use; the API refuses it regardless, so a hidden button is
// courtesy, never the protection.

export const ADMIN = "auth:admin";
export const INVITE = "auth:invite";
export const OWNER = "*";

// can is the worker's own rule for a concrete permission
// (functions/api/auth/_can.ts): allowed when `perms` holds exactly it, its
// "worker:*", or "*". The panel also asks "does this account hold `*`?"
// (owner-only buttons): for a wildcard, only that wildcard itself or "*"
// answers yes — the server's can() refuses wildcard questions altogether.
const NAME = /^(?:\*|[a-z][a-z0-9-]*:(?:\*|[a-z][a-z0-9-]*))$/;
export function can(perms, required) {
  if (!Array.isArray(perms) || !NAME.test(required)) return false;
  if (required.endsWith("*")) return perms.some((p) => p === required || p === "*");
  const all = required.slice(0, required.indexOf(":") + 1) + "*";
  return perms.some((p) => p === required || p === all || p === "*");
}

// VIEW_NEEDS: the permission each view needs (null: none).
export const VIEW_NEEDS = {
  home: null,
  people: ADMIN,
  person: ADMIN,
  invitations: INVITE,
  groups: ADMIN,
  group: ADMIN,
  roles: ADMIN,
  role: ADMIN,
  matrix: ADMIN,
  sessions: ADMIN,
  services: ADMIN,
  service: ADMIN,
  mappings: ADMIN,
  audit: ADMIN,
  status: ADMIN,
  notFound: null,
};

// ACTION_NEEDS: what each button needs beyond its view. Resetting a second
// factor and letting a token carry sensitive permissions are an owner's (`*`).
export const ACTION_NEEDS = { resetFactor: OWNER, allowSensitive: OWNER, invite: INVITE, chooseRoles: ADMIN };

export const NAV = ["people", "invitations", "groups", "roles", "matrix", "sessions", "services", "mappings", "audit", "status"];

// viewLacks: the permission missing to open `view`, or null.
export function viewLacks(me, view) {
  const need = VIEW_NEEDS[view] ?? null;
  return need && !can(me.perms, need) ? need : null;
}

// allows: may `me` press the button for `action`?
export const allows = (me, action) => can(me.perms, ACTION_NEEDS[action] ?? action);

// shown: is the view part of this site at all? Access mappings exist only
// while Cloudflare Access is configured.
export const shown = (me, view) => view !== "mappings" || me.access === "enabled" || me.access === "misconfigured";

// navFor: the sections this account can open, in order.
export const navFor = (me) => NAV.filter((v) => shown(me, v) && viewLacks(me, v) === null);

// home: where "#/" goes — the first section the account can open.
export const home = (me) => navFor(me)[0] ?? "invitations";
