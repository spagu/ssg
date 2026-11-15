// Independent security review of the look preview (GO-188, SECURITY-REVIEW.md
// "Preview (GO-188) review"). Each test pins one finding or one property the
// review settled; the browser checks (sandbox + CSP in Chromium and Firefox)
// are described there, not repeated here.
import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { BASE, makeEnv, request } from "./fixtures.ts";
import { handleFetch } from "../portable/app.ts";
import { MAX_PAGE, PreviewError } from "../src/preview-fetch.ts";
import { feedHref, firstEntry } from "../src/preview-latest.ts";
import { sanitize } from "../src/preview-sanitize.ts";
import { PREVIEW_CSP, withPreview } from "../src/preview-entry.ts";
import { tokensFrom } from "../src/preview-tokens.ts";
import { schemeValues } from "../public/js/models/preview-url.js";

const RESOLVE = fileURLToPath(new URL("./resolve.mjs", import.meta.url));
const SANITIZE = new URL("../src/preview-sanitize.ts", import.meta.url).href;
const LATEST = new URL("../src/preview-latest.ts", import.meta.url).href;

// inSmallHeap runs `body` in a Node child whose JS heap is capped at `mb` MiB
// — half of a Worker isolate's 128 MB, the rest being the isolate's own.
function inSmallHeap(mb: number, body: string): { status: number | null; stderr: string } {
  const code = `import { sanitize } from ${JSON.stringify(SANITIZE)}; import { feedHref } from ${JSON.stringify(LATEST)};\n${body}`;
  const r = spawnSync(process.execPath, [`--max-old-space-size=${mb}`, "--import", RESOLVE, "--input-type=module", "-e", code], { encoding: "utf8" });
  return { status: r.status, stderr: r.stderr.slice(-300) };
}

test("SR-21: a page at the size limit is sanitized within a small heap (no token array per page)", () => {
  // 2 MB of "<1" is 2 million text tokens; held in one array that is >130 MiB.
  for (const unit of ["<1", "<a>", "<b c d e f g h i j k l m n o p q r s t u v w x y z>"]) {
    const html = `${JSON.stringify(unit)}.repeat(${Math.floor(MAX_PAGE / unit.length)})`;
    const r = inSmallHeap(64, `const h = ${html}; const out = sanitize(h, { baseHref: "https://example.com/", scheme: null, css: "" }); feedHref(h); if (out.length < h.length / 2) process.exit(3);`);
    assert.equal(r.status, 0, `${unit}: ${r.stderr}`);
  }
  // In this process too: the chunks join back to the same page.
  const many = "<p>x</p>".repeat(5000);
  assert.equal(sanitize(many, { baseHref: "https://example.com/", scheme: null, css: "" }), `<base href="https://example.com/"><style></style>${many}`);
});

test("SR-22: a token value cannot load from another host (no '//' — image-set and friends take strings)", () => {
  const q = (v: string) => new URLSearchParams([["--a", v]]);
  for (const v of ['image-set("//evil.example.com/x.png" 1x)', "image-set('//evil.example.com/x.png')", '-webkit-image-set("//evil.example.com/a")', "src(//evil.example.com/a)", "//evil.example.com"]) {
    assert.throws(() => tokensFrom(q(v)), (e: unknown) => e instanceof PreviewError && e.code === "bad_token", v);
  }
  // Same-host strings stay possible: a relative URL can only reach the site itself.
  assert.deepEqual(tokensFrom(q('"Inter", serif')), [["--a", '"Inter", serif']]);
  assert.deepEqual(tokensFrom(q("rgb(0 0 0 / 50%)")), [["--a", "rgb(0 0 0 / 50%)"]]);
  // The panel leaves the same values out rather than turning the frame into an error page.
  assert.deepEqual(schemeValues([{ name: "--a", light: 'image-set("//evil.example.com/x")' }, { name: "--b", light: "#fff" }], "light"), [["--b", "#fff"]]);
});

test("token values cannot leave the declaration or spell a URL function with escapes", () => {
  const q = (v: string) => new URLSearchParams([["--a", v]]);
  for (const v of ["\\75 rl(x)", "\\75rl(x)", "u\\rl(x)", "red</style>", "a/*x*/b", "a;b", "a{b", "a}b", "@import 'x'", "a:b", "a\nb", "a\u0000b", "red !important", "a<b", "URL(x)", "Expression(x)"]) {
    assert.throws(() => tokensFrom(q(v)), /bad_token/, JSON.stringify(v));
  }
});

test("SR-23: the first feed entry is found in linear time whatever the feed holds", () => {
  const timed = (f: () => unknown): number => {
    const t = performance.now();
    f();
    return performance.now() - t;
  };
  // Before: 256 KiB of unclosed "<item>" took over a second (quadratic).
  const unclosed = "<item>".repeat(Math.floor((512 * 1024) / 6));
  assert.ok(timed(() => assert.equal(firstEntry(unclosed), null)) < 500);
  const links = `<item>${"<link ".repeat(Math.floor((512 * 1024) / 6))}</item>`;
  assert.ok(timed(() => assert.equal(firstEntry(links), null)) < 500);
  // What it reads stays the same.
  assert.equal(firstEntry("<rss><item><title>t</title><link>/a</link></item></rss>"), "/a");
  assert.equal(firstEntry("<FEED><ENTRY><link rel='alternate' href=\"/b?x=1&amp;y=2\"/></ENTRY></FEED>"), "/b?x=1&y=2");
  assert.equal(firstEntry("<rss><item><link></link><link>/c</link></item></rss>"), "/c");
  assert.equal(firstEntry("<rss><item><title>t</title></item><item><link>/late</link></item></rss>"), null, "only the first entry counts");
  assert.equal(firstEntry("<rss><item><link>/cut-short"), "/cut-short", "a feed cut short still names its entry (a path, checked as any)");
  assert.equal(firstEntry("<rss><item><link></item>"), null);
  assert.equal(feedHref("<link rel=alternate type=application/rss+xml href=/r.xml>"), "/r.xml");
});

test("the fetched host is the request's own: Host-like headers of the caller change nothing", async () => {
  const sent: string[] = [];
  const fetch = async (url: string): Promise<Response> => (sent.push(url), new Response("<p>x</p>", { headers: { "content-type": "text/html" } }));
  const env = makeEnv() as never;
  const r = await request(`${BASE}/api/preview?path=/p/`, ["designer-ui:read"], {
    headers: { "x-forwarded-host": "evil.example.com", forwarded: "host=evil.example.com", "x-original-url": "https://evil.example.com/", "x-host": "evil.example.com" },
  });
  const res = await withPreview(r, env, (req) => handleFetch(req, env, { fetch }));
  assert.equal(res.status, 200);
  assert.deepEqual(sent, ["https://example.com/p/"]);
  assert.equal(res.headers.get("referrer-policy"), "no-referrer");
  assert.equal(res.headers.get("x-content-type-options"), "nosniff");
  assert.equal(res.headers.get("content-type"), "text/html; charset=utf-8", "the charset is forced, never sniffed from the page");
});

test("only the exact preview path gets the preview's policy; neighbours keep the strict one", async () => {
  const env = makeEnv() as never;
  for (const p of ["/api/preview/", "/api//preview", "/api/Preview", "/api/%70review"]) {
    const res = await withPreview(await request(`${BASE}${p}`, ["designer-ui:read"]), env, (req) => handleFetch(req, env, {}));
    assert.equal(res.status, 404, p);
    assert.notEqual(res.headers.get("content-security-policy"), PREVIEW_CSP, p);
    assert.match(String(res.headers.get("content-security-policy")), /frame-ancestors 'none'/, p);
  }
});
