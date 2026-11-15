// GO-173: the panel's mirror of the image naming (public/js/models/media-name.js)
// answers exactly as the worker's src/media-name.ts — run side by side on
// the Go-generated slug samples, the worker's own cases and every code point
// of the first plane, so a change to one side without the other fails here.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as panel from "../public/js/models/media-name.js";
import * as worker from "../src/media-name.ts";
import { slugify as workerSlugify } from "../src/slug.ts";

const samples = JSON.parse(readFileSync(new URL("./slug-samples.json", import.meta.url), "utf8")) as { phrases: [string, string][]; runes: [number, string][] };
const bytes = (...parts: (number[] | string)[]) => new Uint8Array(parts.flatMap((p) => (typeof p === "string" ? [...new TextEncoder().encode(p)] : p)));
const ftyp = (size: number, ...brands: string[]) => bytes([0, 0, 0, size], "ftyp", brands[0] ?? "", [0, 0, 0, 0], brands.slice(1).join(""));

test("slugify: ssg's own samples, and the worker's answer for every code point below U+3000", () => {
  for (const [input, want] of samples.phrases) assert.equal(panel.slugify(input), want, input);
  for (const [cp, want] of samples.runes) assert.equal(panel.slugify(String.fromCodePoint(cp)), want, String(cp));
  for (let cp = 0; cp < 0x3000; cp++) {
    const s = `A${String.fromCodePoint(cp)}b`;
    assert.equal(panel.slugify(s), workerSlugify(s), `U+${cp.toString(16)}`);
  }
});

const NAMES = [
  "C:\\shots\\Screen Shot 2026-11-14 at 10.12.png", "dir/Za\u017C\u00F3\u0142\u0107.JPEG", "\u0928\u092E\u0938\u094D\u0924\u0947.png",
  "a".repeat(30) + "-" + "b".repeat(30), "c".repeat(70), "", ".png", "photo.final.webp", "A cat on a mat", "x.toolongext", "image",
  "\u0130stanbul \u212Aelvin.gif", "---", "a/b\\c.d",
];

test("mediaSlug and mediaPathFor: the worker's cases and more, same answer on both sides", async () => {
  for (const name of NAMES) assert.equal(panel.mediaSlug(name), worker.mediaSlug(name), name);
  assert.equal(panel.mediaSlug("c".repeat(70)), "c".repeat(50));
  assert.equal(panel.mediaSlug("\u0928.png"), panel.MEDIA_FALLBACK);
  const hash = await panel.sha256Hex(bytes("an image"));
  assert.equal(hash, await worker.sha256Hex(bytes("an image")));
  for (const ms of [Date.UTC(2026, 0, 31, 23, 59), Date.UTC(2026, 10, 14), Date.UTC(1999, 11, 31)]) {
    assert.equal(panel.monthOf(ms), worker.monthOf(ms));
    for (const name of NAMES) assert.equal(panel.mediaPathFor("content/blog/media", ms, name, hash, "png"), worker.mediaPathFor("content/blog/media", ms, name, hash, "png"));
  }
  assert.deepEqual(panel.IMAGE_TYPES, worker.IMAGE_TYPES);
  assert.deepEqual([panel.MAX_MEDIA_SLUG, panel.HASH_LENGTH, panel.MEDIA_FALLBACK], [worker.MAX_MEDIA_SLUG, worker.HASH_LENGTH, worker.MEDIA_FALLBACK]);
});

test("sniffImage: the signature decides, as on the worker; SVG and HTML are not images", () => {
  const cases = [
    bytes([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a], "png"), bytes([0xff, 0xd8, 0xff, 0xe0]), bytes("GIF89a...."), bytes("GIF87a"),
    bytes("RIFF", [1, 2, 3, 4], "WEBPVP8 "), bytes("RIFF", [1, 2, 3, 4], "WAVE"), ftyp(24, "avif", "mif1miaf"), ftyp(24, "mif1", "miafavis"),
    ftyp(24, "mif1", "miafheic"), ftyp(12, "mif1", "avif"), bytes("<svg xmlns='http://www.w3.org/2000/svg'/>"), bytes([0x89, 0x50]), new Uint8Array(),
    bytes("<html><body>"), bytes([0, 0, 0, 0], "ftyp"),
  ];
  for (const b of cases) assert.equal(panel.sniffImage(b), worker.sniffImage(b));
  assert.deepEqual(cases.slice(0, 7).map(panel.sniffImage), ["png", "jpg", "gif", "gif", "webp", null, "avif"]);
});

test("urlOf: the media folder's URL base and the path below it (media-scope.ts)", () => {
  assert.equal(panel.urlOf("content/blog/media/2026/11/cat-1a2b3c4d.png", { dir: "content/blog/media", urlBase: "/media/" }), "/media/2026/11/cat-1a2b3c4d.png");
  assert.equal(panel.urlOf("static/2026/11/a-1a2b3c4d.gif", { dir: "static", urlBase: "/" }), "/2026/11/a-1a2b3c4d.gif");
});
