// Hash routing: "#/campaigns/<id>?tab=stats". The worker serves the panel's
// files from {base}/ and knows nothing about views, so the view lives after
// the "#": a deep link or a reload always loads index.html, whatever the base
// path, and no server rule is needed for it.

export const ROUTES = [
  ["dashboard", ""],
  ["subscribers", "subscribers"],
  ["subscriber", "subscribers/:id"],
  ["import", "import"],
  ["campaigns", "campaigns"],
  ["campaignNew", "campaigns/new"],
  ["campaign", "campaigns/:id"],
  ["suppressions", "suppressions"],
  ["settings", "settings"],
  ["activity", "activity"],
  ["help", "help"],
];

// The navigation entry each view belongs to (for aria-current).
export const SECTION = { subscriber: "subscribers", campaignNew: "campaigns", campaign: "campaigns" };

const split = (path) => path.split("/").filter(Boolean);

// isRoute: only "#/…" (or nothing) is a view; "#main" is an in-page anchor
// (the skip link) and must not change the view.
export const isRoute = (hash) => !hash || hash === "#" || hash.startsWith("#/");

// parse("#/subscribers/abc?x=1") → {name: "subscriber", params: {id: "abc"}, query: {x: "1"}}.
export function parse(hash) {
  const raw = (hash || "").replace(/^#\/?/, "");
  const [path, qs = ""] = raw.split("?");
  const query = Object.fromEntries(new URLSearchParams(qs));
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

// href("subscriber", {id}, {tab: "x"}) → "#/subscribers/<id>?tab=x".
export function href(name, params = {}, query = {}) {
  const route = ROUTES.find(([n]) => n === name);
  if (!route) throw new Error(`unknown route ${name}`);
  const path = route[1].replace(/:(\w+)/g, (_, k) => encodeURIComponent(params[k]));
  const q = new URLSearchParams(Object.entries(query).filter(([, v]) => v !== null && v !== undefined && v !== "")).toString();
  return "#/" + path + (q ? "?" + q : "");
}
