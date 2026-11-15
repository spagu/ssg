// Who is signed in, and what the panel may offer them (GO-155).
//
// The API refuses what an account may not do whatever the panel shows; this
// only keeps the panel from offering it. An element that needs a permission
// says so in a data-needs attribute — in index.html or where a script builds
// it — and is marked data-denied (hidden by admin.css) when the account lacks
// it. In the password and Access modes /me lists every permission, so nothing
// is hidden.

let held = [];

/** The permissions /me reported. */
export function setPermissions(list) {
  held = Array.isArray(list) ? [...list] : [];
}

export function may(permission) {
  return held.includes(permission);
}

/** What each screen needs before it is worth opening. */
export const VIEW_NEEDS = {
  dashboard: "ecommerce:read",
  products: "ecommerce:read",
  orders: "ecommerce:read",
  modules: "ecommerce:settings",
  payments: "ecommerce:settings",
  settings: "ecommerce:settings",
  vat: "ecommerce:read",
  log: "ecommerce:read",
};

/** The first screen this account can open, or null for none at all. */
export function firstView(wanted) {
  if (VIEW_NEEDS[wanted] && may(VIEW_NEEDS[wanted])) return wanted;
  return Object.keys(VIEW_NEEDS).find((name) => may(VIEW_NEEDS[name])) ?? null;
}

/** Marks every [data-needs] element at or under `root` the account cannot use. */
export function applyNeeds(root) {
  const nodes = [...(root.matches?.("[data-needs]") ? [root] : []), ...(root.querySelectorAll?.("[data-needs]") ?? [])];
  for (const node of nodes) node.toggleAttribute("data-denied", !may(node.dataset.needs));
}

/** The login URL to follow, as a same-origin path, or null. `auth` lives on the
 *  same host as the shop, so a URL elsewhere is a misconfiguration or a
 *  tampered answer — never a place to send the person who runs the shop. */
export function loginTarget(loginUrl, origin) {
  if (typeof loginUrl !== "string" || !loginUrl) return null;
  let url;
  try {
    url = new URL(loginUrl, origin);
  } catch {
    return null;
  }
  return url.origin === origin ? url.pathname + url.search + url.hash : null;
}

/** How the top bar names who is signed in. */
export function signedInAs(me) {
  if (me.user?.email) return me.user.email;
  return me.mode === "password" ? "Shared password" : "Signed in";
}

/** The sentence for a 403: what is missing, by name. */
export function missingPermission(permission) {
  return `Your account does not have the permission ${permission}. Ask whoever runs the shop's accounts to grant it.`;
}
