// Local preview of the sign-in pages against the real worker code, with
// invented data.
//
//   node --import ./workers/auth/test/resolve.mjs workers/auth/test/dev-server.mjs
//   → http://localhost:8788/auth/login   (owner@example.com or editor@example.com,
//                                         password "demo password 1")
//
// What runs is the worker's own Pages handlers (functions/) on an in-memory
// SQLite database (test/fakes.ts fakeD1) seeded by test/demo-seed.ts; the
// static files come from public/. Mail "goes" to Resend, answered from here
// and kept in memory: GET /__demo shows the demo invitation and reset links
// and the mails sent so far (a developer page; the worker has no such route).
// Nothing leaves the machine. PORT=… changes the port; NOMAIL=1 runs without
// a mail transport (no "forgot password").
//
// Login providers (GO-151): the five are on, answered by the scripted fakes
// of test/providers-fake.ts. A provider button leads to a stand-in consent
// screen at /__provider (a developer page) where you pick who you are at the
// provider: Eda (linked to editor@example.com at GitHub), Noor (the invited
// address, verified), a stranger, or Cancel. NOPROVIDERS=1 runs without them;
// TURNSTILE=1 adds Cloudflare's always-passing test keys (the widget loads
// from challenges.cloudflare.com — the one thing that leaves the machine).
// MFA=1 turns the second factor on (GO-140): owner@example.com is then led to
// /auth/mfa/enrol after signing in; /__totp?secret=<base32> shows the current
// code for a key (a developer page standing in for the phone).
//
// The admin panel (GO-138): http://localhost:8788/auth/admin/ as the owner.
// test/admin-demo-seed.ts adds about forty people, groups, a custom role,
// invitations, service accounts, sessions and ninety days of audit events;
// Cloudflare Access is "on" with a made-up team so the Access mappings view
// shows (NOACCESS=1 leaves it off). Every handler under functions/ is
// routed (test/dev-routes.mjs). With MFA=1 the owner already has an
// authenticator app: /__demo gives its key, /__totp?secret=… its code.
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";

const here = fileURLToPath(new URL(".", import.meta.url));
const PUBLIC = join(here, "..", "public");
const PORT = Number(process.env.PORT || 8788);
const ORIGIN = `http://localhost:${PORT}`;

const { fakeD1, KEYS } = await import("./fakes.ts");
const { seedDemo, DEMO_KNOWN, DEMO_PASSWORD } = await import("./demo-seed.ts");
const fn = (p) => import(`../functions/${p}.ts`);

const { FakeProviders, PROVIDER_ENV } = await import("./providers-fake.ts");
const { identityInsert } = await import("../functions/api/auth/_provider-identities.ts");
const networkFetch = globalThis.fetch;
const providers = process.env.NOPROVIDERS ? null : new FakeProviders().install();

const mails = [];
const realFetch = globalThis.fetch;
globalThis.fetch = async (url, init) => {
  if (String(url).startsWith("https://api.resend.com/")) {
    const b = JSON.parse(String(init?.body ?? "{}"));
    mails.push({ to: b.to, subject: b.subject, text: b.text });
    return new Response(JSON.stringify({ id: "demo" }), { status: 200 });
  }
  if (String(url).startsWith("https://challenges.cloudflare.com/")) return networkFetch(url, init); // TURNSTILE=1
  return realFetch(url, init);
};

const env = {
  AUTH_DB: fakeD1(),
  AUTH_SIGNING_KEY: KEYS.priv,
  AUTH_PBKDF2_ITERATIONS: "10000",
  AUTH_KNOWN_PERMISSIONS: DEMO_KNOWN,
  ...(process.env.NOMAIL ? {} : { AUTH_RESEND_API_KEY: "re_demo", AUTH_MAIL_FROM: "accounts@example.com", AUTH_MAIL_FROM_NAME: "Example accounts" }),
  ...(providers ? PROVIDER_ENV : {}),
  // Cloudflare's documented test keys: the widget always passes.
  ...(process.env.TURNSTILE ? { AUTH_TURNSTILE_SITE_KEY: "1x00000000000000000000AA", AUTH_TURNSTILE_SECRET: "1x0000000000000000000000000000000AA" } : {}),
  // MFA=1: the second factor on (GO-140), with a break-glass secret to try.
  ...(process.env.MFA ? { AUTH_MFA_KEY: "demo-mfa-key-demo-mfa-key-demo-mfa-key", AUTH_MFA_RESET_TOKEN: "demo-break-glass-token-1234567890" } : {}),
  // STEP_UP_MINUTES=1 makes the step-up window short enough to try it (GO-138).
  ...(process.env.STEP_UP_MINUTES ? { AUTH_STEP_UP_MINUTES: process.env.STEP_UP_MINUTES } : {}),
  // GO-138: Access "on" (a made-up team; signing in through it cannot work here).
  ...(process.env.NOACCESS ? {} : { AUTH_ACCESS_TEAM: "demo-team", AUTH_ACCESS_AUD: "demo-application-audience-tag" }),
};
const demo = await seedDemo(env, Date.now());
const ownerId = (await env.AUTH_DB.prepare("SELECT id FROM users WHERE email = 'owner@example.com'").first()).id;
await (await import("./admin-demo-seed.ts")).seedAdminDemo(env, Date.now(), ownerId);
let ownerTotp = null;
if (process.env.MFA) {
  const { startTotp, confirmTotp } = await import("../functions/api/auth/_mfa-factors.ts");
  const { base32Encode } = await import("../functions/api/auth/_totp.ts");
  const started = await startTotp(env, ownerId, Date.now() - 60_000);
  await confirmTotp(env, ownerId, started.id, 0, "Demo phone", Date.now() - 60_000);
  ownerTotp = base32Encode(started.secret);
}
const editorId = (await env.AUTH_DB.prepare("SELECT id FROM users WHERE email = 'editor@example.com'").first()).id;
if (providers) await identityInsert(env, editorId, "github", { subject: "1001", email: "eda@users.example.com", emailVerified: true, name: "Eda" }, Date.now() - 3 * 86_400_000).run();

// The people the stand-in consent screen offers (numeric subjects: GitHub and
// Facebook use numbers).
const PERSONAS = [
  { label: "Eda — linked to editor@example.com at GitHub", person: { sub: "1001", email: "eda@users.example.com", verified: true, name: "Eda" } },
  { label: "Noor — new.person@example.com, the invited address, verified", person: { sub: "2002", email: "new.person@example.com", verified: true, name: "Noor" } },
  { label: "A stranger — stranger@example.com, not invited", person: { sub: "3003", email: "stranger@example.com", verified: true, name: "Stranger" } },
];
const escHtml = (v) => String(v).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
const devPage = (body) => `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Provider stand-in</title><body>${body}</body></html>`;

// [method, pattern, module, export]; :name captures one path segment.
const ROUTES = [
  ["GET", "/auth", "auth/index", "onRequestGet"],
  ["GET", "/auth/", "auth/index", "onRequestGet"],
  ["GET", "/auth/login", "auth/login", "onRequestGet"],
  ["GET", "/auth/account", "auth/account", "onRequestGet"],
  ["GET", "/auth/invite", "auth/invite", "onRequestGet"],
  ["GET", "/auth/reset", "auth/reset", "onRequestGet"],
  ["GET", "/auth/forgot", "auth/forgot", "onRequestGet"],
  ["POST", "/api/auth/login", "api/auth/login", "onRequestPost"],
  ["POST", "/api/auth/logout", "api/auth/logout", "onRequestPost"],
  ["POST", "/api/auth/logout-all", "api/auth/logout-all", "onRequestPost"],
  ["POST", "/api/auth/password", "api/auth/password", "onRequestPost"],
  ["POST", "/api/auth/refresh", "api/auth/refresh", "onRequestPost"],
  ["GET", "/api/auth/session", "api/auth/session", "onRequestGet"],
  ["GET", "/api/auth/resume", "api/auth/resume", "onRequestGet"],
  ["POST", "/api/auth/invite", "api/auth/invite", "onRequestPost"],
  ["POST", "/api/auth/reset", "api/auth/reset", "onRequestPost"],
  ["POST", "/api/auth/forgot", "api/auth/forgot", "onRequestPost"],
  ["GET", "/api/auth/sessions", "api/auth/sessions/index", "onRequestGet"],
  ["POST", "/api/auth/sessions/:id", "api/auth/sessions/[id]", "onRequestPost"],
  ["DELETE", "/api/auth/sessions/:id", "api/auth/sessions/[id]", "onRequestDelete"],
  ["POST", "/api/auth/oauth/:provider/start", "api/auth/oauth/[provider]/start", "onRequestPost"],
  ["GET", "/api/auth/oauth/:provider/callback", "api/auth/oauth/[provider]/callback", "onRequestGet"],
  ["POST", "/api/auth/oauth/:provider/callback", "api/auth/oauth/[provider]/callback", "onRequestPost"],
  ["GET", "/api/auth/methods", "api/auth/methods/index", "onRequestGet"],
  ["POST", "/api/auth/methods/set-password", "api/auth/methods/set-password", "onRequestPost"],
  ["POST", "/api/auth/methods/:id", "api/auth/methods/[id]", "onRequestPost"],
  ["DELETE", "/api/auth/methods/:id", "api/auth/methods/[id]", "onRequestDelete"],
  ["GET", "/api/auth/admin/invitations", "api/auth/admin/invitations/index", "onRequestGet"],
  ["POST", "/api/auth/admin/invitations", "api/auth/admin/invitations/index", "onRequestPost"],
  // GO-140: the second factor (MFA=1).
  ["GET", "/auth/mfa", "auth/mfa/index", "onRequestGet"],
  ["GET", "/auth/mfa/enrol", "auth/mfa/enrol", "onRequestGet"],
  ["GET", "/auth/mfa/step-up", "auth/mfa/step-up", "onRequestGet"],
  ["POST", "/api/auth/mfa/verify", "api/auth/mfa/verify", "onRequestPost"],
  ["POST", "/api/auth/mfa/step-up", "api/auth/mfa/step-up", "onRequestPost"],
  ["POST", "/api/auth/mfa/break-glass", "api/auth/mfa/break-glass", "onRequestPost"],
  ["POST", "/api/auth/mfa/totp/start", "api/auth/mfa/totp/start", "onRequestPost"],
  ["POST", "/api/auth/mfa/totp/confirm", "api/auth/mfa/totp/confirm", "onRequestPost"],
  ["POST", "/api/auth/mfa/backup-codes", "api/auth/mfa/backup-codes", "onRequestPost"],
  ["POST", "/api/auth/mfa/passkey/options", "api/auth/mfa/passkey/options", "onRequestPost"],
  ["POST", "/api/auth/mfa/passkey/register", "api/auth/mfa/passkey/register", "onRequestPost"],
  ["GET", "/api/auth/mfa/factors", "api/auth/mfa/factors/index", "onRequestGet"],
  ["POST", "/api/auth/mfa/factors/:id", "api/auth/mfa/factors/[id]", "onRequestPost"],
  ["DELETE", "/api/auth/mfa/factors/:id", "api/auth/mfa/factors/[id]", "onRequestDelete"],
];

// Everything else under functions/ (GO-138's admin API and panel shell among it).
const known = new Set(ROUTES.map(([m, p]) => `${m} ${p}`));
for (const r of await (await import("./dev-routes.mjs")).discover()) if (!known.has(`${r[0]} ${r[1]}`)) ROUTES.push(r);

function match(method, path) {
  for (const [m, pattern, mod, name] of ROUTES) {
    if (m !== method) continue;
    const a = pattern.split("/");
    const b = path.split("/");
    if (a.length !== b.length) continue;
    const params = {};
    if (a.every((seg, i) => (seg.startsWith(":") ? ((params[seg.slice(1)] = decodeURIComponent(b[i])), true) : seg === b[i]))) return { mod, name, params };
  }
  return null;
}

const TYPES = { ".css": "text/css; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".svg": "image/svg+xml" };

createServer(async (req, res) => {
  try {
    const url = new URL(req.url, ORIGIN);
    if (url.pathname === "/__demo") {
      res.writeHead(200, { "content-type": "application/json; charset=utf-8" });
      res.end(JSON.stringify({ invite: `${ORIGIN}/auth/invite?token=${demo.inviteToken}`, reset: `${ORIGIN}/auth/reset?token=${demo.resetToken}`, password: DEMO_PASSWORD, ownerTotp, mails }, null, 1));
      return;
    }
    if (url.pathname === "/__provider" && providers) {
      // The stand-in consent screen: GET shows the choice, POST answers like
      // the provider would — a redirect to the callback (Apple: its form POST).
      if (req.method === "GET") {
        const to = url.searchParams.get("to") ?? "";
        const host = new URL(to).host;
        const forms = [...PERSONAS.map((p, i) => [p.label, String(i)]), ["Cancel at the provider", "cancel"]]
          .map(([label, v]) => `<form method="post" action="/__provider"><input type="hidden" name="to" value="${escHtml(to)}"><input type="hidden" name="who" value="${v}"><button type="submit">${escHtml(label)}</button></form>`)
          .join("");
        res.writeHead(200, { "content-type": "text/html; charset=utf-8" });
        res.end(devPage(`<h1>Stand-in for ${escHtml(host)}</h1><p>Who are you at this provider?</p>${forms}`));
        return;
      }
      const chunks = [];
      for await (const c of req) chunks.push(c);
      const f = new URLSearchParams(Buffer.concat(chunks).toString());
      const to = new URL(f.get("to"));
      const q = to.searchParams;
      if (f.get("who") === "cancel") {
        const back = new URL(q.get("redirect_uri"));
        back.searchParams.set("state", q.get("state"));
        back.searchParams.set("error", q.get("response_mode") === "form_post" ? "user_cancelled_authorize" : "access_denied");
        res.writeHead(303, { location: back.pathname + back.search });
        res.end();
        return;
      }
      const back = await providers.authorize(to.href, PERSONAS[Number(f.get("who"))].person);
      if (back.form) {
        const fields = Object.entries(back.form).map(([k, v]) => `<input type="hidden" name="${k}" value="${escHtml(v)}">`).join("");
        res.writeHead(200, { "content-type": "text/html; charset=utf-8" });
        res.end(devPage(`<p>Apple posts its answer back:</p><form method="post" action="${escHtml(back.path)}">${fields}<button type="submit">Continue</button></form>`));
        return;
      }
      res.writeHead(303, { location: back.path });
      res.end();
      return;
    }
    if (url.pathname === "/__totp" && process.env.MFA) {
      const { base32Decode, hotp, stepAt } = await import("../functions/api/auth/_totp.ts");
      res.writeHead(200, { "content-type": "text/plain; charset=utf-8" });
      res.end(await hotp(base32Decode(url.searchParams.get("secret") ?? "") ?? new Uint8Array(20), stepAt(Date.now())));
      return;
    }
    if (url.pathname.startsWith("/auth/assets/")) {
      const file = normalize(join(PUBLIC, url.pathname));
      if (!file.startsWith(PUBLIC)) throw new Error("outside public/");
      res.writeHead(200, { "content-type": TYPES[extname(file)] ?? "application/octet-stream", "cache-control": "no-store" });
      res.end(await readFile(file));
      return;
    }
    const route = match(req.method, url.pathname);
    if (!route) {
      res.writeHead(404, { "content-type": "text/plain" });
      res.end(url.pathname === "/" ? "The site would be here. Try /auth/login" : "not found");
      return;
    }
    const chunks = [];
    for await (const c of req) chunks.push(c);
    const headers = new Headers();
    for (const [k, v] of Object.entries(req.headers)) if (typeof v === "string") headers.set(k, v);
    headers.set("cf-connecting-ip", "127.0.0.1");
    const body = ["GET", "HEAD"].includes(req.method) ? undefined : Buffer.concat(chunks);
    const request = new Request(url, { method: req.method, headers, body });
    const handler = (await fn(route.mod))[route.name];
    const answer = await handler({ request, env, params: route.params, waitUntil: () => {} });
    const out = {};
    for (const [k, v] of answer.headers) if (k !== "set-cookie") out[k] = v;
    // A redirect to a provider goes to the stand-in consent screen instead.
    if (providers && /^https:\/\//.test(out.location ?? "")) out.location = `/__provider?to=${encodeURIComponent(out.location)}`;
    out["set-cookie"] = answer.headers.getSetCookie();
    res.writeHead(answer.status, out);
    res.end(Buffer.from(await answer.arrayBuffer()));
  } catch (e) {
    console.error(e);
    res.writeHead(500, { "content-type": "text/plain" });
    res.end(String(e));
  }
}).listen(PORT, () => console.log(`auth preview: ${ORIGIN}/auth/login — owner@example.com / "${DEMO_PASSWORD}"; demo links at ${ORIGIN}/__demo`));
