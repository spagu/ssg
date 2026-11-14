// Regression tests for the security review (workers/auth/docs/SECURITY-REVIEW.md).
import { test } from "node:test";
import assert from "node:assert/strict";
import * as admin from "../functions/api/comments/admin.ts";
import * as importer from "../functions/api/comments/import.ts";
import { normaliseURL } from "../functions/api/comments/_lib.ts";
import { AUD, TEAM, accessIssuer, call, seed, sqliteD1 } from "./harness.ts";

const A = "/api/comments/admin";

test("SR-10: Access mode refuses a cross-site write that carries only the CF_Authorization cookie", async () => {
  const issuer = await accessIssuer();
  const restore = issuer.install();
  try {
    const d = sqliteD1();
    seed(d, [{ id: "c1", status: "pending", created_at: "2000-01-01T00:00:00.000Z" }]);
    const env = { COMMENTS_DB: d, COMMENTS_ACCESS_TEAM: TEAM, COMMENTS_ACCESS_AUD: AUD };
    const cookie = `CF_Authorization=${await issuer.token({ email: "mod@example.com" })}`;
    const forged = { cookie, "content-type": "text/plain", origin: "https://attacker.example.net", "sec-fetch-site": "cross-site" };
    const body = JSON.stringify({ id: "c1", action: "delete" });
    const got = await call(admin.onRequestPost, { method: "POST", path: A, headers: forged, body, env });
    assert.deepEqual([got.status, JSON.parse(got.body)], [403, { error: "cross_origin" }]);
    const noHeader = await call(admin.onRequestPost, { method: "POST", path: A, headers: { cookie, "content-type": "application/json" }, body, env });
    assert.deepEqual([noHeader.status, JSON.parse(noHeader.body)], [403, { error: "csrf_check_failed" }]);
    const imp = await call(importer.onRequestPost, { method: "POST", path: "/api/comments/import", headers: forged, body: '{"items":[]}', env });
    assert.equal(imp.status, 403);
    assert.equal(d.rows().length, 1, "nothing deleted");
    // Reads need no proof; the panel's own write (header, same origin) goes through.
    assert.equal((await call(admin.onRequestGet, { path: A, headers: { cookie }, env })).status, 200);
    const own = { cookie, "content-type": "application/json", "x-ssg-auth": "1", origin: "https://example.com" };
    assert.equal((await call(admin.onRequestPost, { method: "POST", path: A, headers: own, body, env })).status, 200);
  } finally {
    restore();
  }
});

test("SR-11: a comment URL with a control character or a space is refused (it could name another host)", () => {
  for (const raw of ["/\t/evil.example/x", "/\n/evil.example", "/\r/evil.example", "/ /evil.example", "/a\u0000b", "/a\u007fb"]) assert.equal(normaliseURL(raw), null, JSON.stringify(raw));
  assert.equal(normaliseURL("/blog/post/?x=1#c"), "/blog/post/");
});

test("SR-10: the panel sends the CSRF header in every mode (Access needs it on a write)", async () => {
  const { readFileSync } = await import("node:fs");
  const src = readFileSync(new URL("../public/comments-admin.js", import.meta.url), "utf8");
  const body = /function headers\(extra\) \{\n([\s\S]*?)\n  \}/.exec(src)![1];
  for (const mode of ["auth", "password"]) {
    const headers = new Function("mode", "auth", "extra", body) as (m: string, a: string, e?: object) => Record<string, string>;
    assert.equal(headers(mode, "pw")["x-ssg-auth"], "1", mode);
  }
});
