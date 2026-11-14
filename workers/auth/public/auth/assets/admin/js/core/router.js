// Hash routing: "#/people/<id>?status=blocked". The shell is one page
// (/auth/admin/); the view lives after the "#", so a deep link or a reload
// always loads the shell, and the step-up page can send the browser back to
// exactly the view it left.

export const ROUTES = [
  ["home", ""],
  ["people", "people"],
  ["person", "people/:id"],
  ["invitations", "invitations"],
  ["groups", "groups"],
  ["group", "groups/:id"],
  ["roles", "roles"],
  ["role", "roles/:id"],
  ["matrix", "matrix"],
  ["sessions", "sessions"],
  ["services", "services"],
  ["service", "services/:id"],
  ["mappings", "mappings"],
  ["audit", "audit"],
  ["status", "status"],
];

// The navigation entry each view belongs to (for aria-current).
export const SECTION = { person: "people", group: "groups", role: "roles", service: "services" };

const split = (path) => path.split("/").filter(Boolean);

// isRoute: only "#/…" (or nothing) is a view; "#main" is the skip link's anchor.
export const isRoute = (hash) => !hash || hash === "#" || hash.startsWith("#/");

// parse("#/people/abc?x=1") → {name: "person", params: {id: "abc"}, query: {x: "1"}, section: "people"}.
export function parse(hash) {
  const raw = (hash || "").replace(/^#\/?/, "");
  const cut = raw.indexOf("?");
  const path = cut < 0 ? raw : raw.slice(0, cut);
  const query = Object.fromEntries(new URLSearchParams(cut < 0 ? "" : raw.slice(cut + 1)));
  const got = split(path);
  for (const [name, pattern] of ROUTES) {
    const want = split(pattern);
    if (want.length !== got.length) continue;
    const params = {};
    const ok = want.every((w, i) => {
      if (!w.startsWith(":")) return w === got[i];
      try {
        params[w.slice(1)] = decodeURIComponent(got[i]);
        return true;
      } catch {
        return false;
      }
    });
    if (ok) return { name, params, query, section: SECTION[name] ?? name };
  }
  return { name: "notFound", params: {}, query, section: null };
}

// href("person", {id}, {tab: "x"}) → "#/people/<id>?tab=x".
export function href(name, params = {}, query = {}) {
  const route = ROUTES.find(([n]) => n === name);
  if (!route) throw new Error(`unknown route ${name}`);
  const path = route[1].replace(/:(\w+)/g, (_, k) => encodeURIComponent(params[k]));
  const q = new URLSearchParams(Object.entries(query).filter(([, v]) => v !== null && v !== undefined && v !== "")).toString();
  return "#/" + path + (q ? "?" + q : "");
}
