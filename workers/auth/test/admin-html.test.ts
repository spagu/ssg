// GO-138: the admin panel's shell and files — sign-in and permission at the
// door, the CSP, meta tags, the data block that cannot break out of its
// element, nothing inline or from elsewhere, every referenced file present,
// colours only as tokens, and the shared bar and theme adopted in order.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, normalize } from "node:path";
import { onRequestGet as shell } from "../functions/auth/admin/index.ts";
import { dataBlock } from "../functions/api/auth/_admin-shell.ts";
import { noteKey } from "../functions/api/auth/_admin-keys.ts";
import { T0, call, makeEnv, useClock } from "./fakes.ts";
import { signedIn } from "./seed.ts";
import { ACCESS_ENV } from "./access-fake.ts";

const PUBLIC = new URL("../public/", import.meta.url).pathname;
const ADMIN = join(PUBLIC, "auth/assets/admin");
const files = (dir: string): string[] => readdirSync(dir).flatMap((f) => (statSync(join(dir, f)).isDirectory() ? files(join(dir, f)) : [join(dir, f)]));

async function page(perms: string[] | null, over = {}) {
  useClock();
  const env = makeEnv(over);
  const who = perms ? await signedIn(env, "ada<x>@example.com", perms) : null;
  const res = await call(shell, env, "/auth/admin/", { browser: who?.browser });
  return { res, html: await res.text() };
}

test("shell: no session → through resume to sign in; no auth:admin or auth:invite → 403 in words", async () => {
  const none = await page(null);
  assert.equal(none.res.status, 303);
  assert.equal(none.res.headers.get("location"), "/api/auth/resume?next=%2Fauth%2Fadmin%2F");
  const lacking = await page(["newsletter:read"]);
  assert.equal(lacking.res.status, 403);
  assert.match(lacking.html, /auth:admin/);
  assert.doesNotMatch(lacking.html, /auth-admin-config/);
  assert.equal((await page(["auth:invite"])).res.status, 200);
});

test("shell: CSP with connect-src, meta tags, noindex, no tag manager, data block that cannot break out", async () => {
  const { res, html } = await page(["auth:admin"], ACCESS_ENV);
  assert.equal(res.status, 200);
  const csp = res.headers.get("content-security-policy") ?? "";
  assert.match(csp, /^default-src 'none'; style-src 'self'; script-src 'self'; img-src 'self'; form-action 'self' https:\/\/myteam\.cloudflareaccess\.com; base-uri 'none'; frame-ancestors 'none'; connect-src 'self'$/);
  assert.equal(res.headers.get("referrer-policy"), "strict-origin");
  assert.equal(res.headers.get("x-frame-options"), "DENY");
  for (const re of [/<html lang="en">/, /<meta charset="utf-8">/, /<meta name="viewport" content="width=device-width, initial-scale=1">/, /<title>Accounts admin<\/title>/, /<meta name="description" content="[^"]{60,}">/, /<meta name="robots" content="noindex, nofollow, noarchive">/, /<meta name="color-scheme" content="light dark">/, /<meta name="theme-color" content="#[0-9a-f]{6}" media="\(prefers-color-scheme: light\)">/, /<meta name="theme-color" content="#[0-9a-f]{6}" media="\(prefers-color-scheme: dark\)">/, /<link rel="icon"/]) assert.match(html, re);
  assert.doesNotMatch(html, /googletagmanager|gtag\(|GTM-/i);
  assert.doesNotMatch(html, /style=|<style|\son[a-z]+=/i);
  const block = /<script id="auth-admin-config" type="application\/json">([^<]*)<\/script>/.exec(html);
  assert.ok(block, "the data block holds no < at all");
  const config = JSON.parse((block as RegExpExecArray)[1]);
  assert.equal(config.user.email, "ada<x>@example.com");
  assert.deepEqual([config.access, config.mfa, config.mail, config.tokenMinutes], ["enabled", "off", "none", 10]);
  for (const [, type, body] of html.matchAll(/<script([^>]*)>([\s\S]*?)<\/script>/g)) if (!/application\/json/.test(type)) assert.equal(body.trim(), "");
  assert.equal(dataBlock({ a: "</script>&\u2028" }), '{"a":"\\u003c/script\\u003e\\u0026\\u2028"}');
  // core's bar right after the skip link, its theme right after our tokens.
  assert.match(html, /<a class="skip" id="skip" href="#main">[^<]+<\/a>\n<script type="module" src="\/core\/nav\.js"><\/script><ssg-admin-nav current="auth"><\/ssg-admin-nav>/);
  const sheets = [...html.matchAll(/<link rel="stylesheet" href="([^"]+)">/g)].map((m) => m[1]);
  assert.deepEqual(sheets, ["/auth/assets/tokens.css", "/core/theme.css", "/auth/assets/auth.css", "/auth/assets/admin/css/admin.css", "/auth/assets/admin/css/views.css"]);
  // Sign-out works without the script: a plain form.
  assert.match(html, /<form class="account" method="post" action="\/api\/auth\/logout">/);
});

test("panel files: every file referenced exists; nothing from elsewhere; no HTML from strings; colours only as tokens", async () => {
  const { html } = await page(["auth:admin"]);
  const refs: [string, string][] = [];
  for (const [, ref] of html.matchAll(/(?:src|href)="(\/auth\/assets\/[^"]+)"/g)) refs.push([PUBLIC, ref.slice(1)]);
  for (const file of files(ADMIN)) {
    const text = readFileSync(file, "utf8");
    assert.doesNotMatch(text, /https?:\/\/(?!www\.w3\.org\/2000\/svg")/, `${file}: no URL to another host (the SVG namespace is a name, never fetched)`);
    if (file.endsWith(".js")) {
      assert.doesNotMatch(text, /\.innerHTML\s*=|insertAdjacentHTML|outerHTML|document\.write|eval\(|new Function/, file);
      for (const [, ref] of text.matchAll(/(?:\bfrom\s+|import\()["'](\.[^"'$`]+)["']/g)) refs.push([dirname(file), ref]);
    }
    if (file.endsWith(".css")) assert.doesNotMatch(text.replace(/\/\*[\s\S]*?\*\//g, ""), /#[0-9a-f]{3,8}\b|rgb\(|hsl\(/i, `${file}: a colour outside tokens.css`);
  }
  assert.ok(refs.length > 50, String(refs.length));
  for (const [base, ref] of refs) assert.ok(existsSync(normalize(join(base, ref))), `${ref} (from ${base})`);
  const lazy = readFileSync(join(ADMIN, "js/main.js"), "utf8");
  for (const [, view] of lazy.matchAll(/import\("\.\/views\/([\w-]+)\.js"\)/g)) assert.ok(existsSync(join(ADMIN, "js/views", `${view}.js`)), view);
  for (const lang of ["en", "pl", "hi", "de", "fr"]) assert.ok(existsSync(join(ADMIN, "js/strings", `${lang}.js`)), lang);
});

test("core's files exist where the shell expects them (in this repository)", { skip: existsSync(new URL("../../core/public/core/", import.meta.url)) ? false : "workers/core is not next to this copy" }, () => {
  for (const ref of ["/core/theme.css", "/core/nav.js"]) assert.ok(existsSync(new URL(`../../core/public${ref}`, import.meta.url)), ref);
});

test("noteKey: a failed write never fails the sign-in and is tried again later", async () => {
  const env = makeEnv();
  env.AUTH_DB.state.failOn = /signing_keys/;
  const errors: string[] = [];
  const real = console.error;
  console.error = (m: string) => void errors.push(m);
  await noteKey(env, "kid-1", T0);
  console.error = real;
  assert.deepEqual(errors, ["auth: signing key first use not recorded"]);
  env.AUTH_DB.state.failOn = null;
  const { ensureSchema } = await import("../functions/api/auth/_schema.ts");
  await ensureSchema(env);
  await noteKey(env, "kid-1", T0);
  await noteKey(env, "kid-1", T0 + 1);
  assert.equal(env.AUTH_DB.calls.filter((s) => s.includes("INSERT OR IGNORE INTO signing_keys")).length, 2, "once per isolate after it worked");
});
