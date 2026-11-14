// GO-128: the panel's files as the worker serves them: index.html carries its
// meta tags and nothing the CSP would block or that loads from elsewhere;
// every file referenced (stylesheets, scripts, module imports, images)
// exists; and the worker really serves them under the base path with its
// security headers.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, normalize } from "node:path";
import { handleFetch } from "../src/app.ts";
import { ensureSchema } from "../src/schema.ts";
import { CSP } from "../src/lib.ts";
import { PASSWORD, fakeCtx, makeEnv, req } from "./fakes.ts";

const PUBLIC = new URL("../public/", import.meta.url).pathname;
const html = readFileSync(join(PUBLIC, "index.html"), "utf8");

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f);
    return statSync(p).isDirectory() ? files(p) : [p];
  });
}

test("index.html: complete meta tags, noindex, both theme colours, no tag manager", () => {
  for (const re of [
    /<meta charset="utf-8">/,
    /<meta name="viewport" content="width=device-width, initial-scale=1">/,
    /<title>[^<]+<\/title>/,
    /<meta name="description" content="[^"]{40,}">/,
    /<meta name="robots" content="noindex, nofollow, noarchive">/,
    /<meta name="referrer" content="no-referrer">/,
    /<meta name="color-scheme" content="light dark">/,
    /<meta name="theme-color" content="#[0-9a-f]{6}" media="\(prefers-color-scheme: light\)">/,
    /<meta name="theme-color" content="#[0-9a-f]{6}" media="\(prefers-color-scheme: dark\)">/,
    /<html lang="en">/,
  ]) {
    assert.match(html, re);
  }
  assert.doesNotMatch(html, /googletagmanager|gtag\(|GTM-/i, "an admin tool is not measured");
});

test("index.html: nothing the CSP blocks, nothing from another origin", () => {
  assert.doesNotMatch(html, /style=/i, "no style attribute");
  assert.doesNotMatch(html, /<style/i, "no style element");
  assert.doesNotMatch(html, /\son[a-z]+=/i, "no inline handler");
  for (const [, type, body] of html.matchAll(/<script([^>]*)>([\s\S]*?)<\/script>/g)) {
    if (/type="application\/json"/.test(type)) assert.doesNotThrow(() => JSON.parse(body));
    else assert.equal(body.trim(), "", "an executable script must be a file (src=)");
  }
  for (const file of files(PUBLIC)) {
    const text = readFileSync(file, "utf8");
    // xmlns names a namespace and is never fetched; everything else must be relative.
    const urls = [...text.matchAll(/(?:src|href|url\()\s*=?\s*["(]?(https?:)?\/\/[^"')\s]+/g)].map((m) => m[0]);
    assert.deepEqual(urls, [], file);
    assert.doesNotMatch(text, /\.innerHTML\s*=|insertAdjacentHTML|document\.write/, `${file} builds DOM from strings`);
  }
});

// GO-146: two references leave this template on purpose — the shared admin
// bar and the site theme, served by the `core` worker from the site root of
// the same host (another worker's files, so not in public/). Only these two,
// only as site-root paths; everything else must exist here.
const FROM_CORE = ["/core/theme.css", "/core/nav.js"];

test("every file a page, stylesheet or module refers to exists", () => {
  const refs: [string, string][] = [];
  const fromCore: string[] = [];
  for (const [, ref] of html.matchAll(/(?:src|href)="([^"#][^"]*)"/g)) {
    if (ref.startsWith("/")) fromCore.push(ref);
    else refs.push([PUBLIC, ref]);
  }
  assert.deepEqual(fromCore, FROM_CORE, "the only absolute references are core's bar and theme");
  for (const file of files(PUBLIC).filter((f) => f.endsWith(".js"))) {
    const text = readFileSync(file, "utf8");
    for (const [, ref] of text.matchAll(/(?:from|import\()\s*["']([^"']+)["']/g)) refs.push([dirname(file), ref]);
    for (const [, ref] of text.matchAll(/src: [`"](img\/[^`"$]+\.svg)[`"]/g)) refs.push([PUBLIC, ref]);
    for (const [, name] of text.matchAll(/img: "([\w-]+)"/g)) refs.push([PUBLIC, `img/${name}.svg`]);
  }
  assert.ok(refs.length > 40);
  for (const [base, ref] of refs) assert.ok(existsSync(normalize(join(base, ref))), `${ref} (from ${base})`);
});

test("GO-146: core's files exist where the panel expects them (in this repository)", { skip: existsSync(new URL("../../core/public/core/", import.meta.url)) ? false : "workers/core is not next to this copy" }, () => {
  for (const ref of FROM_CORE) assert.ok(existsSync(new URL(`../../core/public${ref}`, import.meta.url)), ref);
});

test("GO-146: the shared bar and theme are adopted as core documents, optional and in order", () => {
  // The theme right after the panel's own tokens (so it overrides them) and
  // before every stylesheet that uses them.
  const sheets = [...html.matchAll(/<link rel="stylesheet" href="([^"]+)">/g)].map((m) => m[1]);
  assert.deepEqual(sheets.slice(0, 3), ["css/tokens.css", "/core/theme.css", "css/base.css"]);
  // The one-line adoption, a module script (deferred: it never blocks the
  // panel), right after the skip link so "Skip to content" passes both bars.
  assert.match(html, /<a class="skip" id="skip" href="#main">[^<]+<\/a>\s*(?:<!--[\s\S]*?-->\s*)?<script type="module" src="\/core\/nav\.js"><\/script><ssg-admin-nav current="newsletter-ui"><\/ssg-admin-nav>\s*<div class="shell">/);
  // Two navigation landmarks, two names: the bar's is "Site administration".
  assert.match(html, /<nav id="nav" aria-label="Newsletter sections">/);
});

test("the worker serves the panel and its files behind auth, with the CSP", async () => {
  const assets = {
    async fetch(r: Request) {
      const path = new URL(r.url).pathname;
      const file = join(PUBLIC, path === "/" ? "index.html" : path);
      return existsSync(file) ? new Response(readFileSync(file)) : new Response("not found", { status: 404 });
    },
  };
  const env = makeEnv({ ASSETS: assets });
  await ensureSchema(env as never);
  const get = (path: string, auth?: string) => handleFetch(req("/newsletter-admin" + path, { auth }), env as never, fakeCtx() as never);
  assert.equal((await get("/")).status, 401, "nothing without credentials");
  const page = await get("/", PASSWORD);
  assert.equal(page.status, 200);
  assert.match(page.headers.get("content-security-policy") ?? "", /script-src 'self'/);
  assert.doesNotMatch(page.headers.get("content-security-policy") ?? "", /unsafe-inline/);
  // GO-146: the bar comes from /core/ on the same host — 'self' covers the
  // script, its stylesheet and its request; nothing was loosened for it.
  assert.equal(CSP, "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; manifest-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'");
  assert.match(await page.text(), /js\/main\.js/);
  for (const path of ["/js/main.js", "/css/tokens.css", "/img/mark.svg", "/js/strings/hi.js"]) assert.equal((await get(path, PASSWORD)).status, 200, path);
});
