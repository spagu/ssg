// The static files of the pages (GO-134): the design tokens are the newsletter
// panel's, byte for byte; nothing loads from elsewhere; the old stop-gap demo
// form is gone.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync, readdirSync } from "node:fs";

const ASSETS = new URL("../public/auth/assets/", import.meta.url);
const PANEL_TOKENS = new URL("../../newsletter-ui/public/css/tokens.css", import.meta.url);
const skip = existsSync(PANEL_TOKENS) ? false : "workers/newsletter-ui is not next to this copy (scaffolded alone): token identity not checked";

test("tokens.css is the newsletter panel's, byte for byte", { skip }, () => {
  assert.ok(readFileSync(new URL("tokens.css", ASSETS)).equals(readFileSync(PANEL_TOKENS)), "copy workers/newsletter-ui/public/css/tokens.css again");
});

test("assets: nothing from another host, no import, no network call, no storage", () => {
  // GO-138: admin/ is the admin panel's files (its own checks: admin-html.test.ts).
  assert.deepEqual(readdirSync(ASSETS).sort(), ["admin", "auth.css", "auth.js", "icon.svg", "tokens.css"]);
  for (const name of ["auth.css", "auth.js", "icon.svg"]) {
    const text = readFileSync(new URL(name, ASSETS), "utf8");
    assert.ok(!/https?:\/\/(?!www\.w3\.org\/2000\/svg)/.test(text), `${name}: a URL to another host`);
    assert.ok(!/@import|fetch\(|XMLHttpRequest|localStorage|sessionStorage|document\.cookie|innerHTML/.test(text), name);
  }
});

test("the stop-gap demo form is gone and nothing refers to it", () => {
  assert.equal(existsSync(new URL("../public/auth-login-form.html", import.meta.url)), false);
  for (const file of ["../README.md", "../wrangler.snippet.toml", "../module.json"]) {
    assert.ok(!readFileSync(new URL(file, import.meta.url), "utf8").includes("auth-login-form"), file);
  }
});
