// Local preview of the designer panel against the real worker, with an
// invented site. Nothing leaves the machine: GitHub is the in-process fake
// (test/fake-github.ts), sign-in a pretend `auth` worker on the same port.
//
//   node --import ./workers/designer-ui/test/resolve.mjs workers/designer-ui/test/dev-server.mjs
//   → http://localhost:8788/designer/   (pick an account at /auth/login)
//
// The site: a .ssg.yaml with a dozen settings (one secret written by hand as
// a literal, one value the reader cannot follow), a .ssg.designer.json the
// panel wrote twice (so History has entries by two people), and the theme
// ssgtheme copied from templates/ssgtheme (tokens.css, templates, images).
// DEMO_THEME=builtin uses the built-in "simple" theme instead (Appearance and
// Files then show the export instruction). DEMO_CORE=1 also serves the
// `core` worker's /core/ files (the shared admin bar). PORT=… changes the port.
//
// Accounts: owner (every permission), settings (read + settings), advanced
// (read + settings + advanced), theme (read + theme), reader (read only).
import { createServer } from "node:http";
import { readFile, readdir } from "node:fs/promises";
import { extname, join, normalize, relative } from "node:path";
import { fileURLToPath } from "node:url";

const here = fileURLToPath(new URL(".", import.meta.url));
const PUBLIC = join(here, "..", "public");
const THEME_SRC = join(here, "..", "..", "..", "templates", "ssgtheme");
const CORE = join(here, "..", "..", "core", "public", "core");
const PORT = Number(process.env.PORT || 8788);
const BASE = "/designer";

const { handleFetch } = await import("../src/app.ts");
const { FakeGitHub, TOKEN } = await import("./fake-github.ts");

const TYPES = { ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8", ".svg": "image/svg+xml", ".json": "application/json", ".png": "image/png", ".webp": "image/webp" };
const ACCOUNTS = {
  owner: ["designer-ui:read", "designer-ui:settings", "designer-ui:advanced", "designer-ui:theme"],
  advanced: ["designer-ui:read", "designer-ui:settings", "designer-ui:advanced"],
  settings: ["designer-ui:read", "designer-ui:settings"],
  theme: ["designer-ui:read", "designer-ui:theme"],
  reader: ["designer-ui:read"],
};

// --- signing as the `auth` worker would ------------------------------------
const enc = new TextEncoder();
const b64u = (b) => Buffer.from(b).toString("base64url");
const part = (v) => b64u(enc.encode(JSON.stringify(v)));
const pair = await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"]);
const jwk = await crypto.subtle.exportKey("jwk", pair.publicKey);
const pub = { crv: "P-256", kty: "EC", x: jwk.x, y: jwk.y };
const kid = b64u(new Uint8Array(await crypto.subtle.digest("SHA-256", enc.encode(JSON.stringify(pub)))));
const ISSUER = "http://localhost/api/auth";
async function sign(as, host, email = `${as}@example.com`) {
  const now = Math.floor(Date.now() / 1000);
  const claims = { iss: ISSUER, aud: host, sub: `u-${as}`, sid: "demo", email, iat: now, exp: now + 600, ver: 1, perms: ACCOUNTS[as] ?? ACCOUNTS.owner };
  const head = `${part({ alg: "ES256", typ: "JWT", kid })}.${part(claims)}`;
  return `${head}.${b64u(new Uint8Array(await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, pair.privateKey, enc.encode(head))))}`;
}

// --- the site in a fake GitHub ---------------------------------------------
const gh = new FakeGitHub();
const builtin = process.env.DEMO_THEME === "builtin";
const SITE_YAML = `# Example Kitchen — written by hand, comments stay
title: Example Kitchen
description: Recipes, menus and notes from a small kitchen
domain: example.com
template: ${builtin ? "simple" : "ssgtheme"}
languages: [en, pl]
default_language: en
timezone: Europe/Warsaw
paginate: 10
feed: true
feed_items: 20
toc: true
webp: true
colors:
  primary: "#0050a6"
  text: "#0f172a"
redirects:
  - from: /old-menu
    to: /menu/
    status: 301
jwt_secret: written-by-hand-literal
mddb:
  enabled: false
  api_key: $MDDB_KEY
hooks:
  pre_build:
    - npm run icons
analytics_ids:
  gtm: GTM-XXXX
story: &story
  enabled: true
`;
const LOGO = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 40"><rect width="120" height="40" rx="8" fill="#0050a6"/><text x="60" y="26" font-family="sans-serif" font-size="16" fill="#fff" text-anchor="middle">Kitchen</text></svg>\n';
async function themeFiles(dir) {
  const out = {};
  for (const e of await readdir(dir, { withFileTypes: true, recursive: true })) {
    if (!e.isFile()) continue;
    const full = join(e.parentPath ?? e.path, e.name);
    if (!/\.(html|css|js|json|svg)$/.test(e.name)) continue;
    out[`templates/ssgtheme/${relative(dir, full)}`] = await readFile(full, "utf8");
  }
  return out;
}
await gh.store.commitFiles("main", { ".ssg.yaml": SITE_YAML, "content/posts/hello.md": "# Hello\n", ...(builtin ? {} : { ...(await themeFiles(THEME_SRC)), "templates/ssgtheme/img/logo.svg": LOGO }) }, "Start the kitchen site");
const env = {
  DESIGNER_BASE: BASE,
  DESIGNER_GIT_REPO: "github.com/acme/site",
  DESIGNER_GIT_BRANCH: "main",
  DESIGNER_GIT_TOKEN: TOKEN,
  DESIGNER_AUTH_ISSUER: ISSUER,
  DESIGNER_AUTH_PUBLIC_KEY: JSON.stringify(pub),
  DESIGNER_AUTH_LOGIN_URL: "/auth/login",
  ASSETS: {
    async fetch(request) {
      let path = decodeURIComponent(new URL(request.url).pathname);
      if (path === "/") path = "/index.html";
      const file = normalize(join(PUBLIC, path));
      if (!file.startsWith(PUBLIC)) return new Response("not found", { status: 404 });
      const body = await readFile(file).catch(() => null);
      return body ? new Response(body, { headers: { "content-type": TYPES[extname(file)] ?? "application/octet-stream" } }) : new Response("not found", { status: 404 });
    },
  },
};
const deps = { fetch: gh.fetch };
const ORIGIN = `http://localhost:${PORT}`;

// Two publications by two people, through the worker itself, for History.
async function seedPublish(as, email, message, changes) {
  const cookie = `__Host-ssg-auth=${await sign(as, `localhost:${PORT}`, email)}`;
  const read = await handleFetch(new Request(`${ORIGIN}${BASE}/api/settings`, { headers: { cookie } }), env, deps);
  const { overlayBlobId } = await read.json();
  const res = await handleFetch(new Request(`${ORIGIN}${BASE}/api/settings/publish`, { method: "POST", headers: { cookie, origin: ORIGIN, "content-type": "application/json" }, body: JSON.stringify({ message, baseBlobId: overlayBlobId, changes }) }), env, deps);
  if (!res.ok) console.error("seed publish failed", res.status, await res.text());
}
await seedPublish("owner", "ana@example.com", "Designer: Site name, Table of contents depth", [{ path: ["title"], value: "Example Kitchen & Garden" }, { path: ["toc_depth"], value: 3 }]);
await seedPublish("settings", "ben@example.com", "Designer: Items per feed", [{ path: ["feed_items"], value: 30 }]);

// --- the pretend auth worker, and core's files ------------------------------
const picked = (headers) => /(?:^|;\s*)demo-as=(\w+)/.exec(headers.get("cookie") ?? "")?.[1];
async function authAnswer(url, method) {
  const next = url.searchParams.get("next") || BASE + "/";
  if (url.pathname === "/auth/login") {
    const as = url.searchParams.get("as");
    if (as in ACCOUNTS) return new Response(null, { status: 302, headers: { location: next, "set-cookie": `demo-as=${as}; Path=/; HttpOnly; SameSite=Lax` } });
    const links = Object.keys(ACCOUNTS).map((a) => `<li><a href="/auth/login?as=${a}&amp;next=${encodeURIComponent(next)}">${a}</a> — ${ACCOUNTS[a].join(", ")}</li>`).join("");
    return new Response(`<!doctype html><title>Sign in (demo)</title><h1>Sign in as…</h1><ul>${links}</ul>`, { headers: { "content-type": TYPES[".html"] } });
  }
  if (url.pathname === "/api/auth/logout" && method === "POST") return new Response(null, { status: 303, headers: { location: BASE + "/", "set-cookie": "demo-as=; Path=/; Max-Age=0" } });
  if (url.pathname === "/api/auth/refresh" && method === "POST") return new Response(null, { status: 204 });
  return null;
}
async function coreAnswer(url) {
  if (!process.env.DEMO_CORE) return url.pathname.startsWith("/core/") ? new Response("not found", { status: 404 }) : null;
  if (url.pathname === "/api/core/modules") return Response.json({ mode: "auth", user: null, lang: "en", home: "/core/", signOut: "/api/auth/logout", items: [{ id: "designer-ui", kind: "worker", title: "Site designer", panel: `${BASE}/`, nav: [], status: "registered" }] });
  if (!url.pathname.startsWith("/core/")) return null;
  const file = normalize(join(CORE, decodeURIComponent(url.pathname.slice("/core/".length))));
  const body = file.startsWith(CORE) ? await readFile(file).catch(() => null) : null;
  return body ? new Response(body, { headers: { "content-type": TYPES[extname(file)] ?? "application/octet-stream" } }) : new Response("not found", { status: 404 });
}

createServer(async (req, res) => {
  const chunks = [];
  for await (const c of req) chunks.push(c);
  const headers = new Headers();
  for (const [k, v] of Object.entries(req.headers)) if (typeof v === "string") headers.set(k, v);
  const url = new URL(`${ORIGIN}${req.url}`);
  const as = picked(headers);
  if (as in ACCOUNTS) headers.set("cookie", `${headers.get("cookie")}; __Host-ssg-auth=${await sign(as, url.host)}`);
  const request = new Request(url, { method: req.method, headers, body: ["GET", "HEAD"].includes(req.method) ? undefined : Buffer.concat(chunks) });
  let response;
  try {
    response = (await coreAnswer(url)) || (await authAnswer(url, req.method)) || (await handleFetch(request, env, deps));
  } catch (e) {
    console.error(e);
    response = new Response("dev server error", { status: 500 });
  }
  res.writeHead(response.status, Object.fromEntries(response.headers));
  res.end(Buffer.from(await response.arrayBuffer()));
}).listen(PORT, () => console.log(`designer-ui demo: ${ORIGIN}${BASE}/  (pick an account at ${ORIGIN}/auth/login)`));
