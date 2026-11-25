// #446, the panel's side: what it decides about campaign images without a
// DOM — the browser-side file check, the alt text and the snippet it inserts,
// the error codes in words, the upload notes, and the pre-send check's
// problems and probe results in words. Every key these build exists in en
// (panel-i18n.test.ts then holds the other four languages to en's keys).
import { test } from "node:test";
import assert from "node:assert/strict";
import { createI18n } from "../public/js/core/i18n.js";
import en from "../public/js/strings/en.js";
import {
  ACCEPTED_TYPES, ASSET_CODES, assetErrorKey, assetsState, byteSize, checkFile, cleanAlt, dimensions, htmlAttr, imageSnippet,
  insertAt, markdownAlt, thumbSize, uploadNotes,
} from "../public/js/models/assets.js";
import { PROBE_LIMIT, PROBE_SECONDS, PROBLEMS, checkLanguage, checkSummary, probeNote, problemKey } from "../public/js/models/image-check.js";
import { MAX_PROBED_IMAGES } from "../src/image-check.ts";
import { PROBE_TIMEOUT_MS } from "../src/image-probe.ts";
import { inline } from "../src/markdown.ts";

const { t } = createI18n("en", { en }, {});
const has = (key: string) => assert.ok(Object.hasOwn(en, key), key);
const file = (name: string, type: string, size: number) => ({ name, type, size });

test("assetsState: off without the store, the problem when it cannot work, else ready", () => {
  assert.equal(assetsState(null), "off");
  assert.equal(assetsState({ enabled: false, problem: null }), "off");
  assert.equal(assetsState({ enabled: true, problem: "set NEWSLETTER_SITE_URL" }), "problem");
  assert.equal(assetsState({ enabled: true, problem: null }), "ready");
});

test("checkFile: empty, type (by MIME, or by name when the browser gives none), size", () => {
  const max = 2_000_000;
  assert.equal(checkFile(null, max, "en")?.key, "asset.check.empty");
  assert.equal(checkFile(file("a.png", "image/png", 0), max, "en")?.key, "asset.check.empty");
  for (const type of ACCEPTED_TYPES) assert.equal(checkFile(file("a", type, 10), max, "en"), null, type);
  assert.equal(checkFile(file("a.gif", "image/gif", 10), max, "en")?.key, "asset.check.type");
  assert.equal(checkFile(file("a.png", "image/svg+xml", 10), max, "en")?.key, "asset.check.type", "the MIME type wins over the name");
  assert.equal(checkFile(file("photo.JPEG", "", 10), max, "en"), null, "no MIME type: the name decides");
  assert.equal(checkFile(file("photo.heic", "", 10), max, "en")?.key, "asset.check.type");
  assert.equal(checkFile({ type: "", size: 10 }, max, "en")?.key, "asset.check.type", "no name either");
  const big = checkFile(file("a.jpg", "image/jpeg", 2_500_000), max, "en");
  assert.deepEqual(big, { key: "asset.check.tooLarge", vars: { size: "2.5 MB", max: "2 MB" } });
  assert.equal(t(big!.key, big!.vars), "This file is 2.5 MB; an image may be at most 2 MB. Make it smaller (a mail image rarely needs more than 1200 pixels across) and try again.");
  assert.equal(checkFile(file("a.jpg", "image/jpeg", 2_500_000), 0, "en"), null, "no limit known: the worker decides");
});

test("alt text: one clean line; Markdown escaping keeps it inside ![…]", () => {
  assert.equal(cleanAlt("  A cat\non a\u0000 mat\t "), "A cat on a mat");
  assert.equal(cleanAlt(undefined), "");
  assert.equal(markdownAlt("a] (x) [b \\ `c`"), "a\\] (x) \\[b \\\\ \\`c\\`");
  const hostile = "x](https://example.org/evil.png) ![y\n\nnext paragraph";
  const snippet = imageSnippet("markdown", hostile, "https://example.com/a/" + "f".repeat(64) + ".png");
  assert.doesNotMatch(snippet, /\n/);
  // The worker's own Markdown renders it as one image whose alt is the text typed.
  const html = inline(snippet);
  assert.equal((html.match(/<img /g) ?? []).length, 1);
  assert.match(html, /src="https:\/\/example\.com\/a\/f{64}\.png"/);
  assert.match(html, /alt="x\]\(https:\/\/example\.org\/evil\.png\) !\[y next paragraph"/);
  assert.doesNotMatch(inline(imageSnippet("markdown", "a `b`", "https://example.com/i.png")), /<code>/);
});

test("imageSnippet: Markdown or HTML, the URL kept inside its delimiters", () => {
  assert.equal(imageSnippet("markdown", "A cat", "https://example.com/i.png"), "![A cat](https://example.com/i.png)");
  assert.equal(imageSnippet("markdown", "A", "https://example.com/my img(1).png"), "![A](https://example.com/my%20img%281%29.png)");
  assert.equal(imageSnippet("html", `"Tom" & <Jerry>`, `https://example.com/i.png?a="b"`), `<img src="https://example.com/i.png?a=&quot;b&quot;" alt="&quot;Tom&quot; &amp; &lt;Jerry&gt;">`);
  assert.equal(htmlAttr("it's"), "it&#39;s");
});

test("insertAt: at the cursor, in place of a selection, on its own paragraph", () => {
  assert.deepEqual(insertAt("", 0, 0, "IMG"), { text: "IMG", cursor: 3 });
  assert.deepEqual(insertAt("Hello world", 5, 5, "IMG"), { text: "Hello\n\nIMG\n\n world", cursor: 10 });
  assert.deepEqual(insertAt("Hello\n", 6, 6, "IMG"), { text: "Hello\nIMG", cursor: 9 });
  assert.deepEqual(insertAt("A\nold\nB", 2, 5, "IMG"), { text: "A\nIMG\nB", cursor: 5 });
});

test("assetErrorKey: every code of the asset routes has words; others fall through", () => {
  for (const code of ASSET_CODES) {
    assert.equal(assetErrorKey({ code }), `asset.error.${code}`);
    has(`asset.error.${code}`);
  }
  assert.equal(assetErrorKey({ code: "invalid_limit" }), null);
  assert.equal(assetErrorKey(null), null);
  assert.equal(t("asset.error.too_large", { max: byteSize(2097152, "en") }), "The image is larger than the worker accepts (at most 2.1 MB).");
});

test("byteSize, dimensions, thumbSize", () => {
  assert.equal(byteSize(512, "en"), "512 byte");
  assert.equal(byteSize(12_345, "en"), "12.3 kB");
  assert.equal(byteSize(undefined, "en"), "0 byte");
  assert.equal(byteSize(1_500_000, "pl"), "1,5 MB");
  assert.equal(dimensions({ width: 1200, height: 800 }), "1200 × 800");
  assert.equal(dimensions({ width: null, height: null }), null);
  assert.deepEqual(thumbSize({ width: 1200, height: 800 }), { width: 120, height: 80 });
  assert.deepEqual(thumbSize({ width: 60, height: 200 }), { width: 36, height: 120 });
  assert.deepEqual(thumbSize({ width: 40, height: 30 }), { width: 40, height: 30 }, "never enlarged");
  assert.deepEqual(thumbSize({ width: 5000, height: 1 }), { width: 120, height: 1 });
  assert.deepEqual(thumbSize({ width: null, height: null }), { width: 120, height: 120 });
});

test("uploadNotes: added; resized with the stored size; resize failed; camera metadata", () => {
  const base = { url: "u", width: 1200, height: 800, bytes: 150_000, metadata: false };
  assert.deepEqual(uploadNotes(base, "en"), { notes: [{ key: "asset.uploaded", vars: {} }], level: "ok" });
  const resized = uploadNotes({ ...base, resize: { resized: true, maxWidth: 1200, original: { width: 4000, height: 2667, bytes: 3e6 }, error: null } }, "en");
  assert.equal(resized.level, "ok");
  assert.equal(t(resized.notes[1].key, resized.notes[1].vars), "It was wider than the limit, so the stored copy is 1200 × 800 pixels (150 kB); the original was 4000 pixels wide.");
  const failed = uploadNotes({ ...base, metadata: true, resize: { resized: false, maxWidth: 1200, original: { width: 4000, height: 1, bytes: 1 }, error: "transform_failed" } }, "en");
  assert.equal(failed.level, "warning");
  assert.deepEqual(failed.notes.map((n) => n.key), ["asset.uploaded", "asset.note.resizeFailed", "asset.note.metadata"]);
  assert.match(t("asset.note.metadata"), /where it was taken/);
  const narrow = uploadNotes({ ...base, resize: { resized: false, maxWidth: 1200, original: { width: 800, height: 1, bytes: 1 }, error: null } }, "en");
  assert.deepEqual(narrow.notes.map((n) => n.key), ["asset.uploaded"], "narrow enough: nothing to say");
  for (const n of [...resized.notes, ...failed.notes]) has(n.key);
});

test("image check: every problem and probe result in words, never the code", () => {
  assert.equal(PROBE_LIMIT, MAX_PROBED_IMAGES, "mirrors the worker");
  assert.equal(PROBE_SECONDS * 1000, PROBE_TIMEOUT_MS, "mirrors the worker");
  for (const p of PROBLEMS) has(problemKey(p));
  assert.equal(problemKey("something_new"), "images.problem.other");
  has("images.problem.other");
  const probe = (over: Record<string, unknown>) => ({ via: "fetch", ok: false, status: null, contentType: null, error: null, agent: "GoogleImageProxy", ...over });
  const cases: [Record<string, unknown> | null, string][] = [
    [null, "images.probe.skipped"],
    [probe({ via: "store", ok: true, status: 200 }), "images.probe.storeOk"],
    [probe({ via: "store", status: 200, error: "not_image" }), "images.probe.storeNotImage"],
    [probe({ via: "store", status: 404 }), "images.probe.storeMissing"],
    [probe({ via: "store", error: "fetch_failed" }), "images.probe.storeFailed"],
    [probe({ ok: true, status: 200 }), "images.probe.fetchOk"],
    ...["timeout", "fetch_failed", "not_https", "too_many_redirects", "not_image"].map((e) => [probe({ error: e }), `images.probe.${e}`] as [Record<string, unknown>, string]),
    [probe({ status: 403 }), "images.probe.status"],
  ];
  for (const [p, key] of cases) {
    const note = probeNote(p);
    assert.equal(note.key, key);
    has(key);
    assert.doesNotMatch(t(note.key, note.vars), /\{\w+\}/, `${key} fills every placeholder`);
  }
  assert.equal(t(probeNote(probe({ status: 403 })).key, probeNote(probe({ status: 403 })).vars), "It answered HTTP 403 to a request introducing itself as GoogleImageProxy; a reader would see an empty box.");
  assert.equal(probeNote(probe({ error: "not_image", status: 200, contentType: "text/html" })).vars.type, "text/html");
  assert.equal(probeNote(probe({ error: "not_image", agent: null })).vars.agent, "");
  // A pass from the store says what it does not prove.
  assert.match(t("images.probe.storeOk"), /does not prove/);
  assert.match(t("images.probe.fetchOk"), /does not prove/);
});

test("checkSummary: none, clean, problems; skipped images said; always only a warning", () => {
  const item = (problems: string[]) => ({ url: "https://example.com/i.png", alt: "x", stored: true, probe: null, problems });
  const keys = (r: Record<string, unknown>) => checkSummary(r).notes.map((n: { key: string }) => n.key);
  assert.deepEqual(keys({ items: [], checked: 0, skipped: 0 }), ["images.summary.none"]);
  assert.deepEqual(keys({}), ["images.summary.none"]);
  assert.deepEqual(checkSummary({ items: [item([]), item([])], skipped: 0 }), { notes: [{ key: "images.summary.clean", vars: { count: 2 } }, { key: "images.summary.warnOnly", vars: {} }], level: "ok" });
  const bad = checkSummary({ items: [item(["no_alt"]), item([]), item(["not_stored", "unreachable"])], skipped: 3 });
  assert.equal(bad.level, "warning");
  assert.deepEqual(bad.notes.map((n: { key: string }) => n.key), ["images.summary.problems", "images.summary.skipped", "images.summary.warnOnly"]);
  assert.equal(t(bad.notes[0].key, bad.notes[0].vars), "2 images of 3 need attention.");
  assert.equal(t(bad.notes[1].key, bad.notes[1].vars), "3 more images were not tested for loading: one check tests the first 8 different images.");
  for (const n of bad.notes) has(n.key);
});

test("checkLanguage: the segment's first language, else English", () => {
  assert.equal(checkLanguage({ segment: { languages: ["pl", "de"] } }), "pl");
  assert.equal(checkLanguage({ segment: { languages: [] } }), "en");
  assert.equal(checkLanguage(null), "en");
});
