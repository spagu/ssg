// The preview's limits per fetch (GO-188 review, owner decisions on SR-24 and
// SR-25): the page size by plan (Free 256 KiB, Paid 2 MB), declared or
// streamed, for a path and for the latest post; and 10 s per fetch, its
// redirects and body included, answered with our own upstream_timeout page.
import { mock, test } from "node:test";
import assert from "node:assert/strict";
import { BASE, ORIGIN, makeEnv, request } from "./fixtures.ts";
import { handleFetch } from "../portable/app.ts";
import { FETCH_TIMEOUT, MAX_PAGE, PAGE_LIMIT, PreviewError, fetchPage } from "../src/preview-fetch.ts";
import { latestPath } from "../src/preview-latest.ts";
import { LANGS, errorPage } from "../src/preview-messages.ts";
import { withPreview } from "../src/preview-entry.ts";

const HTML = { "content-type": "text/html; charset=utf-8" };
// streamed: `size` bytes in 64 KiB chunks, no Content-Length.
function streamed(size: number): Response {
  let left = size;
  const body = new ReadableStream<Uint8Array>({
    pull(c) {
      const n = Math.min(left, 64 * 1024);
      left -= n;
      if (n) c.enqueue(new Uint8Array(n).fill(0x61));
      else c.close();
    },
  });
  return new Response(body, { headers: HTML });
}
const declared = (size: number): Response => new Response("x", { headers: { ...HTML, "content-length": String(size) } });

async function preview(query: string, answer: () => Response, over: Record<string, unknown> = {}) {
  const env = makeEnv(over) as never;
  const doFetch = async (): Promise<Response> => answer();
  const res = await withPreview(await request(`${BASE}/api/preview${query}`, ["designer-ui:read"]), env, (r) => handleFetch(r, env, { fetch: doFetch }));
  return { status: res.status, text: await res.text() };
}

test("SR-24: the page size by plan — Free 256 KiB, Paid 2 MB — declared or streamed", async () => {
  assert.deepEqual(PAGE_LIMIT, { free: 256 * 1024, paid: MAX_PAGE });
  const free = PAGE_LIMIT.free;
  for (const [answer, plan, status] of [
    [() => streamed(free), {}, 200],
    [() => streamed(free + 1), {}, 413],
    [() => declared(free + 1), {}, 413],
    [() => streamed(free + 1), { DESIGNER_PLAN: "paid" }, 200],
    [() => streamed(MAX_PAGE), { DESIGNER_PLAN: "paid" }, 200],
    [() => streamed(MAX_PAGE + 1), { DESIGNER_PLAN: "paid" }, 413],
    [() => declared(MAX_PAGE + 1), { DESIGNER_PLAN: "paid" }, 413],
  ] as [() => Response, Record<string, unknown>, number][]) {
    const out = await preview("?path=/p/", answer, plan);
    assert.equal(out.status, status, `${JSON.stringify(plan)} -> ${status}`);
    if (status === 413) assert.match(out.text, /DESIGNER_PLAN[\s\S]*<code>page_too_large<\/code>/);
  }
});

test("SR-24: the latest post reads its home page and feed within the same cap", async () => {
  const home = '<link rel="alternate" type="application/rss+xml" href="/rss.xml">';
  const big = home + " ".repeat(PAGE_LIMIT.free);
  const fetchFor = (homeText: string) => async (url: string): Promise<Response> =>
    new URL(url).pathname === "/" ? new Response(homeText, { headers: HTML }) : new Response("<rss><item><link>/p/</link></item></rss>", { headers: { "content-type": "application/rss+xml" } });
  await assert.rejects(latestPath(fetchFor(big), ORIGIN, "/designer", PAGE_LIMIT.free), (e: unknown) => e instanceof PreviewError && e.code === "page_too_large");
  assert.equal(await latestPath(fetchFor(big), ORIGIN, "/designer", PAGE_LIMIT.paid), "/p/");
  const out = await preview("?page=latest", () => new Response(big, { headers: HTML }));
  assert.equal(out.status, 413);
});

test("SR-25: a fetch that never answers, or a body that never ends, stops after FETCH_TIMEOUT", async () => {
  assert.equal(FETCH_TIMEOUT, 10_000);
  mock.timers.enable({ apis: ["setTimeout"] });
  try {
    let signal: AbortSignal | undefined;
    const never = (url: string, init: RequestInit): Promise<Response> => ((signal = init.signal ?? undefined), new Promise<Response>(() => {}));
    const hung = fetchPage(never, ORIGIN, "/");
    await Promise.resolve();
    mock.timers.tick(FETCH_TIMEOUT - 1);
    let settled = false;
    void hung.catch(() => (settled = true));
    await new Promise((r) => setImmediate(r));
    assert.equal(settled, false, "not before the limit");
    mock.timers.tick(1);
    await assert.rejects(hung, (e: unknown) => e instanceof PreviewError && e.code === "upstream_timeout" && e.status === 504);
    assert.equal(signal?.aborted, true, "the subrequest is aborted too");

    const slowBody = async (): Promise<Response> => new Response(new ReadableStream({ pull: () => new Promise<void>(() => {}) }), { headers: HTML });
    const reading = fetchPage(slowBody, ORIGIN, "/");
    for (let i = 0; i < 5; i++) await new Promise((r) => setImmediate(r));
    mock.timers.tick(FETCH_TIMEOUT);
    await assert.rejects(reading, /upstream_timeout/);

    // A fetch that finishes in time leaves no timer behind.
    assert.deepEqual(await fetchPage(async () => new Response("<p>x</p>", { headers: HTML }), ORIGIN, "/"), { url: `${ORIGIN}/`, text: "<p>x</p>" });
  } finally {
    mock.timers.reset();
  }
});

test("SR-25: through the route, a timeout is our own 504 page", async () => {
  mock.timers.enable({ apis: ["setTimeout"] });
  try {
    let asked = false;
    const env = makeEnv() as never;
    const never = (): Promise<Response> => ((asked = true), new Promise<Response>(() => {}));
    const pending = withPreview(await request(`${BASE}/api/preview?lang=fr`, ["designer-ui:read"]), env, (r) => handleFetch(r, env, { fetch: never }));
    while (!asked) await new Promise((r) => setImmediate(r));
    mock.timers.tick(FETCH_TIMEOUT);
    const res = await pending;
    assert.equal(res.status, 504);
    const text = await res.text();
    assert.match(text, /<html lang="fr">[\s\S]*10 secondes[\s\S]*<code>upstream_timeout<\/code>/);
  } finally {
    mock.timers.reset();
  }
  for (const lang of LANGS) {
    assert.doesNotMatch(errorPage(lang, "upstream_timeout"), /The site did not answer with the page/, lang === "en" ? "its own text" : lang);
    assert.match(errorPage(lang, "page_too_large"), /DESIGNER_PLAN/);
  }
});
