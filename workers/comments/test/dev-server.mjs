// Local preview of the moderation panel against the real Pages Functions, with
// invented comments. Nothing leaves the machine.
//
//   node --import ./workers/comments/test/resolve.mjs workers/comments/test/dev-server.mjs
//   → http://localhost:8788/comments-admin.html   (password "demo")
//
// `public/` is served from the root as Pages serves it; /api/comments/admin,
// …/admin/whoami and …/import run the template's own handlers on an in-memory
// SQLite database (test/harness.ts sqliteD1). PORT=… changes the port.
//
// DEMO_CORE=1 also serves what the `core` worker would on the same host (in
// this repository: workers/core/public/core/ at /core/, and a stand-in for
// GET /api/core/modules), so the shared navigation appears (GO-146);
// DEMO_CORE=theme adds a demo site theme as /core/theme.css. Without the
// switch /core/* answers 404, exactly as a site without `core` does.
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";

const here = fileURLToPath(new URL(".", import.meta.url));
const PUBLIC = join(here, "..", "public");
const CORE = join(here, "..", "..", "core", "public", "core");
const PORT = Number(process.env.PORT || 8788);
const DEMO_CORE = process.env.DEMO_CORE || "";

const { sqliteD1 } = await import("./harness.ts");
const ROUTES = {
  "/api/comments/admin": await import("../functions/api/comments/admin.ts"),
  "/api/comments/admin/whoami": await import("../functions/api/comments/admin/whoami.ts"),
  "/api/comments/import": await import("../functions/api/comments/import.ts"),
};

const db = sqliteD1();
const insert = db.sqlite.prepare("INSERT INTO comments (id, url, author, email, body, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)");
for (const [i, [status, url, author, body]] of [
  ["pending", "/blog/winter-menu/", "Ada", "Is the beetroot soup still on the menu in January? We are planning a visit."],
  ["pending", "/blog/pierogi/", "Bartek", "Tried the potato and cheese filling — the dough tore twice. Thinner or thicker next time?"],
  ["pending", "/blog/autumn-meetup/", "Chandra", "Thank you for the recordings.\nCould you post the stew recipe too?"],
  ["spam", "/blog/pierogi/", "Cheap watches", "Best prices at example.net, click now!!!"],
  ["approved", "/blog/winter-menu/", "Dorota", "Lovely photos. See you on Saturday."],
].entries()) insert.run(`demo-${i + 1}`, url, author, `${author.toLowerCase().replace(/\W/g, "")}@example.com`, body, status, `2026-11-${String(10 + i).padStart(2, "0")}T09:${String(15 + i * 7)}:00Z`);
const env = { COMMENTS_DB: db, COMMENTS_ADMIN_PASSWORD: "demo" };

// What `core` answers for the bar (GO-145's GET /api/core/modules, password mode).
const MODULES = {
  mode: "password", user: null, lang: "en", home: "/core/", signOut: null,
  items: [
    { id: "comments", kind: "pages-function", title: "Comments", panel: "/comments-admin.html", nav: [], status: "registered" },
    { id: "newsletter-ui", kind: "worker", title: "Newsletter", panel: "/newsletter-admin/", nav: [], status: "registered" },
  ],
};
// A demo site theme: an Indigo main colour and Pink focus instead of Teal
// and Deep Orange, and links in that accent — custom properties only, as
// GO-152's theme.css is.
const DEMO_THEME = `:root { --primary: #283593; --primary-soft: #e8eaf6; --on-primary-soft: #1a237e; --focus: #c51162; --link: #c51162; --nav-bg: #1a237e; --nav-hover: #283593; --nav-muted: #c5cae9; --nav-control: #9fa8da; --nav-current: #ffd54f; }
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) { --primary: #9fa8da; --on-primary: #1a237e; --primary-soft: #1a237e; --on-primary-soft: #c5cae9; --focus: #ffd54f; --link: #ffd54f; } }
:root[data-theme="dark"] { --primary: #9fa8da; --on-primary: #1a237e; --primary-soft: #1a237e; --on-primary-soft: #c5cae9; --focus: #ffd54f; --link: #ffd54f; }
`;

const TYPES = { ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8", ".svg": "image/svg+xml" };
async function file(root, path) {
  const target = normalize(join(root, path));
  if (!target.startsWith(root)) return null;
  try {
    return new Response(await readFile(target), { headers: { "content-type": TYPES[extname(target)] ?? "application/octet-stream" } });
  } catch {
    return null;
  }
}

async function handle(request) {
  const url = new URL(request.url);
  const path = decodeURIComponent(url.pathname);
  const route = ROUTES[path];
  if (route) {
    const handler = route[`onRequest${request.method[0]}${request.method.slice(1).toLowerCase()}`];
    return handler ? handler({ request, env }) : new Response("method not allowed", { status: 405 });
  }
  if (DEMO_CORE && path === "/api/core/modules") return Response.json(MODULES);
  if (DEMO_CORE === "theme" && path === "/core/theme.css") return new Response(DEMO_THEME, { headers: { "content-type": TYPES[".css"] } });
  if (DEMO_CORE && path.startsWith("/core/")) return (await file(CORE, path.slice("/core/".length))) ?? new Response("not found", { status: 404 });
  if (path === "/") return new Response(null, { status: 302, headers: { location: "/comments-admin.html" } });
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
}).listen(PORT, () => console.log(`comments demo: http://localhost:${PORT}/comments-admin.html  (password: demo${DEMO_CORE ? `, core: ${DEMO_CORE}` : ""})`));
