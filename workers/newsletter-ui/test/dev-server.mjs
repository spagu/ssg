// Local preview of the panel against the real worker, with invented data.
//
//   node --import ./workers/newsletter-ui/test/resolve.mjs workers/newsletter-ui/test/dev-server.mjs
//   → http://localhost:8787/newsletter-admin/   (any user name, password "demo")
//
// What runs is the worker's own fetch handler (src/index.ts) on an in-memory
// SQLite database (test/fakes.ts fakeD1) seeded by test/demo-seed.ts. The
// queue is wired to the real queue consumer, the cron ticks every 15 seconds,
// and mail goes to a pretend SMTP server in this process that accepts with
// small delays and refuses any address containing "bounce" (5.1.1) — so a
// campaign really progresses while you watch. The site's feed is answered
// from here too. Nothing leaves the machine. PORT=… changes the port.
//
// DEMO_AUTH=1 runs it in `auth` mode instead (GO-137): no password; a pretend
// `auth` worker on the same port signs session tokens with a key made at
// start. /auth/login lists accounts with different permissions — owner,
// editor, sender, reader, none — and remembers the pick in a plain `demo-as`
// cookie; each request then reaches the worker carrying a freshly signed
// __Host-ssg-auth cookie for that account (a __Host- cookie cannot be set on
// http://localhost, so the server adds it). Sign-out clears the pick.
//
// DEMO_CORE=1 also answers what the `core` worker would on the same host
// (GO-146): workers/core/public/core/ at /core/ (nav.js, nav.css, theme.css…)
// and a stand-in for GET /api/core/modules — so the shared admin bar appears
// above the panel; in DEMO_AUTH mode it shows the account and Sign out.
// DEMO_CORE=theme serves a demo site theme as /core/theme.css. Without the
// switch /core/* reaches the worker and answers 404, as on a site without
// `core`.
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";
import { registerHooks } from "node:module";

// "cloudflare:sockets" does not exist in Node: hand the worker our fake.
registerHooks({
  resolve: (specifier, context, next) =>
    specifier === "cloudflare:sockets"
      ? { url: "data:text/javascript,export const connect = (...a) => globalThis.demoSmtp(...a);", shortCircuit: true }
      : next(specifier, context),
});

const here = fileURLToPath(new URL(".", import.meta.url));
const PUBLIC = join(here, "..", "public");
const PORT = Number(process.env.PORT || 8787);
const BASE = "/newsletter-admin";

const worker = (await import("../src/index.ts")).default;
const { handleQueue } = await import("../src/jobs.ts");
const { handleDeadLetters } = await import("../src/engine-status.ts");
const { fakeD1, withSubscribers } = await import("./fakes.ts");
const { seedDemo } = await import("./demo-seed.ts");

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const jitter = () => 20 + Math.floor(Math.random() * 60);

// --- a pretend SMTP server -------------------------------------------------
globalThis.demoSmtp = () => smtpSocket(true);
function smtpSocket(greet) {
  const enc = new TextEncoder();
  const dec = new TextDecoder();
  let push = () => {};
  let data = false;
  let buffer = "";
  const reply = async (s) => (await sleep(jitter()), push(s));
  const readable = new ReadableStream({
    start(c) {
      push = (s) => c.enqueue(enc.encode(s));
      if (greet) reply("220 smtp.example.com ESMTP demo server ready\r\n");
    },
  });
  const writable = new WritableStream({
    write(chunk) {
      buffer += dec.decode(chunk);
      if (data) {
        if (!buffer.includes("\r\n.\r\n")) return;
        data = false;
        buffer = "";
        return reply("250 2.0.0 Ok: queued as DEMO" + Math.floor(Math.random() * 1e6) + "\r\n");
      }
      let at;
      while ((at = buffer.indexOf("\r\n")) >= 0) {
        const line = buffer.slice(0, at);
        buffer = buffer.slice(at + 2);
        const verb = line.split(/[ :]/)[0].toUpperCase();
        if (verb === "EHLO") reply("250-smtp.example.com greets you\r\n250-STARTTLS\r\n250-AUTH PLAIN LOGIN\r\n250 8BITMIME\r\n");
        else if (verb === "STARTTLS") reply("220 2.0.0 Ready to start TLS\r\n");
        else if (verb === "AUTH") reply("235 2.7.0 Authentication successful\r\n");
        else if (verb === "MAIL") reply("250 2.1.0 Ok\r\n");
        else if (verb === "RCPT") reply(/bounce/i.test(line) ? "550 5.1.1 <recipient>: Recipient address rejected: User unknown\r\n" : "250 2.1.5 Ok\r\n");
        else if (verb === "DATA") (data = true), reply("354 End data with <CR><LF>.<CR><LF>\r\n");
        else if (verb === "RSET") reply("250 2.0.0 Ok\r\n");
        else if (verb === "QUIT") reply("221 2.0.0 Bye\r\n");
        else reply("502 5.5.2 Error: command not recognized\r\n");
      }
    },
  });
  return { readable, writable, startTls: () => smtpSocket(false), close: async () => {} };
}

// --- the queue, wired to the real consumer ---------------------------------
const attempts = new Map();
const queue = {
  async send(body, o = {}) {
    deliver(body, o.delaySeconds);
  },
  async sendBatch(messages) {
    for (const m of messages) deliver(m.body, m.delaySeconds);
  },
};
function deliver(body, delaySeconds = 0) {
  setTimeout(async () => {
    const n = (attempts.get(body) ?? 0) + 1;
    attempts.set(body, n);
    let retried = null;
    const msg = { id: String(n), timestamp: new Date(), body, attempts: n, ack() {}, retry(o = {}) { retried = o; } };
    await handleQueue({ queue: "ssg-newsletter-send", messages: [msg], ackAll() {}, retryAll() {} }, env);
    if (!retried) return;
    if (n >= 5) await handleDeadLetters({ queue: "ssg-newsletter-send-dlq", messages: [{ ...msg, ack() {}, retry() {} }] }, env);
    else deliver(body, Math.min(retried.delaySeconds ?? 5, 10));
  }, Math.min(delaySeconds, 10) * 1000 + 50);
}

// --- the environment -------------------------------------------------------
const TYPES = { ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8", ".svg": "image/svg+xml", ".json": "application/json" };
const assets = {
  async fetch(request) {
    let path = decodeURIComponent(new URL(request.url).pathname);
    if (path === "/") path = "/index.html";
    const file = normalize(join(PUBLIC, path));
    if (!file.startsWith(PUBLIC)) return new Response("not found", { status: 404 });
    try {
      return new Response(await readFile(file), { headers: { "content-type": TYPES[extname(file)] ?? "application/octet-stream" } });
    } catch {
      return new Response("not found", { status: 404 });
    }
  },
};

const env = {
  NEWSLETTER_DB: withSubscribers(fakeD1()),
  NEWSLETTER_QUEUE: queue,
  ASSETS: assets,
  NEWSLETTER_ADMIN_PASSWORD: "demo",
  NEWSLETTER_UI_BASE: BASE,
  NEWSLETTER_SITE_URL: "https://example.com",
  NEWSLETTER_MAIL_FROM: "news@example.com",
  NEWSLETTER_MAIL_FROM_NAME: "Example Kitchen",
  NEWSLETTER_SMTP_HOST: "smtp.example.com",
  NEWSLETTER_SMTP_PORT: "587",
  NEWSLETTER_SMTP_USER: "mailer@example.com",
  NEWSLETTER_SMTP_PASSWORD: "demo-only-not-a-secret",
  NEWSLETTER_SEND_RATE: process.env.DEMO_RATE || "20",
  NEWSLETTER_SEND_BATCH: "10",
  NEWSLETTER_TRACK_CLICKS: "true",
};
await seedDemo(env);

// --- DEMO_AUTH=1: a pretend `auth` worker --------------------------------
const ACCOUNTS = {
  owner: ["*"],
  editor: ["newsletter:read", "newsletter:edit"],
  sender: ["newsletter:read", "newsletter:send"],
  reader: ["newsletter:read"],
  none: [],
};
const authDemo = process.env.DEMO_AUTH === "1" ? await pretendAuth() : null;
async function pretendAuth() {
  const enc = new TextEncoder();
  const b64u = (b) => Buffer.from(b).toString("base64url");
  const part = (v) => b64u(enc.encode(JSON.stringify(v)));
  const pair = await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"]);
  const jwk = await crypto.subtle.exportKey("jwk", pair.publicKey);
  const pub = { crv: "P-256", kty: "EC", x: jwk.x, y: jwk.y };
  const kid = b64u(new Uint8Array(await crypto.subtle.digest("SHA-256", enc.encode(JSON.stringify(pub)))));
  delete env.NEWSLETTER_ADMIN_PASSWORD;
  Object.assign(env, { NEWSLETTER_AUTH_ISSUER: "http://localhost/api/auth", NEWSLETTER_AUTH_PUBLIC_KEY: JSON.stringify(pub), NEWSLETTER_AUTH_LOGIN_URL: "/auth/login" });
  const sign = async (as, host) => {
    const now = Math.floor(Date.now() / 1000);
    const claims = { iss: env.NEWSLETTER_AUTH_ISSUER, aud: host, sub: `u-${as}`, sid: "demo", email: `${as}@example.com`, iat: now, exp: now + 600, ver: 1, perms: ACCOUNTS[as] };
    const head = `${part({ alg: "ES256", typ: "JWT", kid })}.${part(claims)}`;
    return `${head}.${b64u(new Uint8Array(await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, pair.privateKey, enc.encode(head))))}`;
  };
  const picked = (headers) => /(?:^|;\s*)demo-as=(\w+)/.exec(headers.get("cookie") ?? "")?.[1];
  // answer: the pretend auth worker's own pages, or null to pass the request on.
  async function answer(url, method) {
    const next = url.searchParams.get("next") || BASE + "/";
    if (url.pathname === "/auth/login") {
      const as = url.searchParams.get("as");
      if (as in ACCOUNTS) return new Response(null, { status: 302, headers: { location: next, "set-cookie": `demo-as=${as}; Path=/; HttpOnly; SameSite=Lax` } });
      const links = Object.keys(ACCOUNTS).map((a) => `<li><a href="/auth/login?as=${a}&amp;next=${encodeURIComponent(next)}">${a}</a> — ${ACCOUNTS[a].join(", ") || "no permission"}</li>`).join("");
      return new Response(`<!doctype html><title>Sign in (demo)</title><h1>Sign in as…</h1><ul>${links}</ul>`, { headers: { "content-type": "text/html; charset=utf-8" } });
    }
    if (url.pathname === "/api/auth/logout" && method === "POST") return new Response(null, { status: 303, headers: { location: BASE + "/", "set-cookie": "demo-as=; Path=/; Max-Age=0" } });
    if (url.pathname === "/api/auth/refresh" && method === "POST") return new Response(null, { status: 204 });
    return null;
  }
  // withSession: the request as the worker sees it, carrying the picked account's token.
  async function withSession(headers, host) {
    const as = picked(headers);
    if (as in ACCOUNTS) headers.set("cookie", `${headers.get("cookie")}; __Host-ssg-auth=${await sign(as, host)}`);
  }
  return { answer, withSession };
}

// --- DEMO_CORE: what the `core` worker serves on this host ------------------
const CORE = join(here, "..", "..", "core", "public", "core");
// A demo site theme (GO-152's theme.css will be custom properties only, like
// this): Indigo instead of Teal as the main colour, Pink (Amber on dark) as
// the accent: focus and links.
const DEMO_THEME = `:root { --primary: #283593; --primary-soft: #e8eaf6; --on-primary-soft: #1a237e; --focus: #c51162; --link: #c51162; --info: #283593; --c1: #3949ab;
  --nav-bg: #1a237e; --nav-hover: #283593; --nav-muted: #c5cae9; --nav-control: #9fa8da; --nav-current: #ffd54f; }
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) { --primary: #9fa8da; --on-primary: #1a237e; --primary-soft: #1a237e; --on-primary-soft: #c5cae9; --focus: #ffd54f; --link: #ffd54f; --info: #9fa8da; --c1: #7986cb; } }
:root[data-theme="dark"] { --primary: #9fa8da; --on-primary: #1a237e; --primary-soft: #1a237e; --on-primary-soft: #c5cae9; --focus: #ffd54f; --link: #ffd54f; --info: #9fa8da; --c1: #7986cb; }
`;
const coreDemo = process.env.DEMO_CORE ? coreAnswer : null;
async function coreAnswer(url, headers) {
  if (url.pathname === "/api/core/modules") {
    const as = /(?:^|;\s*)demo-as=(\w+)/.exec(headers.get("cookie") ?? "")?.[1];
    const auth = Boolean(authDemo);
    return Response.json({
      mode: auth ? "auth" : "password", user: auth && as ? { id: `u-${as}`, email: `${as}@example.com` } : null, lang: "en",
      home: "/core/", signOut: auth ? "/api/auth/logout" : null,
      items: [
        { id: "comments", kind: "pages-function", title: "Comments", panel: "/comments-admin.html", nav: [], status: "registered" },
        { id: "newsletter-ui", kind: "worker", title: "Newsletter", panel: `${BASE}/`, nav: [], status: "registered" },
      ],
    });
  }
  if (process.env.DEMO_CORE === "theme" && url.pathname === "/core/theme.css") return new Response(DEMO_THEME, { headers: { "content-type": TYPES[".css"] } });
  if (!url.pathname.startsWith("/core/")) return null;
  const file = normalize(join(CORE, decodeURIComponent(url.pathname.slice("/core/".length))));
  const body = file.startsWith(CORE) ? await readFile(file).catch(() => null) : null;
  return body ? new Response(body, { headers: { "content-type": TYPES[extname(file)] ?? "application/octet-stream" } }) : new Response("not found", { status: 404 });
}

// The site's feed, for "New from a post"; every other outside request fails.
const FEED = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xml:lang="en"><title>Example Kitchen</title><id>https://example.com/</id><updated>2026-11-12T08:00:00Z</updated>
${[["winter-menu", "Winter menu preview", "Six dishes for the cold months, from beetroot soup to a slow-cooked stew."], ["pierogi", "Pierogi, step by step", "The dough, three fillings and how to freeze them."], ["autumn-meetup", "Notes from the autumn meetup", "Photos, the recipes we cooked together and the recordings."], ["opening-hours", "Holiday opening hours", "When the kitchen is open between 20 December and 6 January."]]
  .map(([slug, title, summary], i) => `<entry><title>${title}</title><id>https://example.com/blog/${slug}/</id><link href="https://example.com/blog/${slug}/"/><updated>2026-11-${String(12 - i * 3).padStart(2, "0")}T08:00:00Z</updated><summary>${summary}</summary></entry>`)
  .join("\n")}
</feed>`;
const realFetch = globalThis.fetch;
globalThis.fetch = async (input, init) => {
  const url = new URL(typeof input === "string" ? input : input.url);
  if (url.href === "https://example.com/feed.xml") return new Response(FEED, { headers: { "content-type": "application/atom+xml; charset=utf-8" } });
  if (url.hostname === "localhost") return realFetch(input, init);
  return new Response("the demo server does not reach the internet", { status: 503 });
};

// The cron, every 15 seconds instead of every minute.
const ctx = { waitUntil: (p) => p.catch((e) => console.error(e)), passThroughOnException() {} };
const tick = () => worker.scheduled({ cron: "* * * * *", scheduledTime: Date.now() }, env, ctx);
setInterval(tick, 15_000);
tick();

createServer(async (req, res) => {
  const chunks = [];
  for await (const c of req) chunks.push(c);
  const body = chunks.length ? Buffer.concat(chunks) : undefined;
  const headers = new Headers();
  for (const [k, v] of Object.entries(req.headers)) if (typeof v === "string") headers.set(k, v);
  headers.set("cf-connecting-ip", "127.0.0.1");
  const url = new URL(`http://localhost:${PORT}${req.url}`);
  if (authDemo) await authDemo.withSession(headers, url.host);
  const request = new Request(url, { method: req.method, headers, body: ["GET", "HEAD"].includes(req.method) ? undefined : body });
  let response;
  try {
    response = (coreDemo && (await coreDemo(url, headers))) || (authDemo && (await authDemo.answer(url, req.method))) || (await worker.fetch(request, env, ctx));
  } catch (e) {
    console.error(e);
    response = new Response("dev server error", { status: 500 });
  }
  if (!authDemo && response.status === 401 && !response.headers.has("www-authenticate")) response.headers.set("www-authenticate", 'Basic realm="newsletter demo"');
  res.writeHead(response.status, Object.fromEntries(response.headers));
  res.end(Buffer.from(await response.arrayBuffer()));
}).listen(PORT, () => console.log(`newsletter-ui demo: http://localhost:${PORT}${BASE}/  (${authDemo ? "auth mode: pick an account at /auth/login" : "password: demo"}${coreDemo ? `, core: ${process.env.DEMO_CORE}` : ""})`));
