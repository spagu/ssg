// Local preview of the editor panel against the real worker, with an
// invented site. Nothing leaves the machine: GitHub is the in-process fake
// (test/fake-github.ts), sign-in a pretend `auth` worker on the same port,
// and the site's published images (/media/…) are served from the fake
// repository, so the preview shows them as the site would.
//
//   node --import ./workers/editor-ui/test/resolve.mjs workers/editor-ui/test/dev-server.mjs
//   → http://localhost:8789/editor/   (pick an account at /auth/login)
//
// The site: a .ssg.yaml with two languages and a shortcode, five articles
// (one with a component call, raw HTML, a table, a task list and a published
// image), and two publications by two people through the worker itself, so
// History has entries. POST /demo/touch?path=… publishes a change to that
// article as a third person — the "someone else" of a conflict. DEMO_CORE=1
// also serves the `core` worker's /core/ files. PORT=… changes the port.
// /components.json is the site's published component manifest (GO-174's
// Insert list); DEMO_COMPONENTS=0 answers 404 there instead.
//
// Accounts: owner (every permission), writer (read, publish, media),
// texts (read, publish — no images), reader (read only).
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";
import { deflateSync } from "node:zlib";

const here = fileURLToPath(new URL(".", import.meta.url));
const PUBLIC = join(here, "..", "public");
const CORE = join(here, "..", "..", "core", "public", "core");
const PORT = Number(process.env.PORT || 8789);
const BASE = "/editor";

const { handleFetch } = await import("../portable/app.ts");
const { FakeGitHub, TOKEN } = await import("./fake-github.ts");

const TYPES = { ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8", ".svg": "image/svg+xml", ".json": "application/json", ".png": "image/png", ".webp": "image/webp", ".jpg": "image/jpeg", ".gif": "image/gif" };
const ACCOUNTS = {
  owner: ["editor-ui:read", "editor-ui:publish", "editor-ui:media", "editor-ui:delete"],
  writer: ["editor-ui:read", "editor-ui:publish", "editor-ui:media"],
  texts: ["editor-ui:read", "editor-ui:publish"],
  reader: ["editor-ui:read"],
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

// --- a real PNG for the published image ------------------------------------
function png(width, height, pixel) {
  const crcTable = Array.from({ length: 256 }, (_, n) => { let c = n; for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1; return c >>> 0; });
  const crc = (b) => { let c = 0xffffffff; for (const x of b) c = crcTable[(c ^ x) & 0xff] ^ (c >>> 8); return (c ^ 0xffffffff) >>> 0; };
  const chunk = (type, data) => { const out = Buffer.alloc(12 + data.length); out.writeUInt32BE(data.length, 0); out.write(type, 4, "ascii"); data.copy(out, 8); out.writeUInt32BE(crc(out.subarray(4, 8 + data.length)), 8 + data.length); return out; };
  const raw = Buffer.alloc((width * 3 + 1) * height);
  for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) raw.set(pixel(x, y), y * (width * 3 + 1) + 1 + x * 3);
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(width, 0); ihdr.writeUInt32BE(height, 4); ihdr[8] = 8; ihdr[9] = 2;
  return Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), chunk("IHDR", ihdr), chunk("IDAT", deflateSync(raw)), chunk("IEND", Buffer.alloc(0))]);
}
// A kitchen shelf: three jars on a board, under a window.
const SHELF = png(320, 180, (x, y) => (y > 140 ? [121, 85, 72] : y < 40 && x > 200 ? [187, 222, 251] : [[40, 53, 147], [191, 54, 12], [46, 125, 50]].find((_, i) => x > 40 + i * 90 && x < 100 + i * 90 && y > 70) ?? [245, 245, 245]));

// --- the site in a fake GitHub ---------------------------------------------
const gh = new FakeGitHub();
const SITE_YAML = "# Example Kitchen — written by hand\ntitle: Example Kitchen\nsource: blog\nlanguages: [en, pl]\ndefault_language: en\nmermaid: true\nshortcodes:\n  - name: note\n    template: note.html\n    title: Kitchen note\n";
// COMPONENTS: what ssg would publish for three typed components.
const COMPONENTS = {
  schema: 1,
  components: [
    { name: "callout", description: "A boxed remark beside the text.", example: '{{< callout text="\u2026" >}}', props: [
      { name: "kind", type: "string", default: "note", enum: ["note", "warning", "tip"], description: "How loud the box is." },
      { name: "text", type: "string", required: true, description: "What the box says." },
    ] },
    { name: "recipe-card", description: "A recipe summary: time, servings, what it needs.", example: '{{< recipe-card title="\u2026" >}}', props: [
      { name: "minutes", type: "number", default: 30, description: "Total time." },
      { name: "servings", type: "number", description: "How many plates." },
      { name: "tags", type: "array", description: "Words to find it by." },
      { name: "title", type: "string", required: true, description: "The dish." },
      { name: "vegetarian", type: "bool", description: "No meat or fish." },
    ] },
    { name: "youtube", description: "Embed a video from a privacy-friendly host.", example: '{{< youtube id="\u2026" >}}', props: [
      { name: "id", type: "string", required: true, description: "The video id, from the share URL." },
      { name: "ratio", type: "string", default: "16x9", enum: ["16x9", "4x3"] },
    ] },
  ],
};
const FIRST = `---
title: The first week of the kitchen
date: 2026-01-12
description: What we cooked, what burned, and the shelf that holds it all.
tags: [kitchen, notes]
status: publish
author: Ana
---
The kitchen opened on a Monday. **Three** recipes made it to Friday; one did _not_.

![The shelf with three jars](/media/2026/01/shelf-5c1e9a20.png)

## What we cooked

1. Lentil soup
2. Bread, twice
3. A cake that leaned

- [x] Buy jars
- [ ] Fix the window

> Cook what the market has, not what the recipe wants.

| Dish | Minutes |
| --- | ---: |
| Soup | 40 |
| Bread | 180 |

{{< youtube id="abc123" >}}

<div class="note">Raw HTML stays text in the preview.</div>

\`\`\`sh
ssg build
\`\`\`
`;
await gh.store.commitFiles("main", {
  ".ssg.yaml": SITE_YAML,
  "content/blog/posts/first-week.md": FIRST,
  "content/blog/posts/bread-notes.md": "---\ntitle: Notes on bread\ndate: 2026-02-03\ntags: [bread]\nstatus: publish\n---\nFlour, water, salt and **time**.\n",
  "content/blog/posts/zupa.md": "---\ntitle: Zupa z soczewicy\ndate: 2026-02-10\nlang: pl\nstatus: publish\n---\nSoczewica, marchew i cebula.\n",
  "content/blog/posts/menu-ideas.md": "---\ntitle: Menu ideas\nstatus: draft\n---\nA list for later.\n",
  "content/blog/pages/about.md": "---\ntitle: About the kitchen\nstatus: publish\n---\nA small kitchen with a big window.\n",
}, "Start the kitchen site");
{
  const flat = new Map(gh.store.flatAt(gh.store.refs.get("main")));
  flat.set("content/blog/media/2026/01/shelf-5c1e9a20.png", { mode: "100644", sha: await gh.store.putBlob(new Uint8Array(SHELF)) });
  gh.store.refs.set("main", await gh.store.putCommit(await gh.store.putTree(flat), [gh.store.refs.get("main")], "Add the shelf photo"));
}
const env = {
  EDITOR_BASE: BASE,
  EDITOR_GIT_REPO: "github.com/acme/site",
  EDITOR_GIT_BRANCH: "main",
  EDITOR_GIT_TOKEN: TOKEN,
  EDITOR_AUTH_ISSUER: ISSUER,
  EDITOR_AUTH_PUBLIC_KEY: JSON.stringify(pub),
  EDITOR_AUTH_LOGIN_URL: "/auth/login",
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
// deps.fetch: the site's own origin (components.json) is answered here, the
// rest is the fake repository host.
const deps = { fetch: async (u, init) => (String(u).startsWith(ORIGIN + "/") ? (await siteAnswer(new URL(u), "GET")) ?? new Response("not found", { status: 404 }) : gh.fetch(u, init)) };
const ORIGIN = `http://localhost:${PORT}`;

// publishAs sends one publication through the worker, as `as`.
async function publishAs(as, email, message, change) {
  const cookie = `__Host-ssg-auth=${await sign(as, `localhost:${PORT}`, email)}`;
  const path = change.path;
  const read = await handleFetch(new Request(`${ORIGIN}${BASE}/api/content/${encodeURIComponent(path)}`, { headers: { cookie } }), env, deps);
  const { version } = await read.json();
  const res = await handleFetch(new Request(`${ORIGIN}${BASE}/api/changes`, { method: "POST", headers: { cookie, origin: ORIGIN, "content-type": "application/json" }, body: JSON.stringify({ message, changes: [{ op: "put", path, version, ...change }] }) }), env, deps);
  if (!res.ok) console.error("publish failed", res.status, await res.text());
  return res;
}
await publishAs("owner", "ana@example.com", "Update: The first week of the kitchen", { path: "content/blog/posts/first-week.md", fields: { tags: ["kitchen", "notes", "week-one"] } });
await publishAs("writer", "ben@example.com", "Update: Notes on bread", { path: "content/blog/posts/bread-notes.md", body: "Flour, water, salt and **time**.\n\nA starter needs a week.\n" });

// --- the pretend auth worker, the site's media, core's files ----------------
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
async function siteAnswer(url, method) {
  if (url.pathname === "/demo/touch" && method === "POST") {
    const path = url.searchParams.get("path") ?? "content/blog/posts/first-week.md";
    const res = await publishAs("owner", "cleo@example.com", "Update: fix a typo", { path, body: (await (await handleFetch(new Request(`${ORIGIN}${BASE}/api/content/${encodeURIComponent(path)}`, { headers: { cookie: `__Host-ssg-auth=${await sign("owner", url.host)}` } }), env, deps)).json()).body + "\nAdded by someone else.\n" });
    return new Response(null, { status: res.ok ? 204 : 500 });
  }
  if (url.pathname === "/components.json") return process.env.DEMO_COMPONENTS === "0" ? new Response("not found", { status: 404 }) : Response.json(COMPONENTS);
  if (!url.pathname.startsWith("/media/")) return null;
  const sha = gh.store.blobIdAt("main", "content/blog/media/" + decodeURIComponent(url.pathname.slice(7)));
  return sha ? new Response(gh.store.blobs.get(sha), { headers: { "content-type": TYPES[extname(url.pathname)] ?? "application/octet-stream" } }) : new Response("not found", { status: 404 });
}
async function coreAnswer(url) {
  if (!process.env.DEMO_CORE) return url.pathname.startsWith("/core/") ? new Response("not found", { status: 404 }) : null;
  if (url.pathname === "/api/core/modules") return Response.json({ mode: "auth", user: null, lang: "en", home: "/core/", signOut: "/api/auth/logout", items: [{ id: "editor-ui", kind: "worker", title: "Content editor", panel: `${BASE}/`, nav: [], status: "registered" }] });
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
    response = (await coreAnswer(url)) || (await siteAnswer(url, req.method)) || (await authAnswer(url, req.method)) || (await handleFetch(request, env, deps));
  } catch (e) {
    console.error(e);
    response = new Response("dev server error", { status: 500 });
  }
  res.writeHead(response.status, Object.fromEntries(response.headers));
  res.end(Buffer.from(await response.arrayBuffer()));
}).listen(PORT, () => console.log(`editor-ui demo: ${ORIGIN}${BASE}/  (pick an account at ${ORIGIN}/auth/login)`));
