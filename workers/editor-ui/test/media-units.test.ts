// The pure parts of images: the type by the bytes, the canonical name, the
// media folder and its address, base64 and git's blob id.
import { test } from "node:test";
import assert from "node:assert/strict";
import { GitStore } from "./fake-git-store.ts";
import { extOfType, looksLikeMarkup, mediaPathFor, mediaSlug, monthOf, parseMediaName, sha256Hex, sniffImage } from "../src/media-name.ts";
import { decodeBase64, gitBlobId } from "../src/media-change.ts";
import { MEDIA_HINT, mediaPath, mediaPlace, requireMedia, typeOf, urlOf } from "../src/media-scope.ts";
import type { SiteModel } from "../src/site-model.ts";
import { OpError } from "../src/ops.ts";

const bytes = (...parts: (number[] | string)[]) =>
  new Uint8Array(parts.flatMap((p) => (typeof p === "string" ? [...new TextEncoder().encode(p)] : p)));
export const PNG = bytes([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a], "rest of a png");
const ftyp = (size: number, ...brands: string[]) => bytes([0, 0, 0, size], "ftyp", brands[0] ?? "", [0, 0, 0, 0], brands.slice(1).join(""));

test("the type is the bytes' signature, whatever the name; SVG and HTML are not images", () => {
  assert.equal(sniffImage(PNG), "png");
  assert.equal(sniffImage(bytes([0xff, 0xd8, 0xff, 0xe0])), "jpg");
  assert.equal(sniffImage(bytes("GIF89a....")), "gif");
  assert.equal(sniffImage(bytes("GIF87a")), "gif");
  assert.equal(sniffImage(bytes("RIFF", [1, 2, 3, 4], "WEBPVP8 ")), "webp");
  assert.equal(sniffImage(bytes("RIFF", [1, 2, 3, 4], "WAVE")), null);
  assert.equal(sniffImage(ftyp(24, "avif", "mif1miaf")), "avif", "major brand");
  assert.equal(sniffImage(ftyp(24, "mif1", "miafavis")), "avif", "compatible brand");
  assert.equal(sniffImage(ftyp(24, "mif1", "miafheic")), null, "HEIC is no AVIF");
  assert.equal(sniffImage(ftyp(12, "mif1", "avif")), null, "a brand outside the box is not read");
  assert.equal(sniffImage(bytes("<svg xmlns='http://www.w3.org/2000/svg'/>")), null);
  assert.equal(sniffImage(bytes([0x89, 0x50])), null);
  assert.equal(sniffImage(new Uint8Array()), null);
  assert.equal(looksLikeMarkup(bytes("\uFEFF \n<html>")), true);
  assert.equal(looksLikeMarkup(bytes("plain")), false);
});

test("the client's type is a hint mapped to the canonical extension; SVG has none", () => {
  assert.deepEqual(["png", ".PNG", "image/jpeg", "jpeg", "image/pjpeg", "jpg", "webp", "image/gif", "avif"].map(extOfType), ["png", "png", "jpg", "jpg", "jpg", "jpg", "webp", "gif", "avif"]);
  assert.deepEqual(["svg", "image/svg+xml", "text/html", "", "toString"].map(extOfType), [null, null, null, null, null]);
});

test("names: a-z, 0-9 and hyphens from the original name, cut at a hyphen, a fallback; canonical paths parse back", async () => {
  assert.equal(mediaSlug("C:\\shots\\Screen Shot 2026-11-14 at 10.12.png"), "screen-shot-2026-11-14-at-10-12");
  assert.equal(mediaSlug("dir/Za\u017C\u00F3\u0142\u0107.JPEG"), "za");
  assert.equal(mediaSlug("\u0928\u092E\u0938\u094D\u0924\u0947.png"), "image");
  assert.equal(mediaSlug("a".repeat(30) + "-" + "b".repeat(30)), "a".repeat(30));
  assert.equal(mediaSlug("c".repeat(70)), "c".repeat(50));
  assert.equal(monthOf(Date.UTC(2026, 0, 31, 23, 59)), "2026/01");
  const hash = await sha256Hex(PNG);
  assert.match(hash, /^[0-9a-f]{64}$/);
  const p = mediaPathFor("content/blog/media", Date.UTC(2026, 10, 14), "Cat.png", hash, "png");
  assert.equal(p, `content/blog/media/2026/11/cat-${hash.slice(0, 8)}.png`);
  assert.equal(mediaPathFor("m", Date.UTC(2026, 10, 14), "Cat.png", hash, "png"), mediaPathFor("m", Date.UTC(2026, 10, 1), "other/Cat.gif", hash, "png"), "same bytes, same month: same path");
  assert.deepEqual(parseMediaName(`2026/11/cat-${hash.slice(0, 8)}.png`), { month: "2026/11", slug: "cat", hash: hash.slice(0, 8), ext: "png" });
  for (const bad of ["2026/13/cat-1a2b3c4d.png", "26/11/cat-1a2b3c4d.png", "2026/11/Cat-1a2b3c4d.png", "2026/11/cat-1a2b3c4.png", "2026/11/cat-1a2b3c4d.svg",
    "2026/11/cat-1a2b3c4d.jpeg", "2026/11/-1a2b3c4d.png", "2026/11/x/cat-1a2b3c4d.png", `2026/11/${"c".repeat(51)}-1a2b3c4d.png`]) {
    assert.equal(parseMediaName(bad), null, bad);
  }
});

test("base64 decodes natively or by the fallback, malformed base64 is refused; git's blob id is the store's", async () => {
  const b64 = Buffer.from(PNG).toString("base64");
  assert.deepEqual(decodeBase64(b64), PNG);
  const native = (Uint8Array as any).fromBase64;
  (Uint8Array as any).fromBase64 = undefined;
  try {
    assert.deepEqual(decodeBase64(b64), PNG);
  } finally {
    (Uint8Array as any).fromBase64 = native;
  }
  assert.throws(() => decodeBase64("not base64!"), { code: "invalid_change" });
  assert.equal(await gitBlobId(PNG), await new GitStore().putBlob(PNG as Uint8Array<ArrayBuffer>));
});

const model = (over: Partial<SiteModel> = {}): SiteModel => ({
  contentRoots: ["content/blog"], sections: [], mediaDir: "content/blog/media", staticDir: "static", languages: [], defaultLanguage: null,
  languageSections: {}, shortcodes: [], mddb: null, problems: [], head: "h", branch: "main", configFile: ".ssg.yaml", overlay: false,
  overrides: { contentDirs: false, mediaDir: false }, ...over,
});

test("the media folder and its address: ssg's content media, static_dir/images, overrides", () => {
  const base = (m: SiteModel) => { const p = mediaPlace(m); return p && [p.dir, p.urlBase]; };
  assert.deepEqual(base(model()), ["content/blog/media", "/media/"]);
  assert.deepEqual(base(model({ mediaDir: null })), ["static/images", "/images/"]);
  assert.equal(base(model({ mediaDir: null, staticDir: null })), null);
  const over = { overrides: { contentDirs: false, mediaDir: true } };
  assert.deepEqual(base(model({ ...over, mediaDir: "static" })), ["static", "/"]);
  assert.deepEqual(base(model({ ...over, mediaDir: "static/img/up" })), ["static/img/up", "/img/up/"]);
  assert.deepEqual(base(model({ ...over, mediaDir: "content/news/media", staticDir: null })), ["content/news/media", "/media/"]);
  assert.equal(base(model({ ...over, mediaDir: "uploads" })), null, "a folder ssg does not publish");
  assert.throws(() => requireMedia(model({ ...over, mediaDir: "uploads" })), (e: OpError) => e.status === 503 && e.code === "media_not_configured" && e.detail === "uploads is not a folder ssg publishes" && e.extra.hint === MEDIA_HINT);
  assert.throws(() => requireMedia(model({ mediaDir: null, staticDir: null })), { detail: "the site has no media folder" });
  const place = requireMedia(model());
  assert.equal(urlOf("content/blog/media/2026/11/a.png", place), "/media/2026/11/a.png");
  assert.equal(mediaPath("content/blog/media/a.svg", place, "read"), "content/blog/media/a.svg");
  for (const [raw, reason] of [["content/blog/a.png", "outside_roots"], ["content/blog/media/a.md", "extension"], ["content/blog/media/workers/a.png", "denied"], ["../a.png", "traversal"]]) {
    assert.throws(() => mediaPath(raw, place, "write"), (e: OpError) => e.code === "invalid_path" && e.extra.reason === reason && e.extra.mediaDir === place.dir, raw);
  }
  assert.throws(() => mediaPath(7, place, "write"), (e: OpError) => e.extra.path === null && e.detail === "not a valid repository path");
  assert.deepEqual(["a.PNG", "a.jpeg", "a.svg", "a.avif"].map(typeOf), ["image/png", "image/jpeg", "image/svg+xml", "image/avif"]);
});
