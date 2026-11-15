// Hash routing: "#/options/site?q=feed". The worker serves the panel's files
// from {base}/ and knows nothing about views, so the view lives after the
// "#": a deep link or a reload always loads index.html, whatever the base
// path, and no server rule is needed for it. Generic — a panel passes its
// own route table to createRouter, so another panel can copy this file as is.

const split = (path) => path.split("/").filter(Boolean);

// isRoute: only "#/…" (or nothing) is a view; "#main" is an in-page anchor
// (the skip link) and must not change the view.
export const isRoute = (hash) => !hash || hash === "#" || hash.startsWith("#/");

// createRouter(routes, sections): routes = [[name, "pattern/:param"], …] in
// match order; sections = {routeName: navName} for views that belong to
// another view's navigation entry.
export function createRouter(routes, sections = {}) {
  // parse("#/options/site?q=x") → {name, params: {group: "site"}, query: {q: "x"}, section}.
  function parse(hash) {
    const raw = (hash || "").replace(/^#\/?/, "");
    const [path, qs = ""] = raw.split("?");
    const query = Object.fromEntries(new URLSearchParams(qs));
    const got = split(path);
    for (const [name, pattern] of routes) {
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
      if (ok) return { name, params, query, section: sections[name] ?? name };
    }
    return { name: "notFound", params: {}, query, section: null };
  }

  // href("options", {group: "site"}, {q: "x"}) → "#/options/site?q=x".
  function href(name, params = {}, query = {}) {
    const route = routes.find(([n]) => n === name);
    if (!route) throw new Error(`unknown route ${name}`);
    const path = route[1].replace(/:(\w+)/g, (_, k) => encodeURIComponent(params[k]));
    const q = new URLSearchParams(Object.entries(query).filter(([, v]) => v !== null && v !== undefined && v !== "")).toString();
    return "#/" + path + (q ? "?" + q : "");
  }

  return { parse, href };
}
