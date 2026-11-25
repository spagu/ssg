// The campaign image store's type sniffer and header parsers (#446), and its
// configuration (src/asset-config.ts).
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  ASSET_KEY, ASSET_MAX_BYTES_CEILING, DEFAULT_ASSET_MAX_BYTES, assetMaxBytes, assetUrl, assetsBaseUrl, assetsConfigProblem, assetsEnabled,
} from "../src/asset-config.ts";
import { sniffImage } from "../src/asset-image.ts";
import { sanitize } from "../src/sanitize.ts";
import { gif, jpeg, png, svg, webpExtended, webpLossless, webpLossy } from "./asset-fixtures.ts";

test("sniffImage: the three types by their bytes, with size and metadata", () => {
  assert.deepEqual(sniffImage(jpeg(640, 480)), { type: "image/jpeg", ext: "jpg", width: 640, height: 480, metadata: false });
  assert.deepEqual(sniffImage(jpeg(1200, 900, { exif: true, sof: 0xc2 })), { type: "image/jpeg", ext: "jpg", width: 1200, height: 900, metadata: true });
  assert.deepEqual(sniffImage(png(32, 16)), { type: "image/png", ext: "png", width: 32, height: 16, metadata: false });
  assert.deepEqual(sniffImage(png(32, 16, { exif: true })), { type: "image/png", ext: "png", width: 32, height: 16, metadata: true });
  assert.deepEqual(sniffImage(webpLossy(300, 200)), { type: "image/webp", ext: "webp", width: 300, height: 200, metadata: false });
  assert.deepEqual(sniffImage(webpLossless(16383, 1)), { type: "image/webp", ext: "webp", width: 16383, height: 1, metadata: false });
  assert.deepEqual(sniffImage(webpExtended(5000, 4000)), { type: "image/webp", ext: "webp", width: 5000, height: 4000, metadata: false });
  assert.equal(sniffImage(webpExtended(10, 10, 0x08))?.metadata, true, "EXIF flag");
  assert.equal(sniffImage(webpExtended(10, 10, 0x04))?.metadata, true, "XMP flag");
});

test("sniffImage: anything else is refused, whatever it claims to be", () => {
  for (const b of [svg(), gif(), new Uint8Array(0), new Uint8Array([0xff, 0xd8]), new TextEncoder().encode("RIFF....WAVEfmt "), png(1, 1).subarray(0, 7)]) {
    assert.equal(sniffImage(b), null);
  }
});

test("sniffImage: headers that do not say give null dimensions, never a throw", () => {
  const cut = (b: Uint8Array, n: number) => b.subarray(0, n);
  // JPEG: cut before SOF, a non-marker byte, a zero length, an end-of-image marker, a SOF too short to read
  assert.deepEqual(sniffImage(cut(jpeg(5, 5), 20))?.width, null);
  assert.equal(sniffImage(new Uint8Array([0xff, 0xd8, 0xff, 0xe0, 0x00, 0x00, 0, 0]))?.width, null);
  assert.equal(sniffImage(new Uint8Array([0xff, 0xd8, 0xff, 0xe0, 0x00, 0x04, 0, 0, 0x00, 0x00, 0, 0]))?.width, null);
  assert.equal(sniffImage(new Uint8Array([0xff, 0xd8, 0xff, 0xd9, 0, 0]))?.width, null);
  assert.equal(sniffImage(new Uint8Array([0xff, 0xd8, 0xff, 0xd8, 0xff, 0xc0, 0, 17, 8, 0]))?.width, null);
  assert.equal(sniffImage(jpeg(0, 0))?.width, null, "a zero size says nothing");
  // PNG: no IHDR where it belongs, a chunk length running past the end
  const noIhdr = png(4, 4);
  noIhdr.set([0x58, 0x58, 0x58, 0x58], 12);
  assert.deepEqual([sniffImage(noIhdr)?.width, sniffImage(noIhdr)?.height], [null, null]);
  assert.equal(sniffImage(cut(png(4, 4), 20))?.width, null);
  const huge = png(4, 4);
  huge.set([0x7f, 0xff, 0xff, 0xff], 8);
  assert.equal(sniffImage(huge)?.metadata, false);
  // WebP: an unknown first chunk, a VP8 without its start code, a truncated VP8L
  const odd = webpLossy(3, 3);
  odd.set([0x41, 0x4c, 0x50, 0x48], 12); // "ALPH"
  assert.deepEqual(sniffImage(odd), { type: "image/webp", ext: "webp", width: null, height: null, metadata: false });
  const noStart = webpLossy(3, 3);
  noStart[23] = 0;
  assert.equal(sniffImage(noStart)?.width, null);
  assert.equal(sniffImage(cut(webpLossless(3, 3), 22))?.width, null);
  assert.equal(sniffImage(cut(webpLossy(3, 3), 14))?.width, null);
  assert.equal(sniffImage(cut(webpExtended(3, 3), 26))?.width, null);
  const badSig = webpLossless(3, 3);
  badSig[20] = 0;
  assert.equal(sniffImage(badSig)?.width, null);
});

const KEY = `${"ab".repeat(32)}.png`;

test("asset config: the binding is the switch; size default and ceiling", () => {
  assert.equal(assetsEnabled({} as never), false);
  assert.equal(assetsEnabled({ NEWSLETTER_ASSETS: {} } as never), true);
  assert.equal(assetMaxBytes({} as never), DEFAULT_ASSET_MAX_BYTES);
  assert.equal(assetMaxBytes({ NEWSLETTER_ASSETS_MAX_BYTES: "1000" } as never), 1000);
  assert.equal(assetMaxBytes({ NEWSLETTER_ASSETS_MAX_BYTES: "999999999" } as never), ASSET_MAX_BYTES_CEILING);
  assert.equal(assetMaxBytes({ NEWSLETTER_ASSETS_MAX_BYTES: "-5" } as never), DEFAULT_ASSET_MAX_BYTES);
  assert.ok(ASSET_KEY.test(KEY));
  for (const bad of [`${"ab".repeat(32)}.svg`, `${"AB".repeat(32)}.png`, `../${KEY}`, `${KEY}/x`, "x.png"]) assert.equal(ASSET_KEY.test(bad), false, bad);
});

test("asset config: the base is NEWSLETTER_ASSETS_URL, else this worker on the site", () => {
  const on = { NEWSLETTER_ASSETS: {}, NEWSLETTER_SITE_URL: "https://example.com" };
  assert.equal(assetsBaseUrl(on as never), "https://example.com/newsletter-admin/hooks/a");
  assert.equal(assetUrl({ ...on, NEWSLETTER_UI_BASE: "/news" } as never, KEY), `https://example.com/news/hooks/a/${KEY}`);
  assert.equal(assetsConfigProblem(on as never), null);
  for (const [raw, want] of [["https://img.example.com/", "https://img.example.com"], [" https://img.example.com/news// ", "https://img.example.com/news"]]) {
    assert.equal(assetsBaseUrl({ ...on, NEWSLETTER_ASSETS_URL: raw } as never), want);
  }
  assert.equal(assetUrl({ NEWSLETTER_ASSETS_URL: "https://img.example.com" } as never, KEY), `https://img.example.com/${KEY}`);
});

test("asset config: a base that cannot work is a problem, never a fallback", () => {
  const on = { NEWSLETTER_ASSETS: {}, NEWSLETTER_SITE_URL: "https://example.com" };
  for (const bad of ["http://img.example.com", "img.example.com", "https://u:p@img.example.com", "https://img.example.com/?a=1", "https://img.example.com/#x", "https://[::1"]) {
    const env = { ...on, NEWSLETTER_ASSETS_URL: bad };
    assert.match(assetsConfigProblem(env as never) ?? "", /NEWSLETTER_ASSETS_URL must be an https/, bad);
    assert.equal(assetsBaseUrl(env as never), "", bad);
  }
  // A local preview on http: the sanitizer would drop such an image, so no address is made.
  assert.match(assetsConfigProblem({ NEWSLETTER_ASSETS: {}, NEWSLETTER_SITE_URL: "http://localhost:8787" } as never) ?? "", /NEWSLETTER_SITE_URL/);
  assert.match(assetsConfigProblem({ NEWSLETTER_ASSETS: {} } as never) ?? "", /NEWSLETTER_SITE_URL/);
  assert.match(assetsConfigProblem({ ...on, NEWSLETTER_UI_BASE: "/a b" } as never) ?? "", /NEWSLETTER_UI_BASE/);
  assert.equal(assetsConfigProblem({ NEWSLETTER_ASSETS_URL: "http://x" } as never), null, "nothing to report without the binding");
});

test("an asset URL passes the sanitizer unchanged; the https-only rule is not widened", () => {
  const url = assetUrl({ NEWSLETTER_ASSETS: {}, NEWSLETTER_SITE_URL: "https://example.com" } as never, KEY);
  assert.equal(sanitize(`<img src="${url}" alt="A">`), `<img src="${url}" alt="A">`);
  assert.equal(sanitize(`<img src="http://localhost:8787/newsletter-admin/hooks/a/${KEY}" alt="A">`), "");
});
