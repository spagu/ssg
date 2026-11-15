// GO-183, the security review of editor-ui: what the review found and fixed,
// each as the attack it stops.
import { test } from "node:test";
import assert from "node:assert/strict";
import manifest from "../module.json" with { type: "json" };
import { merge } from "../portable/config-files.ts";
import { parse } from "../src/site-config.ts";
import { inlineValue } from "../portable/yaml-scalar.ts";
import { BASE, ORIGIN, makeEnv, request, run, site } from "./fixtures.ts";

// ssg renders raw HTML written in Markdown (goldmark WithUnsafe), and the
// panels, the auth worker and the site share one origin: a script published in
// an article runs with the session of every administrator who opens the page,
// and can call designer-ui or auth as them. Publishing content therefore has
// the trust of a theme edit — the permission says so in its description and is
// declared sensitive, so `auth` gives it to a service token only with an
// owner's allowSensitive (and to people under its second-factor policy).
test("editor-ui:publish is declared sensitive, as its own description says", () => {
  const p = manifest.permissions.find((x) => x.name === "editor-ui:publish") as { sensitive?: boolean; description: { en: string } };
  assert.equal(p.sensitive, true);
  assert.match(p.description.en, /raw HTML/);
  for (const name of ["editor-ui:read", "editor-ui:media", "editor-ui:delete"]) {
    assert.notEqual((manifest.permissions.find((x) => x.name === name) as { sensitive?: boolean }).sensitive, true, name);
  }
});

// The overlay is JSON (own "__proto__" keys survive JSON.parse); merging it by
// assignment replaced the prototype of the merged mapping, so a value hidden
// under "__proto__" was read by the site model as if it were a key — one the
// designer's key-by-key checks never saw.
test("merging the overlay stores __proto__ as data, never as the prototype", () => {
  const base = parse(".ssg.yaml", "mddb:\n  enabled: true\n  collection: blog\n");
  const overlay = parse(".ssg.designer.json", '{"mddb": {"__proto__": {"sources": [{"collection": "secret"}]}, "lang": "en"}}');
  const out = merge(base, overlay);
  const mddb = out.mddb as Record<string, unknown>;
  assert.equal(Object.getPrototypeOf(mddb), Object.prototype);
  assert.equal(mddb.sources, undefined);
  assert.deepEqual(Object.keys(mddb).sort(), ["__proto__", "collection", "enabled", "lang"]);
});

// A flow mapping on one line ({k: v}) had the same assignment, and `in`
// counted inherited names, so `{toString: x}` was refused as a duplicate.
test("a flow mapping keeps __proto__ and toString as its own keys", () => {
  const v = inlineValue("{__proto__: {a: 1}, toString: b}", 1) as Record<string, unknown>;
  assert.equal(Object.getPrototypeOf(v), Object.prototype);
  assert.deepEqual(Object.keys(v), ["__proto__", "toString"]);
  assert.deepEqual(Object.getOwnPropertyDescriptor(v, "__proto__")?.value, { a: 1 });
  assert.equal((v as { a?: unknown }).a, undefined);
  assert.throws(() => inlineValue("{a: 1, a: 2}", 3), /twice/);
});

// A route's operation permission is checked before its body is read: an
// account that may only read never makes the Worker parse (here: refuse as
// unsupported or oversized) the body of a publication.
test("POST /api/changes without editor-ui:publish: 403 before the body is read", async () => {
  const gh = await site();
  for (const headers of [{ "content-type": "text/plain" }, { "content-type": "application/json", "content-length": String(9 * 1024 * 1024) }]) {
    const req = await request(BASE + "/api/changes", ["editor-ui:read"], { method: "POST", body: "{nope", headers: { origin: ORIGIN, ...headers } });
    const res = await run(req, makeEnv(), gh);
    assert.deepEqual([res.status, await res.json()], [403, { error: "forbidden", permission: "editor-ui:publish" }]);
  }
});

// SR-14: a body without Content-Length (chunked) was read whole by
// request.text() before its size was compared with the limit. It is now read
// as a stream with a running byte count and abandoned at the limit, with the
// same 413 answer as a declared length.
test("POST /api/changes with a chunked body over 8 MB: 413 before the whole body is read", async () => {
  const gh = await site();
  const chunk = new Uint8Array(64 * 1024).fill(0x20);
  const total = 64 * 1024 * 1024;
  let sent = 0;
  const body = new ReadableStream<Uint8Array>({
    pull(c) {
      if (sent >= total) return c.close();
      c.enqueue(chunk);
      sent += chunk.byteLength;
    },
  });
  const init = { method: "POST", body, duplex: "half", headers: { origin: ORIGIN, "content-type": "application/json" } } as RequestInit;
  const req = await request(BASE + "/api/changes", ["editor-ui:read", "editor-ui:publish"], init);
  assert.equal(req.headers.get("content-length"), null, "no declared length");
  const res = await run(req, makeEnv(), gh);
  assert.deepEqual([res.status, (await res.json()).error], [413, "too_large"]);
  assert.ok(sent <= 8 * 1024 * 1024 + 2 * chunk.byteLength, `read ${sent} bytes of ${total}`);
});
