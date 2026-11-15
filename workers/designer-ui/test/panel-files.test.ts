// GO-179: the Files tab's model (public/js/models/theme-files.js) and the
// generic line diff (public/js/core/diff.js).
import { test } from "node:test";
import assert from "node:assert/strict";
import { MAX_CELLS, hunks, lineDiff, stats } from "../public/js/core/diff.js";
import { base64Of, createDrafts, dataUrl, describeDraft, extOf, findAll, isImage, lineAt, lineNumbers, nextMatch, tree } from "../public/js/models/theme-files.js";

const ops = (a: string, b: string) => lineDiff(a, b).map((o: { op: string; text: string }) => `${o.op[0]} ${o.text}`);

test("lineDiff: the same, an insertion, a removal, a change", () => {
  assert.deepEqual(ops("a\nb", "a\nb"), ["s a", "s b"]);
  assert.deepEqual(ops("a\nc", "a\nb\nc"), ["s a", "a b", "s c"]);
  assert.deepEqual(ops("a\nb\nc", "a\nc"), ["s a", "d b", "s c"]);
  assert.deepEqual(ops("a\nb\nc", "a\nx\nc"), ["s a", "a x", "d b", "s c"]);
  assert.deepEqual(ops("", "a"), ["a a"]);
  assert.deepEqual(ops("a", ""), ["d a"]);
  assert.deepEqual(ops("x\ny\nz", "z\ny\nx"), ["a z", "a y", "s x", "d y", "d z"]);
  assert.deepEqual(ops("a\r\nb", "a\nb"), ["s a", "s b"], "line endings do not count");
  const d = lineDiff("1\n2\n3", "1\nX\n3");
  assert.deepEqual(d.map((o: { a: number | null; b: number | null }) => [o.a, o.b]), [[1, 1], [null, 2], [2, null], [3, 3]]);
});

test("lineDiff: a middle too large for the table is removed-then-added", () => {
  const n = Math.ceil(Math.sqrt(MAX_CELLS)) + 1;
  const a = Array.from({ length: n }, (_, i) => `a${i}`).join("\n");
  const b = Array.from({ length: n }, (_, i) => `b${i}`).join("\n");
  const d = lineDiff(a, b);
  assert.deepEqual(stats(d), { added: n, removed: n });
  assert.equal(d[0].op, "del");
  assert.equal(d[n].op, "add");
});

test("hunks: changed runs with context, split when far apart", () => {
  const a = Array.from({ length: 30 }, (_, i) => `l${i}`);
  const b = [...a];
  b[2] = "X";
  b[25] = "Y";
  const h = hunks(lineDiff(a.join("\n"), b.join("\n")), 2);
  assert.equal(h.length, 2);
  assert.deepEqual(h[0].map((o: { op: string }) => o.op), ["same", "same", "add", "del", "same", "same"]);
  b[5] = "Z";
  assert.equal(hunks(lineDiff(a.join("\n"), b.join("\n")), 2).length, 2, "close runs join");
  assert.deepEqual(hunks(lineDiff("a", "a")), []);
});

const FILES = [
  { path: "templates/t/index.html", blobId: "1", text: true },
  { path: "templates/t/css/tokens.css", blobId: "2", text: true },
  { path: "templates/t/css/style.css", blobId: "3", text: true },
  { path: "templates/t/img/logo.png", blobId: "4", text: false },
  { path: "templates/t/a/b/deep.html", blobId: "5", text: true },
  { path: "other/outside.css", blobId: "6", text: true },
];

test("tree: folders first, sorted, nested", () => {
  const t = tree(FILES, "templates/t");
  assert.deepEqual(t.dirs.map((d: { name: string }) => d.name), ["a", "css", "img", "other"]);
  assert.deepEqual(t.files.map((f: { name: string }) => f.name), ["index.html"]);
  assert.deepEqual(t.dirs[1].files.map((f: { name: string }) => f.name), ["style.css", "tokens.css"]);
  assert.equal(t.dirs[0].dirs[0].files[0].path, "templates/t/a/b/deep.html");
  assert.equal(t.dirs[0].dirs[0].path, "templates/t/a/b");
  assert.equal(tree([{ path: "x.css" }, { path: "x.css" }], "").files.length, 2, "nothing is dropped");
});

test("drafts: text and images, back to the original drops the draft", () => {
  const d = createDrafts();
  d.setText(FILES[1], "a", "b");
  d.setImage(FILES[3], "QUJD");
  assert.equal(d.size, 2);
  assert.ok(d.has(FILES[1].path));
  assert.deepEqual(d.list(), [
    { op: "put", path: FILES[1].path, content: { text: "b" }, baseBlobId: "2" },
    { op: "put", path: FILES[3].path, content: { base64: "QUJD" }, baseBlobId: "4" },
  ]);
  assert.deepEqual(describeDraft(d.get(FILES[1].path)), { path: FILES[1].path, image: false, added: 1, removed: 1 });
  assert.deepEqual(describeDraft(d.get(FILES[3].path)), { path: FILES[3].path, image: true, added: 0, removed: 0 });
  d.setText(FILES[1], "a", "a");
  assert.equal(d.size, 1);
  d.discard(FILES[3].path);
  assert.equal(d.items().length, 0);
  d.setText(FILES[0], "", "x");
  d.clear();
  assert.equal(d.size, 0);
});

test("search: every match, case-insensitive, with lines; next and previous wrap", () => {
  const text = "Body {\n  color: red;\n}\nbody p { COLOR: blue }";
  const m = findAll(text, "color");
  assert.deepEqual(m.map((x: { line: number }) => x.line), [2, 4]);
  assert.deepEqual(findAll(text, ""), []);
  assert.equal(nextMatch(m, 0), 0);
  assert.equal(nextMatch(m, m[0].start + 1), 1);
  assert.equal(nextMatch(m, text.length), 0, "wraps to the first");
  assert.equal(nextMatch(m, m[1].start, -1), 0);
  assert.equal(nextMatch(m, 0, -1), 1, "wraps to the last");
  assert.equal(nextMatch([], 0), -1);
  assert.equal(lineAt(text, 0), 1);
  assert.equal(lineAt(text, 999), 4);
  assert.equal(lineNumbers("a\nb\nc"), "1\n2\n3");
  assert.equal(lineNumbers(""), "1");
});

test("images: types, base64 and data URLs", () => {
  assert.equal(extOf("a/b/Logo.PNG"), "png");
  assert.equal(extOf("README"), "");
  assert.ok(isImage("x.svg") && isImage("x.webp") && !isImage("x.css"));
  assert.equal(base64Of(new TextEncoder().encode("ABC").buffer), "QUJD");
  assert.equal(base64Of(new Uint8Array(70000).buffer).length, Math.ceil(70000 / 3) * 4, "large files in chunks");
  assert.equal(dataUrl("a.svg", "QUJD"), "data:image/svg+xml;base64,QUJD");
});
