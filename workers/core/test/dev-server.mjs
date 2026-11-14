// Local preview of the admin home page and the shared navigation against the
// real core handlers, with invented modules (test/demo.ts).
//
//   node --import ./workers/core/test/resolve.mjs workers/core/test/dev-server.mjs
//   → http://localhost:8790/core/   (password mode: any user name, password "demo")
//
// What runs: core's own Pages handlers (functions/api/core/*.ts) and its
// public/ files, as Pages would serve them, with a strict CSP on every page.
// The modules are scripted in this process — core's requests to them (and to
// /build.json) are answered by a fetch stand-in, per scenario. Nothing leaves
// the machine. Switches (plain GET, for the browser or a script):
//   /dev/set?scenario=good|todo|down&mode=password|auth&as=owner|limited|manager&core=on|off&theme=none|harbour|lemon|plum&db=on|off|reset
//     auth mode sets a session cookie minted for an owner ("*") or for an
//     account that may only read comments; core=off answers 404 for every
//     /core/ file and /api/core/ route, as on a site without core.
//     theme= puts one of the demo looks (GO-152) in the registry and serves
//     its /core/theme.css, both as ssg's build writes them
//     (test/fixtures/themes/, from internal/admintheme): harbour (dark navy and
//     coral, a logo for light and one for dark backgrounds), lemon (a very
//     light yellow and grey — the colours that need adjusting most), plum (one
//     colour; its logo path is missing, so the bar falls back to the name).
//     db= binds CORE_DB (GO-149): an in-memory SQLite database standing in for
//     D1 — off, the Modules view (/core/#modules) is read-only; reset empties
//     it. as=limited may read comments only; as=manager holds core:manage.
//   /dev/host.html — a bare panel under a strict CSP that adopts the nav with
//     the one-line snippet; /comments-admin.html, /newsletter-admin/,
//     /kitchen/ are the same page standing in for those modules' panels.
// PORT=… changes the port.
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";

const here = fileURLToPath(new URL(".", import.meta.url));
const PUBLIC = join(here, "..", "public");
const PORT = Number(process.env.PORT || 8790);
const ORIGIN = `http://localhost:${PORT}`;

const { useRegistry } = await import("../functions/api/core/_registry.ts");
const { clearStatusCache } = await import("../functions/api/core/_status.ts");
const { CSP } = await import("../functions/api/core/_lib.ts");
const { AUTH_ENV, session } = await import("./harness.ts");
const { fakeD1 } = await import("./d1.ts");
const { REGISTRY, SCENARIOS } = await import("./demo.ts");
const ROUTES = {
  modules: (await import("../functions/api/core/modules.ts")).onRequest,
  status: (await import("../functions/api/core/status.ts")).onRequest,
  overview: (await import("../functions/api/core/overview.ts")).onRequest,
  info: (await import("../functions/api/core/info.ts")).onRequest,
  module: (await import("../functions/api/core/module/index.ts")).onRequest,
  manage: (await import("../functions/api/core/manage/index.ts")).onRequest,
  "manage/order": (await import("../functions/api/core/manage/order.ts")).onRequest,
  "manage/export": (await import("../functions/api/core/manage/export.ts")).onRequest,
  "manage/import": (await import("../functions/api/core/manage/import.ts")).onRequest,
};
const MODULE_SETTINGS = (await import("../functions/api/core/manage/modules/[id].ts")).onRequest;

useRegistry(REGISTRY);
const state = { scenario: "good", mode: "password", as: "owner", core: true, theme: "none", db: false };
let db = fakeD1();
const THEMES = ["harbour", "lemon", "plum"];
const themeFile = (ext) => join(here, "fixtures", "themes", `${state.theme}.${ext}`);
async function applyTheme() {
  useRegistry({ ...REGISTRY, theme: THEMES.includes(state.theme) ? JSON.parse(await readFile(themeFile("json"), "utf8")) : null });
}
const LIMITS = { CORE_PROBE_TIMEOUT_MS: "1500", CORE_STATUS_DEADLINE_MS: "2500" };
const base = () => (state.mode === "auth" ? { ...AUTH_ENV, CORE_AUTH_AUDIENCE: "example.com", ...LIMITS } : { CORE_ADMIN_PASSWORD: "demo", ...LIMITS });
const env = () => (state.db ? { ...base(), CORE_DB: db } : base());
const PERMS = { owner: ["*"], limited: ["comments:read"], manager: ["core:manage", "comments:read"] };

// The modules: core's fetches to this origin are answered from the scenario.
const realFetch = globalThis.fetch;
globalThis.fetch = async (input, init = {}) => {
  const url = new URL(typeof input === "string" ? input : input.url);
  if (url.origin !== ORIGIN) return realFetch(input, init);
  const answer = SCENARIOS[state.scenario]()[url.pathname];
  if (!answer) return new Response("not found", { status: 404 });
  if (answer.delayMs) {
    await new Promise((resolve, reject) => {
      const timer = setTimeout(resolve, answer.delayMs);
      init.signal?.addEventListener("abort", () => (clearTimeout(timer), reject(new DOMException("aborted", "AbortError"))));
    });
  }
  return answer.raw !== undefined ? new Response(answer.raw, { status: answer.status }) : Response.json(answer.body, { status: answer.status });
};

const TYPES = { ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8", ".svg": "image/svg+xml" };
const page = (body, status = 200, type = TYPES[".html"], extra = {}) => new Response(body, { status, headers: { "content-type": type, "content-security-policy": CSP, ...extra } });

async function host(current, title) {
  const html = await readFile(join(here, "fixtures", "host.html"), "utf8");
  return page(html.replaceAll("{{current}}", current).replaceAll("{{title}}", title));
}

async function control(url) {
  const p = url.searchParams;
  for (const k of ["scenario", "mode", "as", "theme"]) if (p.has(k)) state[k] = p.get(k);
  await applyTheme();
  if (p.has("core")) state.core = p.get("core") !== "off";
  if (p.get("db") === "reset") db = fakeD1();
  if (p.has("db")) state.db = p.get("db") !== "off";
  clearStatusCache();
  const headers = {};
  if (state.mode === "auth") headers["set-cookie"] = `${await session(PERMS[state.as] ?? PERMS.owner)}; Path=/; Secure; SameSite=Lax`;
  else headers["set-cookie"] = "__Host-ssg-auth=; Path=/; Secure; Max-Age=0";
  return Response.json({ ...state, db: state.db }, { headers });
}

async function handle(request) {
  const url = new URL(request.url);
  const path = decodeURIComponent(url.pathname);
  if (path === "/dev/set") return control(url);
  if (path === "/favicon.ico") return new Response(null, { status: 302, headers: { location: "/core/img/mark.svg" } });
  if (path === "/dev/host.html") return host("demo", "A panel without core");
  if (path === "/dev/logo.svg" || path === "/dev/logo-dark.svg") return page(await readFile(join(here, "fixtures", path.slice(5))), 200, TYPES[".svg"]);
  if (path === "/core/theme.css" && state.core && THEMES.includes(state.theme)) return page(await readFile(themeFile("css")), 200, TYPES[".css"]);
  if (path === "/dev/host.css") return page(await readFile(join(here, "fixtures", "host.css")), 200, TYPES[".css"]);
  if (path === "/comments-admin.html") return host("comments", "Comments");
  if (path === "/newsletter-admin/") return host("newsletter-ui", "Newsletter");
  if (path === "/kitchen/") return host("kitchen-log", "Kitchen log");
  if (path === "/auth/login") return page("<!doctype html><title>Sign in</title><h1>Sign in (demo)</h1>");
  if (path === "/api/auth/logout" && request.method === "POST") return new Response(null, { status: 303, headers: { location: "/auth/login", "set-cookie": "__Host-ssg-auth=; Path=/; Secure; Max-Age=0" } });
  if (path.startsWith("/api/core/")) {
    const name = path.slice(10);
    const handler = !state.core ? null : name.startsWith("manage/modules/") ? MODULE_SETTINGS : ROUTES[name];
    if (!handler) return new Response("not found", { status: 404 });
    return handler({ request, env: env() });
  }
  if (path === "/") return new Response(null, { status: 302, headers: { location: "/core/" } });
  if (path.startsWith("/core/") && !state.core) return page("not found", 404, "text/plain");
  const file = normalize(join(PUBLIC, path.endsWith("/") ? path + "index.html" : path));
  if (!file.startsWith(PUBLIC)) return page("not found", 404, "text/plain");
  try {
    return page(await readFile(file), 200, TYPES[extname(file)] ?? "application/octet-stream");
  } catch {
    return page("not found", 404, "text/plain");
  }
}

createServer(async (req, res) => {
  const headers = new Headers();
  for (const [k, v] of Object.entries(req.headers)) if (typeof v === "string") headers.set(k, v);
  const chunks = [];
  for await (const c of req) chunks.push(c);
  const body = ["GET", "HEAD"].includes(req.method) ? undefined : Buffer.concat(chunks);
  const request = new Request(`${ORIGIN}${req.url}`, { method: req.method, headers, body });
  let response;
  try {
    response = await handle(request);
  } catch (e) {
    console.error(e);
    response = new Response("dev server error", { status: 500 });
  }
  res.writeHead(response.status, Object.fromEntries(response.headers));
  res.end(Buffer.from(await response.arrayBuffer()));
}).listen(PORT, () => console.log(`core demo: ${ORIGIN}/core/  (password: demo) — switch with ${ORIGIN}/dev/set?scenario=todo&mode=auth`));
