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

test("every file a page, stylesheet or module refers to exists", () => {
  const refs: [string, string][] = [];
  for (const [, ref] of html.matchAll(/(?:src|href)="([^"#][^"]*)"/g)) refs.push([PUBLIC, ref]);
  for (const file of files(PUBLIC).filter((f) => f.endsWith(".js"))) {
    const text = readFileSync(file, "utf8");
    for (const [, ref] of text.matchAll(/(?:from|import\()\s*["']([^"']+)["']/g)) refs.push([dirname(file), ref]);
    for (const [, ref] of text.matchAll(/src: [`"](img\/[^`"$]+\.svg)[`"]/g)) refs.push([PUBLIC, ref]);
    for (const [, name] of text.matchAll(/img: "([\w-]+)"/g)) refs.push([PUBLIC, `img/${name}.svg`]);
  }
  assert.ok(refs.length > 40);
  for (const [base, ref] of refs) assert.ok(existsSync(normalize(join(base, ref))), `${ref} (from ${base})`);
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
  assert.match(await page.text(), /js\/main\.js/);
  for (const path of ["/js/main.js", "/css/tokens.css", "/img/mark.svg", "/js/strings/hi.js"]) assert.equal((await get(path, PASSWORD)).status, 200, path);
});
