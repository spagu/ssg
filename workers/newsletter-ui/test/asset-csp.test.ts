// #446: the panel shows stored campaign images (the library's thumbnails,
// the preview), so the panel's img-src names the image store's origin — that
// one origin only, only on the panel's own files, and only when the store
// has an address. The API's JSON keeps the plain policy.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { assetsOrigin } from "../src/asset-config.ts";
import { CSP, cspWithImages } from "../src/lib.ts";
import type { Env } from "../src/lib.ts";
import { PASSWORD, fakeCtx, fakeR2, makeEnv, req } from "./fakes.ts";

const env = (over: Record<string, unknown> = {}) => makeEnv({ NEWSLETTER_SITE_URL: "https://example.com", NEWSLETTER_ASSETS: fakeR2(), ...over }) as unknown as Env;
const csp = async (e: Env, path = "/newsletter-admin/") =>
  (await handleFetch(req(path, { auth: PASSWORD }), e, fakeCtx() as never)).headers.get("content-security-policy");

test("cspWithImages adds exactly one origin to img-src and changes nothing else", () => {
  const out = cspWithImages("https://img.example.net");
  assert.equal(out, CSP.replace("img-src 'self' data:", "img-src 'self' data: https://img.example.net"));
  assert.doesNotMatch(out, /\*|unsafe-/);
});

test("assetsOrigin: the store's own URL, this worker's site, or nothing", () => {
  assert.equal(assetsOrigin(env({ NEWSLETTER_ASSETS_URL: "https://img.example.net/news/" })), "https://img.example.net");
  assert.equal(assetsOrigin(env()), "https://example.com", "served by this worker under the site URL");
  assert.equal(assetsOrigin(env({ NEWSLETTER_ASSETS: undefined })), null, "store off");
  assert.equal(assetsOrigin(env({ NEWSLETTER_ASSETS_URL: "http://img.example.net" })), null, "misconfigured: no address");
});

test("the panel's files carry the store's origin in img-src; the API does not", async () => {
  const own = env({ NEWSLETTER_ASSETS_URL: "https://img.example.net" });
  assert.equal(await csp(own), cspWithImages("https://img.example.net"));
  assert.equal(await csp(own, "/newsletter-admin/js/main.js"), cspWithImages("https://img.example.net"));
  assert.equal(await csp(own, "/newsletter-admin/api/settings"), CSP);
  assert.equal(await csp(env({ NEWSLETTER_ASSETS: undefined })), CSP, "store off: the policy is as before");
  assert.equal(await csp(env({ NEWSLETTER_ASSETS_URL: "ftp://img.example.net" })), CSP, "no usable address: nothing added");
});
