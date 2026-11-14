// GET /api/core/modules: what each sign-in mode sees, the language of the
// texts, links on this host, and modules described live.
import { after, beforeEach, test } from "node:test";
import assert from "node:assert/strict";
import * as modules from "../functions/api/core/modules.ts";
import { MODULE } from "../functions/api/core/_module.ts";
import { useRegistry } from "../functions/api/core/_registry.ts";
import { clearStatusCache } from "../functions/api/core/_status.ts";
import { pickLang } from "../functions/api/core/_view.ts";
import { AUTH_ENV, ORIGIN, PASSWORD, PW_ENV, basic, call, entry, fakeFetch, freshIP, manifest, registry, session } from "./harness.ts";

const comments = manifest("comments", { nav: [["/comments-admin.html", "comments:read"]], perms: ["comments:read", "comments:moderate"] });
const blog = manifest("blog", { nav: [["/blog-admin.html", "blog:read"], ["/blog-admin.html#/drafts", "blog:edit"]] });
const push = manifest("web-push", { perms: ["web-push:notify"], endpoints: {} });
const plain = manifest("rate-limit", { endpoints: {} });
const ui = manifest("newsletter-ui", { kind: "worker", nav: [["/", "newsletter:read"]], endpoints: { manifest: "/api/module" } });
const UI_URL = `${ORIGIN}/newsletter-admin/api/module`;

const REGISTRY = registry(
  entry("blog", blog),
  entry("comments", comments),
  entry("core", MODULE),
  entry("newsletter-ui", null, "/newsletter-admin"),
  entry("rate-limit", plain),
  entry("web-push", push),
);

beforeEach(() => {
  useRegistry(REGISTRY);
  clearStatusCache();
});
after(() => useRegistry());

const list = async (env: Record<string, unknown>, headers: Record<string, string> = {}, query = "") => {
  const r = await call(modules.onRequest, { path: `/api/core/modules${query}`, headers, env });
  assert.equal(r.status, 200, r.body);
  return r.json();
};
const pw = () => ({ authorization: basic(PASSWORD), "cf-connecting-ip": freshIP() });

test("password mode: every module, with mode and no user; the live one is asked with the caller's credential only", async () => {
  const f = fakeFetch({ [UI_URL]: Response.json(ui) });
  try {
    const headers = { ...pw(), cookie: "a=1", "x-other": "kept out" };
    const got = await list(PW_ENV, headers);
    assert.equal(got.mode, "password");
    assert.equal(got.user, null);
    assert.deepEqual(got.items.map((m: { id: string; status: string }) => `${m.id}:${m.status}`), [
      "blog:registered",
      "comments:registered",
      "core:registered",
      "newsletter-ui:live",
      "rate-limit:registered",
      "web-push:registered",
    ]);
    assert.deepEqual(f.sent, [{ url: UI_URL, method: "GET", headers: { authorization: headers.authorization, cookie: "a=1" }, redirect: "manual" }]);
    const nl = got.items.find((m: { id: string }) => m.id === "newsletter-ui");
    assert.deepEqual([nl.kind, nl.panel, nl.nav[0].path, nl.version], ["worker", "/newsletter-admin/", "/newsletter-admin/", "1.2.3"]);
    const b = got.items[0];
    assert.deepEqual(b, {
      id: "blog",
      kind: "pages-function",
      title: "blog title",
      description: "blog description",
      version: "1.2.3",
      panel: "/blog-admin.html",
      nav: [
        { label: "blog /blog-admin.html", path: "/blog-admin.html", permission: "blog:read" },
        { label: "blog /blog-admin.html#/drafts", path: "/blog-admin.html#/drafts", permission: "blog:edit" },
      ],
      status: "registered",
    });
  } finally {
    f.restore();
  }
});

test("a module that refuses the caller is left out; one that cannot be asked is shown without links", async () => {
  for (const [answer, shown] of [
    [new Response("no", { status: 403 }), null],
    [new Response("no", { status: 401 }), null],
    [new Response("down", { status: 502 }), "unreachable"],
    [Response.json({ id: "newsletter-ui" }), "invalid_manifest"],
  ] as const) {
    clearStatusCache();
    const f = fakeFetch({ [UI_URL]: answer });
    try {
      const nl = (await list(PW_ENV, pw())).items.find((m: { id: string }) => m.id === "newsletter-ui");
      if (shown === null) assert.equal(nl, undefined);
      else assert.deepEqual(nl, { id: "newsletter-ui", kind: null, title: "newsletter-ui", description: null, version: null, panel: null, nav: [], status: shown });
    } finally {
      f.restore();
    }
  }
});

test("auth mode: a module is listed only with a permission one of its nav entries needs (or, without nav, one it declares)", async () => {
  const f = fakeFetch({ [UI_URL]: Response.json(ui) });
  try {
    const cases: [string[], string[]][] = [
      [[], []],
      [["comments:moderate"], []], // declared, but no nav entry needs it
      [["comments:read"], ["comments"]],
      [["blog:edit"], ["blog"]],
      [["web-push:notify"], ["web-push"]],
      [["core:manage"], ["core"]],
      [["newsletter:*"], ["newsletter-ui"]],
      [["*"], ["blog", "comments", "core", "newsletter-ui", "web-push"]], // rate-limit declares nothing to hold
    ];
    for (const [perms, want] of cases) {
      clearStatusCache();
      const got = await list(AUTH_ENV, { cookie: await session(perms) });
      assert.equal(got.mode, "auth");
      assert.deepEqual(got.user, { id: "u1", email: "ada@example.com" });
      assert.deepEqual(got.items.map((m: { id: string }) => m.id), want, perms.join(","));
    }
    const [b] = (await list(AUTH_ENV, { cookie: await session(["blog:edit"]) })).items;
    assert.deepEqual(b.nav.map((n: { permission: string }) => n.permission), ["blog:edit"], "nav entries are filtered the same way");
  } finally {
    f.restore();
  }
});

test("auth mode: a module core has no manifest for is not shown, whatever the caller holds", async () => {
  const f = fakeFetch({ [UI_URL]: new Response("down", { status: 500 }) });
  try {
    const ids = (await list(AUTH_ENV, { cookie: await session(["*"]) })).items.map((m: { id: string }) => m.id);
    assert.ok(!ids.includes("newsletter-ui"));
  } finally {
    f.restore();
  }
});

test("language: ?lang=, then Accept-Language, then English; a language a module lacks falls back to English", async () => {
  const withEs = { ...comments, title: { ...comments.title, es: "Comentarios" } };
  useRegistry(registry(entry("comments", withEs)));
  const title = async (query: string, headers: Record<string, string> = {}) => (await list(PW_ENV, { ...pw(), ...headers }, query)).items[0].title;
  assert.equal(await title("?lang=pl"), "comments title (pl)");
  assert.equal(await title("", { "accept-language": "de-DE,de;q=0.9" }), "comments title (de)");
  assert.equal(await title("?lang=es"), "Comentarios");
  assert.equal(await title("?lang=pt-BR"), "comments title");
  assert.equal(await title("?lang=1x", { "accept-language": "*, fr" }), "comments title (fr)");
  assert.equal((await list(PW_ENV, pw(), "?lang=PL")).lang, "pl");
  assert.equal(pickLang(new Request("https://example.com/")), "en");
});

test("an invalid registry manifest shows as invalid_manifest; a base that leaves the host gives no links", async () => {
  const broken = { ...comments, nav: "nope" };
  useRegistry(registry(entry("comments", broken), entry("blog", blog, "//evil.example.com"), entry("shop", manifest("shop"), "/shop/")));
  const got = (await list(PW_ENV, pw())).items;
  assert.deepEqual(got.map((m: { id: string; status: string }) => `${m.id}:${m.status}`), ["comments:invalid_manifest", "blog:registered", "shop:registered"]);
  assert.deepEqual([got[1].panel, got[1].nav], [null, []]);
  assert.equal(got[0].title, "comments");
});
