// GET /api/newsletter/module (GO-143): module.json behind the admin gate. It
// refuses exactly as GET /api/newsletter/admin does and needs no database.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as admin from "../functions/api/newsletter/admin.ts";
import * as mod from "../functions/api/newsletter/module/index.ts";
import { sqliteD1, withSchemaV1 } from "./sqlite-d1.ts";

type Fn = (ctx: { request: Request; env: unknown }) => Promise<Response>;
const manifest = JSON.parse(readFileSync(new URL("../module.json", import.meta.url), "utf8"));
const PASSWORD = "admin-pass";
const basic = (p: string) => "Basic " + btoa(`admin:${p}`);

async function call(fn: Fn, path: string, authorization: string | null, env: Record<string, unknown>) {
  const request = new Request(`https://example.com${path}`, { headers: authorization ? { authorization } : {} });
  const res = await fn({ request, env });
  return { status: res.status, headers: [...res.headers.entries()].sort(), body: await res.text() };
}

test("module: with admin credentials it answers module.json, without touching D1", async () => {
  const got = await call(mod.onRequestGet as unknown as Fn, "/api/newsletter/module", basic(PASSWORD), { NEWSLETTER_ADMIN_PASSWORD: PASSWORD });
  assert.equal(got.status, 200);
  assert.deepEqual(got.headers, [["cache-control", "no-store"], ["content-type", "application/json"]]);
  assert.deepEqual(JSON.parse(got.body), manifest);
});

test("module: every refusal is the admin endpoint's, byte for byte", async () => {
  const env = { NEWSLETTER_DB: withSchemaV1(sqliteD1()), NEWSLETTER_ADMIN_PASSWORD: PASSWORD };
  const cases: [Record<string, unknown>, string | null][] = [
    [{ NEWSLETTER_DB: env.NEWSLETTER_DB }, basic(PASSWORD)], // no password configured
    [env, null],
    [env, basic("nope")],
    [env, "Basic %%%"],
    [{ ...env, NEWSLETTER_ACCESS_TEAM: "team", NEWSLETTER_ACCESS_AUD: "aud" }, basic(PASSWORD)], // Access takes over
  ];
  for (const [e, authorization] of cases) {
    const want = await call(admin.onRequestGet as unknown as Fn, "/api/newsletter/admin", authorization, e);
    assert.notEqual(want.status, 200);
    assert.deepEqual(await call(mod.onRequestGet as unknown as Fn, "/api/newsletter/module", authorization, e), want);
  }
});
