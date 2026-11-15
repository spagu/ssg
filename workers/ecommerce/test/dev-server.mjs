// Local preview of the shop's admin panel against the real Pages Functions,
// with invented data. Nothing leaves the machine.
//
//   node --import ./workers/ecommerce/test/resolve.mjs workers/ecommerce/test/dev-server.mjs
//   → http://localhost:8788/ecommerce-admin/   (password "demo")
//
// `public/` is served from the root as Pages serves it; every route under
// functions/ runs the template's own handler — admin routes behind the real
// admin/_middleware.ts — on the test fakes: D1 is SQLite in memory
// (test/fakes.ts), R2 and KV live in memory. The demo shop has six products,
// sixty orders in every state, their invoices and download links, a few
// refunds, failed payment notifications and an outbox with messages that keep
// failing. Dates are relative to today (UTC), so two runs on one day draw the
// same screens. Stripe has a test key and PayPal runs in its sandbox; no mail
// API is set, so nothing is ever sent. PORT=… changes the port.
//
// DEMO_CORE=1 also serves what the `core` worker would on the same host (in
// this repository: workers/core/public/core/ at /core/, and a stand-in for
// GET /api/core/modules), so the shared admin bar appears (GO-146);
// DEMO_CORE=theme adds a demo site theme as /core/theme.css. Without the
// switch /core/* answers 404, exactly as a site without `core` does.
import { createServer } from "node:http";
import { readdir, readFile } from "node:fs/promises";
import { extname, join, normalize, relative } from "node:path";
import { fileURLToPath } from "node:url";

const here = fileURLToPath(new URL(".", import.meta.url));
const PUBLIC = join(here, "..", "public");
const FUNCTIONS = join(here, "..", "functions");
const CORE = join(here, "..", "..", "core", "public", "core");
const PORT = Number(process.env.PORT || 8788);
const DEMO_CORE = process.env.DEMO_CORE || "";

const { fakeD1, fakeKV, fakeR2 } = await import("./fakes.ts");
const { seedDemoShop } = await import("./demo-seed.ts");

const env = {
  SHOP_DB: fakeD1(),
  SHOP_FILES: fakeR2(),
  SHOP_KV: fakeKV(),
  SHOP_ADMIN_PASSWORD: "demo",
  SHOP_IP_SALT: "demo-salt",
  SHOP_GATEWAYS: "stripe,paypal",
  SHOP_PAYPAL_ENV: "sandbox",
  STRIPE_SECRET_KEY: "sk_test_demo",
  STRIPE_WEBHOOK_SECRET: "whsec_demo",
  PAYPAL_CLIENT_ID: "demo-client",
  PAYPAL_CLIENT_SECRET: "demo-secret",
  PAYPAL_WEBHOOK_ID: "demo-webhook",
};
await seedDemoShop(env);

// The route table, built from the file tree as Pages builds it: [name] is a
// parameter, index.ts its directory, a leading underscore no route at all.
async function routes(dir = FUNCTIONS) {
  const out = [];
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) out.push(...(await routes(full)));
    else if (entry.name.endsWith(".ts") && !entry.name.startsWith("_")) {
      const path = "/" + relative(FUNCTIONS, full).replace(/\.ts$/, "").replace(/(^|\/)index$/, "");
      const names = [];
      const pattern = path.replace(/\[([^\]]+)\]/g, (_, name) => (names.push(name), "([^/]+)"));
      out.push({ regex: new RegExp(`^${pattern}/?$`), names, module: await import(full), static: !path.includes("[") });
    }
  }
  // A fixed segment wins over a parameter, as in Pages.
  return out.sort((a, b) => Number(b.static) - Number(a.static));
}
const ROUTES = await routes();
const { onRequest: adminMiddleware } = await import(join(FUNCTIONS, "api/shop/admin/_middleware.ts"));

// What `core` answers for the bar (GO-145's GET /api/core/modules, password mode).
const MODULES = {
  mode: "password", user: null, lang: "en", home: "/core/", signOut: null,
  items: [
    { id: "ecommerce", kind: "pages-function", title: "Shop", panel: "/ecommerce-admin/", nav: [], status: "registered" },
    { id: "comments", kind: "pages-function", title: "Comments", panel: "/comments-admin.html", nav: [], status: "registered" },
  ],
};
// A demo site theme, custom properties only, as GO-152's theme.css is.
const DEMO_THEME = `:root { --primary: #283593; --primary-soft: #e8eaf6; --on-primary-soft: #1a237e; --focus: #c51162; --link: #c51162; --nav-bg: #1a237e; --nav-hover: #283593; --nav-muted: #c5cae9; --nav-control: #9fa8da; --nav-current: #ffd54f; }
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) { --primary: #9fa8da; --on-primary: #1a237e; --primary-soft: #1a237e; --on-primary-soft: #c5cae9; --focus: #ffd54f; --link: #ffd54f; } }
`;

const TYPES = { ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8", ".svg": "image/svg+xml" };
async function file(root, path) {
  const target = normalize(join(root, path.endsWith("/") ? path + "index.html" : path));
  if (!target.startsWith(root)) return null;
  try {
    return new Response(await readFile(target), { headers: { "content-type": TYPES[extname(target)] ?? "application/octet-stream" } });
  } catch {
    return null;
  }
}

async function api(request, path) {
  for (const route of ROUTES) {
    const m = route.regex.exec(path);
    if (!m) continue;
    const params = Object.fromEntries(route.names.map((n, i) => [n, decodeURIComponent(m[i + 1])]));
    const verb = `onRequest${request.method[0]}${request.method.slice(1).toLowerCase()}`;
    const handler = route.module[verb] ?? route.module.onRequest;
    if (!handler) return new Response("method not allowed", { status: 405 });
    const pending = [];
    const context = { request, env, params, data: {}, functionPath: path, waitUntil: (p) => pending.push(p), passThroughOnException() {} };
    const run = () => handler({ ...context, next: async () => new Response("not found", { status: 404 }) });
    const res = path.startsWith("/api/shop/admin/") ? await adminMiddleware({ ...context, next: run }) : await run();
    await Promise.allSettled(pending);
    return res;
  }
  return new Response(JSON.stringify({ error: "not_found" }), { status: 404 });
}

async function handle(request) {
  const raw = new URL(request.url).pathname;
  const path = decodeURIComponent(raw);
  // Routes match the path as sent, as Pages matches it: an invoice number's
  // %2F stays inside its one segment, and each parameter is decoded alone.
  if (path.startsWith("/api/") && !path.startsWith("/api/core/")) return api(request, raw);
  if (DEMO_CORE && path === "/api/core/modules") return Response.json(MODULES);
  if (DEMO_CORE === "theme" && path === "/core/theme.css") return new Response(DEMO_THEME, { headers: { "content-type": TYPES[".css"] } });
  if (DEMO_CORE && path.startsWith("/core/")) return (await file(CORE, path.slice("/core/".length))) ?? new Response("not found", { status: 404 });
  if (path === "/") return new Response(null, { status: 302, headers: { location: "/ecommerce-admin/" } });
  return (!path.startsWith("/core/") && (await file(PUBLIC, path))) || new Response("not found", { status: 404, headers: { "content-type": "text/plain" } });
}

createServer(async (req, res) => {
  const chunks = [];
  for await (const c of req) chunks.push(c);
  const headers = new Headers();
  for (const [k, v] of Object.entries(req.headers)) if (typeof v === "string") headers.set(k, v);
  const request = new Request(`http://localhost:${PORT}${req.url}`, { method: req.method, headers, body: ["GET", "HEAD"].includes(req.method) ? undefined : Buffer.concat(chunks) });
  let response;
  try {
    response = await handle(request);
  } catch (e) {
    console.error(e);
    response = new Response("dev server error", { status: 500 });
  }
  res.writeHead(response.status, Object.fromEntries(response.headers));
  res.end(Buffer.from(await response.arrayBuffer()));
}).listen(PORT, () => console.log(`shop demo: http://localhost:${PORT}/ecommerce-admin/  (password: demo${DEMO_CORE ? `, core: ${DEMO_CORE}` : ""})`));
