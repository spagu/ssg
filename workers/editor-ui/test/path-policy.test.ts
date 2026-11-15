// GO-168 acceptance: the path-policy table — traversal, .github/x, a/../b, unicode, double slashes, case.
import { test } from "node:test";
import assert from "node:assert/strict";
import { checkPath, checkPrefix, normalizePath, type PathPolicy } from "../portable/path-policy";

const editor: PathPolicy = { roots: ["content", "static/images"], extensions: [".md", ".png", ".JPG"] };
const designer: PathPolicy = { roots: ["themes/site"], extensions: [".html", ".css"], writeFiles: [".ssg.designer.json"], readFiles: [".ssg.yaml"] };

// [input, expected normalised path or refusal reason]
const table: [unknown, string][] = [
  ["content/posts/hello.md", "content/posts/hello.md"],
  ["content//posts///hello.md", "content/posts/hello.md"], // double slashes collapsed
  ["content\\posts\\hello.md", "content/posts/hello.md"], // backslashes are separators
  ["content/posts/cafe\u0301.md", "content/posts/caf\u00e9.md"], // NFC
  ["content/posts/zażółć gęślą.md", "content/posts/zażółć gęślą.md"],
  ["static/images/photo.PNG", "static/images/photo.PNG"], // extension compared case-insensitively
  ["static/images/photo.jpg", "static/images/photo.jpg"], // policy extension given in upper case
  ["../etc/passwd", "traversal"],
  ["content/../.github/workflows/x.yml", "traversal"],
  ["a/../b", "traversal"],
  ["content/posts/../../x.md", "traversal"],
  ["content/./posts/x.md", "dot_segment"],
  [".github/x", "denied"],
  [".GitHub/workflows/deploy.yml", "denied"], // case-insensitive denial
  ["content/.git/config", "denied"],
  ["content/workers/x.md", "denied"],
  ["content/WRANGLER.toml", "denied"],
  ["content/.dev.vars", "denied"],
  ["content/.env.production", "denied"],
  ["content/.hidden.md", "dot_segment"],
  ["Content/posts/x.md", "outside_roots"], // roots are case-sensitive, as git is
  ["contentx/posts/x.md", "outside_roots"],
  ["content", "outside_roots"],
  ["static/x.png", "outside_roots"],
  ["/content/posts/x.md", "absolute"],
  ["//content/x.md", "absolute"],
  ["content/posts/x.md\u0000.png", "control_character"],
  ["content/posts/x\n.md", "control_character"],
  ["content/posts/\u202Edm.exe", "control_character"], // bidi override
  ["content/posts/a\u200Bb.md", "control_character"], // zero-width space
  ["C:/content/x.md", "control_character"],
  ["content/posts/x?.md", "control_character"],
  ["content/posts/", "bad_segment"],
  ["content/posts/ x.md", "bad_segment"],
  ["content/posts/x.md.", "bad_segment"],
  ["content/" + "a".repeat(400) + ".md", "too_long"],
  ["content/" + "a".repeat(300) + ".md", "bad_segment"],
  ["content/posts/README", "extension"],
  ["content/posts/run.sh", "extension"],
  ["", "empty"],
  [42, "not_a_string"],
  [null, "not_a_string"],
];

test("path-policy table (write access, editor policy)", () => {
  for (const [input, want] of table) {
    const v = checkPath(input, { ...editor, maxBytes: input === "content/" + "a".repeat(300) + ".md" ? 1000 : undefined }, "write");
    assert.equal(v.ok ? v.path : v.reason, want, `input ${JSON.stringify(input)}`);
  }
});

test("exact files: dotfiles allowed only when listed; readFiles are read-only; denials still win", () => {
  assert.deepEqual(checkPath(".ssg.yaml", designer, "read"), { ok: true, path: ".ssg.yaml" });
  assert.deepEqual(checkPath(".ssg.yaml", designer, "write"), { ok: false, reason: "read_only" });
  assert.deepEqual(checkPath(".ssg.designer.json", designer, "write"), { ok: true, path: ".ssg.designer.json" });
  assert.deepEqual(checkPath(".ssg.yaml", editor, "read"), { ok: false, reason: "dot_segment" });
  const sneaky: PathPolicy = { roots: ["workers", ".github"], extensions: [".yml", ".ts"], writeFiles: [".github/workflows/ci.yml", "../x", "wrangler.toml"] };
  assert.deepEqual(checkPath(".github/workflows/ci.yml", sneaky, "write"), { ok: false, reason: "denied" });
  assert.deepEqual(checkPath("workers/x/a.ts", sneaky, "read"), { ok: false, reason: "denied" });
  assert.deepEqual(checkPath("wrangler.toml", sneaky, "read"), { ok: false, reason: "denied" });
  assert.deepEqual(checkPath("themes/site/base.html", designer, "write"), { ok: true, path: "themes/site/base.html" });
});

test("an empty or invalid root never matches everything", () => {
  const p: PathPolicy = { roots: ["", "/", "../x"], extensions: [".md"] };
  assert.deepEqual(checkPath("x.md", p, "read"), { ok: false, reason: "outside_roots" });
  assert.deepEqual(checkPath("a/x.md", p, "read"), { ok: false, reason: "outside_roots" });
});

test("checkPrefix: a root or a directory inside one", () => {
  assert.deepEqual(checkPrefix("content", editor), { ok: true, path: "content" });
  assert.deepEqual(checkPrefix("content/posts/", editor), { ok: true, path: "content/posts" });
  assert.deepEqual(checkPrefix("static", editor), { ok: false, reason: "outside_roots" });
  assert.deepEqual(checkPrefix("", editor), { ok: false, reason: "empty" });
  assert.deepEqual(checkPrefix("content/.github", editor), { ok: false, reason: "denied" });
  assert.deepEqual(checkPrefix("content/.drafts", editor), { ok: false, reason: "dot_segment" });
  assert.deepEqual(checkPrefix("content/../x", editor), { ok: false, reason: "traversal" });
  assert.deepEqual(checkPrefix(7, editor), { ok: false, reason: "not_a_string" });
});

test("normalizePath default limit is 400 bytes of UTF-8", () => {
  const z = "ż".repeat(99); // 198 bytes, a valid segment
  assert.equal(normalizePath(`a/${z}/${z}`).ok, true); // 399 bytes
  assert.deepEqual(normalizePath(`a/${z}/${z}ż`), { ok: false, reason: "too_long" }); // 401 bytes
});
