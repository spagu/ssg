// GET /api/widgets and the MCP tool widgets_list (GO-174): components from
// the site's published components.json — read safely, any failure an empty
// section with a reason — shortcodes from .ssg.yaml, built-in blocks by the
// site's switches.
import { test } from "node:test";
import assert from "node:assert/strict";
import { BASE, ORIGIN, SITE_YAML, folderHistory, makeEnv, request, site } from "./fixtures.ts";
import type { FakeGitHub } from "./fake-github.ts";
import { tool } from "./mcp-fixtures.ts";
import { mddbOnlyEnv, world, worldFetch } from "./mddb-fixtures.ts";
import { handleFetch } from "../src/app.ts";
import { PERM } from "../src/permissions.ts";
import { MAX_MANIFEST_BYTES, parseManifest, readComponents } from "../src/widget-manifest.ts";
import { blocksFor, featuresOf } from "../src/widgets.ts";

const READ = [PERM.read];
const MANIFEST = {
  schema: 1,
  components: [
    { name: "youtube", description: "A video.", example: '{{< youtube id="\u2026" >}}', props: [
      { name: "id", type: "string", required: true, description: "The video id." },
      { name: "ratio", type: "string", default: "16x9", enum: ["16x9", "4x3"] },
      { name: "autoplay", type: "bool" },
      { name: "start", type: "number", default: 0.5 },
      { name: "tags", type: "array", default: ["a", "b"] },
    ] },
    { name: "aside", example: "{{< aside >}}", props: [] },
  ],
};

type Answer = Response | (() => Response) | "throw";
interface Seen { url: string; init: RequestInit }

// widgets asks GET /api/widgets; the site's own origin answers `answer`.
async function widgets(answer: Answer, env: Record<string, unknown> = {}, gh?: FakeGitHub) {
  const repo = gh ?? (await site());
  const seen: Seen[] = [];
  const fetch = async (url: string, init: RequestInit = {}) => {
    if (!url.startsWith(ORIGIN + "/")) return folderHistory(repo)(url, init);
    seen.push({ url, init });
    if (answer === "throw") throw new TypeError("network");
    return typeof answer === "function" ? answer() : answer;
  };
  const res = await handleFetch(await request(BASE + "/api/widgets", READ), makeEnv(env) as never, { fetch });
  return { status: res.status, body: await res.json(), seen };
}

test("a site with components, shortcodes and every switch: one list, components first", async () => {
  const gh = await site({ ".ssg.yaml": SITE_YAML + "mermaid: true\nmath: true\nshortcode_brackets: true\n" + "" });
  const { status, body, seen } = await widgets(Response.json(MANIFEST), {}, gh);
  assert.equal(status, 200);
  assert.deepEqual(body.items.map((i: { kind: string; name: string }) => `${i.kind}:${i.name}`), [
    "component:aside", "component:youtube", "shortcode:note",
    "block:table", "block:code", "block:mermaid", "block:math", "block:footnote", "block:embed",
  ]);
  const yt = body.items[1];
  assert.deepEqual([yt.title, yt.syntax, yt.props.length], ["youtube", '{{< youtube id="\u2026" >}}', 5]);
  assert.deepEqual(yt.props[0], { name: "id", type: "string", required: true, description: "The video id." });
  assert.deepEqual(yt.props[4], { name: "tags", type: "array", required: false, default: ["a", "b"] });
  assert.equal(body.items[2].description, "{{note}} or [note]");
  assert.deepEqual(body.components, { status: "ok", url: ORIGIN + "/components.json" });
  assert.deepEqual(body.features, { mermaid: true, math: true, shortcodeBrackets: true });
  assert.equal(body.nextCursor, null);
  // The request: the site's origin, no redirects, no credential of the caller.
  assert.equal(seen.length, 1);
  assert.equal(seen[0].init.redirect, "manual");
  assert.equal(new Headers(seen[0].init.headers).get("cookie"), null);
  assert.equal(new Headers(seen[0].init.headers).get("authorization"), null);
});

test("no switches: Mermaid and math are left out; a shortcode's title is shown", async () => {
  const gh = await site({ ".ssg.yaml": SITE_YAML.replace("    template: note.html", "    template: note.html\n    title: A note\n  - name: banner\n    template: banner.html") + "mermaid: yes-ish\n" });
  const { body } = await widgets(Response.json(MANIFEST), {}, gh);
  assert.deepEqual(body.features, { mermaid: false, math: false, shortcodeBrackets: false });
  assert.ok(!body.items.some((i: { name: string }) => i.name === "mermaid" || i.name === "math"));
  assert.deepEqual(body.items.filter((i: { kind: string }) => i.kind === "shortcode").map((i: { name: string }) => i.name), ["banner", "note"]);
  const note = body.items.find((i: { name: string }) => i.name === "note");
  assert.deepEqual([note.title, note.syntax, note.description], ["A note", "{{note}}", "{{note}}"]);
});

test("components.json missing, redirected, failing, unreachable: an empty section with a reason, the rest listed", async () => {
  const cases: [Answer, string, number | undefined][] = [
    [new Response("no", { status: 404 }), "not_published", 404],
    [new Response(null, { status: 302, headers: { location: "https://elsewhere.example.com/c.json" } }), "redirected", 302],
    [new Response("down", { status: 503 }), "unavailable", 503],
    [new Response(null, { status: 200 }), "unavailable", 200],
    ["throw", "unavailable", undefined],
  ];
  for (const [answer, status, httpStatus] of cases) {
    const { body } = await widgets(answer);
    assert.equal(body.components.status, status, status);
    assert.equal(body.components.httpStatus, httpStatus);
    assert.match(body.components.hint, /\S/);
    assert.ok(!body.items.some((i: { kind: string }) => i.kind === "component"));
    assert.ok(body.items.some((i: { kind: string }) => i.kind === "block"));
  }
});

test("too large, by its header or by what arrives; a body that breaks off", async () => {
  const big = "x".repeat(MAX_MANIFEST_BYTES + 1);
  assert.equal((await widgets(new Response(big, { headers: { "content-length": String(big.length) } }))).body.components.status, "too_large");
  const chunks = () => new Response(new ReadableStream({ pull(c) { c.enqueue(new TextEncoder().encode("x".repeat(70_000))); } }));
  assert.equal((await widgets(chunks)).body.components.status, "too_large");
  const broken = () => new Response(new ReadableStream({ pull(c) { c.error(new Error("reset")); } }));
  assert.equal((await widgets(broken)).body.components.status, "unavailable");
});

test("not the manifest ssg writes: invalid, never half a list", async () => {
  for (const text of ["{", "[]", JSON.stringify({ schema: 2, components: [] }), JSON.stringify({ schema: 1 })]) {
    assert.equal((await widgets(new Response(text))).body.components.status, "invalid", text);
  }
  assert.equal(parseManifest(JSON.stringify({ schema: 1, components: [] }))!.length, 0);
  const one = (c: unknown) => parseManifest(JSON.stringify({ schema: 1, components: [c] }));
  assert.deepEqual(one({ name: "plain" }), [{ name: "plain", description: "", example: "{{< plain >}}", props: [] }]);
  assert.deepEqual(one({ name: "p", props: [{ name: "x", default: null }] })![0].props, [{ name: "x", type: "string", required: false }]);
  for (const bad of [
    null, { name: "Bad Name" }, { name: 3 }, { name: "a", description: 1 }, { name: "a", example: 1 }, { name: "a", props: "x" },
    { name: "a", props: [null] }, { name: "a", props: [{ name: "1x" }] }, { name: "a", props: [{ name: "x", type: "date" }] },
    { name: "a", props: [{ name: "x", description: 1 }] }, { name: "a", props: [{ name: "x", enum: [1] }] },
    { name: "a", props: [{ name: "x", default: { a: 1 } }] },
  ]) assert.equal(one(bad), null, JSON.stringify(bad));
});

test("EDITOR_COMPONENTS_URL: another path on the site is used; another host is refused unasked", async () => {
  const at = await widgets(Response.json(MANIFEST), { EDITOR_COMPONENTS_URL: "/blog/components.json" });
  assert.equal(at.seen[0].url, ORIGIN + "/blog/components.json");
  assert.equal(at.body.components.status, "ok");
  const away = await widgets(Response.json(MANIFEST), { EDITOR_COMPONENTS_URL: "https://elsewhere.example.com/components.json" });
  assert.deepEqual([away.body.components.status, away.body.components.url, away.seen.length], ["bad_url", null, 0]);
});

test("a site read from MDDB alone: no shortcodes, the blocks without switches", async () => {
  const w = await world({ git: false });
  const base = worldFetch(w);
  const fetch = async (url: string, init?: RequestInit) => (url.startsWith(ORIGIN + "/") ? new Response("", { status: 404 }) : base(url, init));
  const res = await handleFetch(await request(BASE + "/api/widgets", READ), mddbOnlyEnv() as never, { fetch });
  const body = await res.json();
  assert.deepEqual(body.items.map((i: { name: string }) => i.name), ["table", "code", "footnote", "embed"]);
  assert.equal(body.components.status, "not_published");
});

test("the permission, and the MCP tool answers the same list", async () => {
  const gh = await site();
  const res = await handleFetch(await request(BASE + "/api/widgets", [PERM.publish]), makeEnv() as never, { fetch: folderHistory(gh) });
  assert.equal(res.status, 403);
  const out = await tool(gh, READ, "widgets_list", {});
  assert.equal(out.isError, false);
  assert.equal((out.structuredContent.components as { status: string }).status, "not_published");
  assert.ok((out.structuredContent.items as { name: string }[]).some((i) => i.name === "note"));
});

test("blocksFor and featuresOf without a model", () => {
  assert.deepEqual(featuresOf(null), { mermaid: false, math: false, shortcodeBrackets: false });
  assert.equal(blocksFor({ mermaid: true, math: false, shortcodeBrackets: false }).length, 5);
});

test("without an injected fetch the Worker's own fetch is used", async () => {
  const real = globalThis.fetch;
  const asked: string[] = [];
  globalThis.fetch = (async (u: string) => (asked.push(String(u)), new Response("", { status: 404 }))) as typeof fetch;
  try {
    const out = await readComponents({ env: {}, deps: {}, url: new URL(ORIGIN + BASE + "/api/widgets") } as never);
    assert.deepEqual([out.status, asked], ["not_published", [ORIGIN + "/components.json"]]);
  } finally {
    globalThis.fetch = real;
  }
});
