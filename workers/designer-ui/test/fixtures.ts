// Shared setup for the worker's tests: `auth` tokens signed here exactly as
// the `auth` worker signs them (ES256, kid = RFC 7638 thumbprint, the
// contract's claims) without importing workers/auth, an environment trusting
// the key, and a fake GitHub (test/fake-github.ts, in-process) seeded with a
// small site and its theme. No network, no real token.
import { FakeGitHub, TOKEN } from "./fake-github.ts";
import { handleFetch } from "../src/app.ts";
import { clearConfigCache } from "../src/config-read.ts";
import type { Route } from "../src/router.ts";

export const ORIGIN = "https://example.com";
export const BASE = "/designer";
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

export const AUTH_VARS = { DESIGNER_AUTH_ISSUER: ISSUER, DESIGNER_AUTH_PUBLIC_KEY: PUBLIC_JWK, DESIGNER_AUTH_LOGIN_URL: LOGIN };
export const GIT_VARS = { DESIGNER_GIT_REPO: "github.com/acme/site", DESIGNER_GIT_BRANCH: "main", DESIGNER_GIT_TOKEN: TOKEN };

// Permission sets the tests use.
export const READ = ["designer-ui:read"];
export const SETTINGS = ["designer-ui:read", "designer-ui:settings"];
export const ADVANCED = ["designer-ui:read", "designer-ui:settings", "designer-ui:advanced"];
export const THEME = ["designer-ui:read", "designer-ui:theme"];

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

// request builds a request; perms null = no credential. A user token rides in
// the cookie, a service token in Authorization: Bearer. A JSON body is sent
// from this site's own origin (the CSRF rule).
export async function request(path: string, perms: string[] | null, init: RequestInit & { kind?: "user" | "service"; json?: unknown; claims?: Record<string, unknown> } = {}): Promise<Request> {
  const { kind = "user", json, claims, ...rest } = init;
  const headers = new Headers(rest.headers);
  if (perms) {
    const t = await token(perms, kind === "service" ? { kind: "service", email: "", ...claims } : claims);
    if (kind === "service") headers.set("authorization", `Bearer ${t}`);
    else headers.set("cookie", `${COOKIE}=${t}`);
  }
  if (json !== undefined) {
    headers.set("content-type", "application/json");
    headers.set("origin", ORIGIN);
    return new Request(ORIGIN + path, { method: "POST", ...rest, headers, body: JSON.stringify(json) });
  }
  return new Request(ORIGIN + path, { ...rest, headers });
}

// LITERAL: a secret written by hand into .ssg.yaml — it must never come back.
export const LITERAL = "hunter2-literal-SECRET";

// SITE_YAML: the seeded site's configuration.
export const SITE_YAML = [
  "# the site",
  "title: Example",
  "template: mytheme",
  "source: blog",
  "languages: [en, pl]",
  "mddb:",
  "  enabled: false",
  "  api_key: $MDDB_KEY",
  `jwt_secret: ${LITERAL}   # written by hand, against the advice`,
  "hooks:",
  "  pre_build:",
  "    - echo hi",
  "story: true",
  "",
].join("\n");

export const THEME_FILES: Record<string, string> = {
  "templates/mytheme/index.html": "<!doctype html><title>{{ .Site.Title }}</title>\n",
  "templates/mytheme/css/style.css": "body { color: #222; }\n",
  "templates/mytheme/img/logo.png": "\u0089PNG-not-really",
  "templates/mytheme/notes.txt": "not a theme file by extension\n",
};

// site seeds a fake GitHub; extra files are added in the same commit (null = left out).
export async function site(files: Record<string, string | null> = {}) {
  clearConfigCache();
  const gh = new FakeGitHub();
  const all: Record<string, string> = { ".ssg.yaml": SITE_YAML, "content/blog/posts/hello.md": "# Hello\n", ...THEME_FILES };
  for (const [k, v] of Object.entries(files)) {
    if (v === null) delete all[k];
    else all[k] = v;
  }
  await gh.store.commitFiles("main", all);
  return gh;
}

// run sends a request through the whole pipeline with the fake GitHub.
export async function run(r: Request, env: Record<string, unknown> = makeEnv(), gh?: FakeGitHub, routes?: Route[]): Promise<Response> {
  return handleFetch(r, env as never, gh ? { fetch: gh.fetch } : {}, routes);
}

// call: GET (or POST with `json`) {BASE}{path} and the parsed body.
export async function call(gh: FakeGitHub, path: string, perms: string[] | null, init: Parameters<typeof request>[2] = {}, env = makeEnv()) {
  const res = await run(await request(BASE + path, perms, init), env, gh);
  const text = await res.text();
  return { status: res.status, text, body: text ? JSON.parse(text) : null, headers: res.headers };
}
