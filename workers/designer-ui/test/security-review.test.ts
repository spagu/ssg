// GO-183, the security review: concrete attacks a holder of designer-ui:settings
// alone (not sensitive) could publish before the review, each now refused.
// The schema twin (src/config-schema.json) decides the classes; ssg's own
// tests (internal/config/schema_security_test.go) hold the same line.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ADVANCED, SETTINGS, call, site } from "./fixtures.ts";
import { changedFiles, publish } from "./helpers.ts";
import { validateOverlay } from "../src/overlay-validate.ts";

// A file the build reads and renders into the public site: CI's environment
// (/proc/self/environ holds every secret given to the build step) or
// .git/config (the checkout's token) would become a public page.
const EXFILTRATION: { name: string; path: string[]; value: unknown }[] = [
  { name: "a shortcode whose template climbs out of the theme", path: ["shortcodes"], value: [{ name: "env", template: "../../../../../../../../proc/self/environ" }] },
  { name: "a render hook read from an absolute path", path: ["render_hooks"], value: { link: "/proc/self/environ" } },
  { name: "a custom output format whose template is any file", path: ["outputs_custom"], value: [{ name: "leak", suffix: "leak.txt", template: ".git/config" }] },
  // A tag-manager container is JavaScript chosen by whoever owns the id, run on
  // every page at the origin of the admin panels and the auth worker.
  { name: "an analytics id (a third-party script on every page)", path: ["analytics_ids"], value: { gtm: "GTM-ATTACKER" } },
  { name: "the switch that renders recorded tracking snippets", path: ["analytics"], value: true },
  // SR-10 (owner decision 2026-11-15): keys that send visitors elsewhere.
  { name: "a _redirects rule over the login routes", path: ["redirects"], value: [{ from: "/api/auth/*", to: "https://example.net/login", status: 302 }] },
  { name: "a link rewrite to another host", path: ["link_rewrites"], value: { "/": "https://example.net/" } },
  { name: "the site's domain", path: ["domain"], value: "example.net" },
  { name: "a module whose base path is another host", path: ["modules"], value: [{ id: "newsletter-ui", url: "https://example.net/admin" }] },
];

for (const c of EXFILTRATION) {
  test(`designer-ui:settings alone cannot publish ${c.name}`, async () => {
    const gh = await site();
    const head = gh.store.refs.get("main");
    const r = await publish(gh, SETTINGS, [{ path: c.path, value: c.value }]);
    assert.equal(r.status, 403, r.text);
    assert.equal(r.body.permission, "designer-ui:advanced");
    assert.equal(gh.store.refs.get("main"), head, "nothing committed");
    const ok = await publish(gh, ADVANCED, [{ path: c.path, value: c.value }]);
    assert.equal(ok.status, 200, `with designer-ui:advanced it is an ordinary publication: ${ok.text}`);
    assert.deepEqual(changedFiles(gh), [".ssg.designer.json"]);
  });
}

test("template is one folder name: a path out of templates_dir is refused for everyone, as ssg refuses it", async () => {
  const gh = await site();
  for (const bad of ["../../.git", "..", "a/b", "a\\b", "/etc"]) {
    const r = await publish(gh, ADVANCED, [{ path: ["template"], value: bad }]);
    assert.equal(r.status, 422, `${bad}: ${r.text}`);
    assert.ok(r.body.problems.some((p: string) => p.startsWith("template:")), r.text);
  }
  assert.deepEqual(validateOverlay({ template: "my-theme.v2" }), []);
  assert.equal((await publish(gh, SETTINGS, [{ path: ["template"], value: "other" }])).status, 200, "switching to another theme stays plain");
});

// The REST bodies were read whole (request.json()) before the operation's
// permission check: any signed-in account, even one without designer-ui:read,
// could make the Worker parse a body as large as the platform lets through.
// Now the operation's permission is checked before the body is read, and a
// body stops at the MCP endpoint's limit, as editor-ui's do.
test("a REST body: refused unread without the permission, and over 8 MB for anyone", async () => {
  const gh = await site();
  const big = "x".repeat(8 * 1024 * 1024 + 1);
  const ALL = ["designer-ui:*"];
  for (const route of ["/api/settings/publish", "/api/theme/publish", "/api/publish"]) {
    const declared = await call(gh, route, ALL, { method: "POST", body: "{}", headers: { origin: "https://example.com", "content-type": "application/json", "content-length": String(9 * 1024 * 1024) } });
    assert.deepEqual([declared.status, declared.body.error], [413, "too_large"], route);
    const stranger = await call(gh, route, ["other:read"], { method: "POST", body: JSON.stringify({ message: big }), headers: { origin: "https://example.com", "content-type": "application/json" } });
    assert.deepEqual([stranger.status, stranger.body.error], [403, "forbidden"], `${route}: refused before the body is read`);
    const streamed = await call(gh, route, ALL, { method: "POST", body: JSON.stringify({ message: big }), headers: { origin: "https://example.com", "content-type": "application/json" } });
    assert.deepEqual([streamed.status, streamed.body.error], [413, "too_large"], route);
  }
});

// SR-14: a body without Content-Length (chunked) was read whole by
// request.text() before its size was compared with the limit. It is now read
// as a stream with a running byte count and abandoned at the limit, with the
// same 413 answer as a declared length.
test("a REST body sent chunked over 8 MB: 413 before the whole body is read", async () => {
  const gh = await site();
  const chunk = new Uint8Array(64 * 1024).fill(0x20);
  const total = 64 * 1024 * 1024;
  for (const route of ["/api/settings/publish", "/api/theme/publish", "/api/publish"]) {
    let sent = 0;
    const body = new ReadableStream<Uint8Array>({
      pull(c) {
        if (sent >= total) return c.close();
        c.enqueue(chunk);
        sent += chunk.byteLength;
      },
    });
    const init = { method: "POST", body, duplex: "half", headers: { origin: "https://example.com", "content-type": "application/json" } } as RequestInit;
    const r = await call(gh, route, ["designer-ui:*"], init);
    assert.deepEqual([r.status, r.body.error], [413, "too_large"], route);
    assert.ok(sent <= 8 * 1024 * 1024 + 2 * chunk.byteLength, `${route}: read ${sent} bytes of ${total}`);
  }
});
