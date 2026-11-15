// The media library routes (GO-171): the list with the site's addresses, the
// canonical path for a new image, the articles linking an image — and the
// honest answer when that search had to stop.
import { test } from "node:test";
import assert from "node:assert/strict";
import { makeEnv, site } from "./fixtures.ts";
import { MEDIA, bytesOf, get, jpeg, png, seedBytes } from "./media-fixtures.ts";
import { LIMITS } from "../src/media-change.ts";
import { monthOf } from "../src/media-name.ts";
import { MAX_LINK_CACHE, cachedLinks, clearLinkCache, linksOf, rememberLinks } from "../src/media-references.ts";
import { clearListingCache } from "../src/content-list.ts";

test("the library: images of the media folder, newest path first, with the site's address, a page at a time", async () => {
  const gh = await site();
  await seedBytes(gh, {
    [`${MEDIA}/2026/10/a-11111111.png`]: png("a"), [`${MEDIA}/2026/11/b-22222222.jpg`]: jpeg("b"), [`${MEDIA}/old.svg`]: bytesOf("<svg/>"),
    [`${MEDIA}/notes.md`]: bytesOf("# not an image"), [`${MEDIA}/readme.txt`]: bytesOf("x"),
  });
  const res = await get(gh, "/api/media");
  assert.equal(res.status, 200);
  const page = await res.json();
  assert.deepEqual([page.dir, page.urlBase, page.limits, page.truncated, page.nextCursor], [MEDIA, "/media/", LIMITS, false, null]);
  assert.deepEqual(page.items.map((i: Record<string, unknown>) => [i.path, i.url, i.type, i.size]), [
    [`${MEDIA}/old.svg`, "/media/old.svg", "image/svg+xml", 6],
    [`${MEDIA}/2026/11/b-22222222.jpg`, "/media/2026/11/b-22222222.jpg", "image/jpeg", 5],
    [`${MEDIA}/2026/10/a-11111111.png`, "/media/2026/10/a-11111111.png", "image/png", 9],
  ]);
  assert.equal(page.items[0].version, gh.store.blobIdAt("main", `${MEDIA}/old.svg`));
  const one = await (await get(gh, "/api/media?limit=2")).json();
  assert.equal(one.items.length, 2);
  const rest = await (await get(gh, `/api/media?limit=x&cursor=${one.nextCursor}`)).json();
  assert.deepEqual([rest.items.map((i: { path: string }) => i.path), rest.nextCursor], [[`${MEDIA}/2026/10/a-11111111.png`], null]);
  assert.equal((await get(gh, "/api/media?cursor=%25")).status, 400);
  const none = await (await get(await site(), "/api/media")).json();
  assert.deepEqual(none.items, [], "no media folder yet: an empty library");
  const off = await get(gh, "/api/media", makeEnv({ EDITOR_MEDIA_DIR: "uploads" }));
  assert.deepEqual([off.status, (await off.json()).error], [503, "media_not_configured"]);
});

test("the canonical path of a new image: this month, the name's slug, the hash, the type's extension", async () => {
  const gh = await site();
  const hash = "1A2B3C4D5E6F";
  const out = await (await get(gh, `/api/media/new-path?name=My%20Cat.PNG&type=image%2Fjpeg&hash=${hash}`)).json();
  const month = monthOf(Date.now());
  assert.deepEqual(out, { path: `${MEDIA}/${month}/my-cat-1a2b3c4d.jpg`, url: `/media/${month}/my-cat-1a2b3c4d.jpg`, name: "my-cat-1a2b3c4d.jpg" });
  const noName = await (await get(gh, "/api/media/new-path?type=webp&hash=1a2b3c4d")).json();
  assert.equal(noName.name, "image-1a2b3c4d.webp");
  for (const [q, error] of [["type=svg&hash=1a2b3c4d", "unsupported_type"], ["hash=1a2b3c4d", "unsupported_type"], ["type=png&hash=xyz", "bad_hash"], ["type=png", "bad_hash"]]) {
    const r = await get(gh, `/api/media/new-path?${q}`);
    assert.deepEqual([r.status, (await r.json()).error], [400, error], q);
  }
});

test("links of an article: anything ending in an image extension, query and fragment cut off", () => {
  const md = "![a](/media/2026/11/a-1.png) <img src=\"../media/b.JPG?w=1\"> [c](https://example.com/media/c.webp#x) d.gif\n![e](e.png)";
  assert.deepEqual(linksOf(bytesOf(md)), ["/media/2026/11/a-1.png", "../media/b.JPG", "https://example.com/media/c.webp", "d.gif", "e.png"]);
  assert.deepEqual(linksOf(bytesOf([0xff, 0xfe])), [], "not UTF-8: no links");
});

test("references: the articles linking an image, however the link is written", async () => {
  clearLinkCache();
  const img = `${MEDIA}/2026/11/cat-1a2b3c4d.png`;
  const gh = await site({
    "content/blog/posts/a.md": "![](/media/2026/11/cat-1a2b3c4d.png)",
    "content/blog/posts/b.md": "<img src=\"../media/2026/11/cat-1a2b3c4d.png?x\">",
    "docs/c.md": "[c](https://example.com/media/2026/11/cat-1a2b3c4d.png)",
    "docs/d.md": "![](/media/2026/11/dog-1a2b3c4d.png) and 2026/11/cat-1a2b3c4d.png.bak",
  });
  const res = await get(gh, "/api/media/references?path=" + encodeURIComponent(img));
  const out = await res.json();
  assert.equal(res.status, 200);
  assert.deepEqual(out, {
    path: img, url: "/media/2026/11/cat-1a2b3c4d.png", items: [{ path: "content/blog/posts/a.md" }, { path: "content/blog/posts/b.md" }, { path: "docs/c.md" }],
    scanned: 7, total: 7, complete: true, nextCursor: null,
  });
  const bad = await get(gh, "/api/media/references?path=" + encodeURIComponent("docs/c.md"));
  assert.deepEqual([bad.status, (await bad.json()).reason], [400, "outside_roots"]);
});

test("references within the subrequest budget: cut short honestly, continued by the cursor, cached the next time", async () => {
  clearLinkCache();
  const files: Record<string, string> = {};
  for (let i = 0; i < 40; i++) files[`docs/p${String(i).padStart(2, "0")}.md`] = i % 10 === 0 ? `![](/media/x-${i}.png) /media/a.png` : `text ${i}`;
  const gh = await site(files);
  const q = "/api/media/references?path=" + encodeURIComponent(`${MEDIA}/a.png`);
  const found: string[] = [];
  let cursor: string | null = null;
  let pages = 0;
  do {
    const page = await (await get(gh, q + (cursor ? `&cursor=${cursor}` : ""))).json();
    pages++;
    found.push(...page.items.map((i: { path: string }) => i.path));
    assert.equal(page.complete, page.nextCursor === null);
    if (pages === 1) assert.ok(page.scanned < page.total && page.nextCursor, "the first page is cut short");
    cursor = page.nextCursor;
  } while (cursor && pages < 10);
  assert.deepEqual(found, ["docs/p00.md", "docs/p10.md", "docs/p20.md", "docs/p30.md"]);
  assert.ok(pages > 1);
  const cached = await (await get(gh, q)).json();
  assert.deepEqual([cached.complete, cached.items.length], [true, 4], "every article cached: one page");
});

test("a listing the host cut short: the library and the search say so; the link cache keeps its bound", async () => {
  clearLinkCache();
  clearListingCache();
  const gh = await site();
  await seedBytes(gh, { [`${MEDIA}/z.png`]: png("z") });
  gh.override = (method, path) => {
    if (method !== "GET" || !path.startsWith("/git/trees/")) return undefined;
    return Response.json({ sha: "x", tree: [{ path: "2026/11/a-11111111.png", mode: "100644", type: "blob", sha: "a".repeat(40) }], truncated: true });
  };
  const lib = await (await get(gh, "/api/media")).json();
  assert.deepEqual([lib.truncated, lib.items[0].size], [true, null]);
  const refs = await (await get(gh, "/api/media/references?path=" + encodeURIComponent(`${MEDIA}/a.png`))).json();
  assert.deepEqual([refs.complete, refs.nextCursor], [false, null], "every listed article read, but the listing itself was cut");
  gh.override = null;
  clearListingCache();
  for (let i = 0; i <= MAX_LINK_CACHE; i++) rememberLinks(`b${i}`, []);
  assert.deepEqual([cachedLinks("b0"), cachedLinks(`b${MAX_LINK_CACHE}`)], [undefined, []]);
  clearLinkCache();
});
