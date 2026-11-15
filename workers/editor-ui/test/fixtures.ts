// Shared setup for the worker's tests: `auth` tokens signed here exactly as
// the `auth` worker signs them (ES256, kid = RFC 7638 thumbprint, the
// contract's claims) without importing workers/auth, an environment trusting
// the key, and a fake GitHub (test/fake-github.ts, in-process) seeded with a
// small site. No network, no real token.
import { FakeGitHub, TOKEN } from "./fake-github.ts";
import { handleFetch } from "../portable/app.ts";
import type { Route } from "../portable/router.ts";
import { clearSiteCache } from "../src/site-model.ts";

export const ORIGIN = "https://example.com";
export const BASE = "/editor";
export const ISSUER = "https://example.com/api/auth";
export const LOGIN = "/auth/login";
export const COOKIE = "__Host-ssg-auth";

const enc = new TextEncoder();
const b64u = (b: Uint8Array) => Buffer.from(b).toString("base64url");
const part = (v: unknown) => b64u(enc.encode(JSON.stringify(v)));
const pair = (await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"])) as CryptoKeyPair;
const jwk = await crypto.subtle.exportKey("jwk", pair.publicKey);
const canonical = { crv: "P-256", kty: "EC", x: jwk.x as string, y: jwk.y as string };
export const PUBLIC_JWK = JSON.stringify({ ...canonical, alg: "ES256", use: "sig" });
const KID = b64u(new Uint8Array(await crypto.subtle.digest("SHA-256", enc.encode(JSON.stringify(canonical)))));

export const AUTH_VARS = { EDITOR_AUTH_ISSUER: ISSUER, EDITOR_AUTH_PUBLIC_KEY: PUBLIC_JWK, EDITOR_AUTH_LOGIN_URL: LOGIN };
export const GIT_VARS = { EDITOR_GIT_REPO: "github.com/acme/site", EDITOR_GIT_BRANCH: "main", EDITOR_GIT_TOKEN: TOKEN };

// token signs an access token for `perms`; kind "service" for a bearer.
export async function token(perms: string[], over: Record<string, unknown> = {}): Promise<string> {
  const now = Math.floor(Date.now() / 1000);
  const claims = { iss: ISSUER, aud: "example.com", sub: "u-ada", sid: "s-1", email: "ada@example.com", iat: now, exp: now + 600, ver: 1, perms, ...over };
  const head = `${part({ alg: "ES256", typ: "JWT", kid: KID })}.${part(claims)}`;
  const sig = new Uint8Array(await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, pair.privateKey, enc.encode(head)));
  return `${head}.${b64u(sig)}`;
}

// Assets: a stand-in for the ASSETS binding that echoes the asked path.
export const assets = { fetch: async (r: Request) => new Response(`asset ${new URL(r.url).pathname}`, { headers: { "content-type": "text/plain" } }) };

export function makeEnv(over: Record<string, unknown> = {}): Record<string, unknown> {
  return { ...AUTH_VARS, ...GIT_VARS, ASSETS: assets, ...over };
}

export type Who = "none" | "user" | "service";
// request builds a request; perms null = no credential. A user token rides in
// the cookie, a service token in Authorization: Bearer.
export async function request(path: string, perms: string[] | null, init: RequestInit & { kind?: "user" | "service"; claims?: Record<string, unknown> } = {}): Promise<Request> {
  const { kind = "user", claims, ...rest } = init;
  const headers = new Headers(rest.headers);
  if (perms) {
    const t = await token(perms, kind === "service" ? { kind: "service", ...claims } : claims);
    if (kind === "service") headers.set("authorization", `Bearer ${t}`);
    else headers.set("cookie", `${COOKIE}=${t}`);
  }
  return new Request(ORIGIN + path, { ...rest, headers });
}

// SITE_YAML: the seeded site's configuration.
export const SITE_YAML = [
  "# the site",
  "source: blog",
  "languages: [en, pl]",
  "default_language: en",
  "shortcodes:",
  "  - name: note",
  "    template: note.html",
  "content_sources:",
  "  - path: docs",
  "    type: page",
  "mddb:",
  "  enabled: false",
  "  api_key: $MDDB_KEY",
  "",
].join("\n");

// site seeds a fake GitHub; extra files are added in the same commit.
export async function site(files: Record<string, string | null> = {}) {
  clearSiteCache();
  const gh = new FakeGitHub();
  await gh.store.commitFiles("main", {
    ".ssg.yaml": SITE_YAML,
    "content/blog/posts/hello.md": "# Hello\n",
    "content/blog/pages/about.md": "# About\n",
    "docs/guide.md": "# Guide\n",
    ...files,
  });
  return gh;
}

// folderHistory: GitHub's commit list filters by a folder as well as a file;
// the fake only knows files. This wraps its fetch to answer a folder's history
// (commits that changed any file under it), newest first.
export function folderHistory(gh: FakeGitHub) {
  return async (url: string, init: RequestInit = {}): Promise<Response> => {
    const u = new URL(url);
    const dir = u.searchParams.get("path") ?? "";
    // Only the commit list (GET …/commits), never POST …/git/commits (GO-170).
    if (!u.pathname.endsWith("/commits") || u.pathname.endsWith("/git/commits") || gh.store.flatAt(gh.store.resolve("main")!).has(dir)) return gh.fetch(url, init);
    const s = gh.store;
    const under = (c: string | undefined) => (c ? JSON.stringify([...s.flatAt(c)].filter(([p]) => p.startsWith(dir + "/"))) : "[]");
    const items = [];
    for (let c = s.resolve(u.searchParams.get("sha")!); c; c = s.commits.get(c)!.parents[0]) {
      if (under(c) !== under(s.commits.get(c)!.parents[0])) items.push({ sha: c, commit: { message: s.commits.get(c)!.message, author: s.commits.get(c)!.author } });
    }
    return Response.json(items.slice(0, Number(u.searchParams.get("per_page"))));
  };
}

// run sends a request through the whole pipeline with the fake GitHub.
export async function run(r: Request, env: Record<string, unknown> = makeEnv(), gh?: FakeGitHub, routes?: Route[]): Promise<Response> {
  return handleFetch(r, env as never, gh ? { fetch: folderHistory(gh) } : {}, routes);
}
