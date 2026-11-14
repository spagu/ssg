// GET /api/comments/module (GO-143): module.json behind the moderation gate.
// It must refuse exactly as GET /api/comments/admin does, in both auth modes,
// and it needs no database.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as admin from "../functions/api/comments/admin.ts";
import * as mod from "../functions/api/comments/module/index.ts";
import { AUD, JSON_HEADERS, PASSWORD, TEAM, accessIssuer, basic, call, sqliteD1 } from "./harness.ts";

const manifest = readFileSync(new URL("../module.json", import.meta.url), "utf8");
const M = "/api/comments/module";

test("module: with the moderation password it answers module.json, without touching D1", async () => {
  const got = await call(mod.onRequestGet, { path: M, headers: { authorization: basic(PASSWORD) }, env: { COMMENTS_ADMIN_PASSWORD: PASSWORD } });
  assert.deepEqual(got, { status: 200, headers: JSON_HEADERS, body: JSON.stringify(JSON.parse(manifest)) });
});

test("module: Basic mode refusals are the admin endpoint's, byte for byte", async () => {
  const env = { COMMENTS_DB: sqliteD1(), COMMENTS_ADMIN_PASSWORD: PASSWORD };
  const cases: [Record<string, unknown>, Record<string, string>][] = [
    [{ COMMENTS_DB: sqliteD1() }, { authorization: basic(PASSWORD) }],
    [env, {}],
    [env, { authorization: basic("nope") }],
    [env, { authorization: "Basic %%%" }],
  ];
  for (const [e, headers] of cases) {
    const want = await call(admin.onRequestGet, { path: "/api/comments/admin", headers, env: e });
    assert.notEqual(want.status, 200);
    assert.deepEqual(await call(mod.onRequestGet, { path: M, headers, env: e }), want);
  }
});

test("module: Access mode — a valid token gets the manifest, others are refused as by the admin endpoint", async () => {
  const issuer = await accessIssuer();
  const restore = issuer.install();
  try {
    const env = { COMMENTS_ACCESS_TEAM: TEAM, COMMENTS_ACCESS_AUD: AUD, COMMENTS_ADMIN_PASSWORD: PASSWORD };
    const ok = await call(mod.onRequestGet, { path: M, headers: { "cf-access-jwt-assertion": await issuer.token() }, env });
    assert.equal(ok.status, 200);
    assert.deepEqual(JSON.parse(ok.body), JSON.parse(manifest));
    for (const headers of [{}, { authorization: basic(PASSWORD) }, { "cf-access-jwt-assertion": await issuer.token({ aud: ["other"] }) }] as Record<string, string>[]) {
      const want = await call(admin.onRequestGet, { path: "/api/comments/admin", headers, env: { ...env, COMMENTS_DB: sqliteD1() } });
      assert.deepEqual(await call(mod.onRequestGet, { path: M, headers, env }), want);
    }
  } finally {
    restore();
  }
});
