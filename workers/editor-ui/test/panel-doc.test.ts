// GO-173: the article document the editor holds and stores as a draft
// (public/js/models/article-doc.js), its frontmatter form (fm-form.js), the
// decision taken when a draft meets a newer version, the list's filters
// (article-list.js), waiting images (images.js) and the access model.
import { test } from "node:test";
import assert from "node:assert/strict";
import * as doc from "../public/js/models/article-doc.js";
import * as form from "../public/js/models/fm-form.js";
import * as list from "../public/js/models/article-list.js";
import * as images from "../public/js/models/images.js";
import { NOBODY, PERM, account, capabilities, loginTarget } from "../public/js/models/access.js";

const t = (key: string, vars: Record<string, unknown> = {}) => `${key}${Object.keys(vars).length ? JSON.stringify(vars) : ""}`;
const ARTICLE = {
  path: "content/blog/posts/hello.md", version: "v1", commit: "c1", format: "yaml", bom: false,
  frontmatterRaw: "title: Hello\ndate: 2026-11-01\ntags: [a, b]\nstatus: publish\nweight: 3\nmenu:\n  main: 1\n",
  fields: { title: "Hello", date: "2026-11-01", tags: ["a", "b"], status: "publish", weight: 3 }, fixed: ["menu"], body: "Body.\n",
};

test("form: inputs from fields, values back, only changed keys, status as ssg reads it", () => {
  const inputs = form.formOf(ARTICLE.fields);
  assert.deepEqual(inputs, { title: "Hello", description: "", date: "2026-11-01", status: true, slug: "", tags: "a, b", categories: "" });
  assert.deepEqual(form.fieldChanges(ARTICLE.fields, inputs), {});
  assert.deepEqual(form.fieldChanges(ARTICLE.fields, { ...inputs, title: "Hi", tags: "a, , c ", status: false, description: "" }), { title: "Hi", tags: ["a", "c"], status: "draft" });
  assert.deepEqual(form.fieldChanges({}, { title: "New", status: false, slug: "" }), { title: "New" }, "an absent key left empty is not added");
  assert.deepEqual(form.fieldChanges({ status: "review" }, { status: true }), { status: "publish" });
  assert.deepEqual(form.fieldChanges({ status: "review" }, { status: false }), {}, "unticked and not publish: left as it is");
  assert.equal(form.valueOf("status", false, "review"), "review");
  assert.equal(form.valueOf("status", false, undefined), "draft");
  assert.equal(form.valueOf("text", "3", 3), 3, "a number the input still spells stays a number");
  assert.equal(form.valueOf("text", "3.0", 3), "3.0");
  assert.equal(form.inputOf("text", null), "");
  assert.deepEqual(form.fieldLines(ARTICLE.fields, { ...inputs, title: "Hi", status: false, categories: "x" }, t), [
    { label: "form.title", from: "Hello", to: "Hi" },
    { label: "form.status", from: "publish", to: "draft" },
    { label: "form.categories", from: "form.unset", to: "x" },
  ]);
  assert.equal(form.shown("description", "", t), "form.empty");
  assert.equal(form.shown("other", 5, t), "5");
});

test("form problems: a title, a date ssg reads, a slug of a-z 0-9 and hyphens", () => {
  assert.deepEqual(form.problems({ title: " ", date: "14.11.2026", slug: "Bad Slug" }), { title: "form.titleMissing", date: "form.dateInvalid", slug: "form.slugInvalid" });
  assert.deepEqual(form.problems({ title: "", date: "", slug: "" }, false), {});
  for (const d of ["2026-11-14", "2026-11-14T10:00", "2026-11-14 10:00:05", "2026-11-14T10:00:00Z", "2026-11-14T10:00:00.5+01:00"]) assert.deepEqual(form.problems({ title: "x", date: d }), {}, d);
  assert.deepEqual(form.problems({}), { title: "form.titleMissing" });
});

test("document: the change publishing sends, for form edits, typed frontmatter, files without one, and new articles", () => {
  const d = doc.docOf(ARTICLE);
  assert.equal(doc.changeOf(d), null);
  assert.equal(doc.isDirty(d), false);
  assert.deepEqual(doc.changeOf({ ...d, body: "New.\n" }), { op: "put", path: ARTICLE.path, version: "v1", body: "New.\n" });
  assert.deepEqual(doc.changeOf({ ...d, body: "" }), { op: "put", path: ARTICLE.path, version: "v1", body: "" }, "emptying the text is a change");
  assert.deepEqual(doc.changeOf({ ...d, inputs: { ...d.inputs, title: "Hi" } }), { op: "put", path: ARTICLE.path, version: "v1", fields: { title: "Hi" } });
  const raw = doc.withRaw(d, "title: Hello\n");
  assert.equal(raw.rawEdited, true);
  assert.deepEqual(doc.formChanges({ ...raw, inputs: { ...raw.inputs, title: "ignored" } }), {}, "typed frontmatter locks the form");
  assert.deepEqual(doc.changeOf(raw), { op: "put", path: ARTICLE.path, version: "v1", frontmatterRaw: "title: Hello\n", body: "Body.\n" });
  assert.equal(doc.withRaw(raw, ARTICLE.frontmatterRaw).rawEdited, false);
  const none = doc.docOf({ ...ARTICLE, format: "none", frontmatterRaw: "", fields: {}, fixed: [], body: "Just text.\n" });
  assert.equal(doc.formUsable(none), false);
  assert.equal(doc.rawUsable(none), true);
  assert.deepEqual(doc.changeOf(doc.withRaw(none, "title: Added")), { op: "put", path: ARTICLE.path, version: "v1", text: "---\ntitle: Added\n---\nJust text.\n" });
  assert.deepEqual(doc.changeOf(doc.withRaw(none, "  ")), { op: "put", path: ARTICLE.path, version: "v1", text: "Just text.\n" });
  assert.deepEqual(doc.changeOf(doc.withRaw(none, "a: 1\n")).text, "---\na: 1\n---\nJust text.\n");
  const unclosed = doc.docOf({ ...ARTICLE, format: "unclosed" });
  assert.deepEqual([doc.formUsable(unclosed), doc.rawUsable(unclosed)], [false, false]);
  const fresh = doc.newDoc({ key: "new:1", path: "content/blog/posts/new.md", section: "content/blog/posts", title: "New", date: "2026-11-15" });
  assert.deepEqual([fresh.isNew, fresh.version, fresh.origin, doc.rawUsable(fresh), doc.formUsable(fresh)], [true, null, "git", false, true]);
  assert.deepEqual(doc.changeOf(fresh), { op: "put", path: "content/blog/posts/new.md", version: null, fields: { title: "New", date: "2026-11-15", status: "draft" }, body: "" });
  assert.equal(doc.changeOf({ ...fresh, inputs: { ...fresh.inputs, status: true } }).fields.status, "publish");
  assert.equal(doc.newDoc({ key: "k", path: "mddb:blog/en/x", section: "mddb:blog", origin: "mddb", title: "M", date: "2026-11-15" }).origin, "mddb");
});

test("document: title, problems, message", () => {
  const d = doc.docOf(ARTICLE);
  assert.equal(doc.titleOf(d), "Hello");
  assert.equal(doc.titleOf({ ...d, inputs: { ...d.inputs, title: " " } }), "hello");
  assert.deepEqual(doc.formProblems({ ...d, inputs: { ...d.inputs, title: "" } }), { title: "form.titleMissing" });
  const untitled = doc.docOf({ ...ARTICLE, fields: {} });
  assert.deepEqual(doc.formProblems(untitled), {}, "a file without a title need not get one");
  assert.deepEqual(doc.formProblems(doc.withRaw(d, "x: 1\n")), {});
  assert.equal(doc.messageOf(d, t), 'publish.messageUpdate{"title":"Hello"}');
  assert.equal(doc.messageOf({ ...d, isNew: true }, t), 'publish.messageNew{"title":"Hello"}');
});

test("opening with a draft: carry on, nothing to keep, or choose — and 'mine' keeps every edit on the new version", () => {
  const d = doc.docOf(ARTICLE);
  assert.equal(doc.openChoice(null, ARTICLE), "repository");
  assert.equal(doc.openChoice(d, ARTICLE), "repository", "a draft that says what the article says");
  const edited = { ...d, body: "Mine.\n", inputs: { ...d.inputs, title: "My title" } };
  assert.equal(doc.openChoice(edited, ARTICLE), "draft");
  const theirs = { ...ARTICLE, version: "v2", body: "Theirs.\n", fields: { ...ARTICLE.fields, date: "2026-11-02" } };
  assert.equal(doc.openChoice(edited, theirs), "choose");
  assert.equal(doc.openChoice({ ...edited, body: "Theirs.\n", inputs: { ...d.inputs, title: "Hello", date: "2026-11-02" } }, theirs), "repository", "the same edit published elsewhere");
  const mine = doc.rebase(edited, theirs);
  assert.deepEqual([mine.version, mine.body, mine.inputs.title, mine.inputs.date, mine.originalBody], ["v2", "Mine.\n", "My title", "2026-11-02", "Theirs.\n"]);
  assert.deepEqual(doc.changeOf(mine), { op: "put", path: ARTICLE.path, version: "v2", fields: { title: "My title" }, body: "Mine.\n" });
  const typed = doc.withRaw(d, "title: typed\n");
  assert.equal(doc.sameAs(typed, { ...ARTICLE, frontmatterRaw: "title: typed\n" }), true);
  assert.equal(doc.sameAs(typed, ARTICLE), false);
  assert.equal(doc.rebase(typed, theirs).raw, "title: typed\n");
  assert.equal(doc.rebase(typed, theirs).rawEdited, true);
});

test("the list: newest first, filters on the page, sections and languages, drafts of new articles", () => {
  const items = [
    { path: "a/old.md", title: "Old", date: "2025-01-01", language: "en", draft: false, section: "a" },
    { path: "a/new.md", title: "", date: "2026-01-01", language: "pl", draft: true, section: "a/deep" },
    { path: "b/undated.md", title: null, date: null, language: null, draft: false, section: "b" },
    { path: "b/also.md", title: "Also", date: null, language: "en", draft: false, section: "b" },
  ];
  assert.deepEqual(list.filterItems(items).map((i) => i.path), ["a/new.md", "a/old.md", "b/also.md", "b/undated.md"]);
  assert.deepEqual(list.filterItems(items, { language: "en" }).map((i) => i.path), ["a/old.md", "b/also.md"]);
  assert.deepEqual(list.filterItems(items, { state: "drafts" }).map((i) => i.path), ["a/new.md"]);
  assert.deepEqual(list.filterItems(items, { state: "published" }).length, 3);
  assert.deepEqual(list.filterItems(items, { state: "local" }, new Set(["b/also.md"])).map((i) => i.path), ["b/also.md"]);
  assert.equal(list.byNewest({ path: "x", title: "A", date: null }, { path: "y", title: "A", date: null }) < 0, true, "ties broken by path");
  assert.deepEqual([list.titleOfItem(items[1]), list.titleOfItem(items[2]), list.fileName("mddb:blog/en/key")], ["new", "undated", "key"]);
  assert.deepEqual(list.languagesOf(items, ["en"]), ["en", "pl"]);
  assert.deepEqual(list.languagesOf(items), ["en", "pl"]);
  assert.deepEqual(list.sectionsOf(items, [{ path: "a" }, { path: "c" }]), ["a", "c", "a/deep", "b"]);
  assert.deepEqual(list.sectionsOf([{ path: "x" }]), []);
  assert.deepEqual([list.sectionLabel("mddb:blog"), list.sectionLabel("content/blog/posts")], ["blog (MDDB)", "content/blog/posts"]);
  const drafts = [{ key: "a", isNew: false, savedAt: "3" }, { key: "b", isNew: true, savedAt: "1" }, { key: "c", isNew: true, savedAt: "2" }];
  assert.deepEqual(list.newDrafts(drafts).map((d) => d.key), ["c", "b"]);
  assert.ok(list.STATES.includes("local"));
});

test("waiting images: which a publication carries, what fits, how small to make one, base64, sizes", () => {
  const imgs = [{ url: "/media/a.png" }, { url: "/media/b.png" }, { url: "/media/c.png" }];
  assert.deepEqual(images.usedImages("![x](/media/a.png) ![y](/media/c.png \"t\")", imgs), [imgs[0], imgs[2]]);
  const limits = { maxFilesPerChange: 4, maxFileBytes: 100, maxChangeBytes: 150 };
  assert.equal(images.fits(101, [], limits), "file");
  assert.equal(images.fits(10, [1, 1, 1], limits), "count");
  assert.equal(images.fits(60, [100], limits), "total");
  assert.equal(images.fits(50, [100], limits), null);
  assert.equal(images.scaleFor(100, 100), null);
  assert.ok(Math.abs(images.scaleFor(400, 100)! - Math.sqrt(0.85 / 4)) < 1e-9);
  assert.equal(images.scaleFor(1e12, 1), 0.05, "never below 5 %");
  assert.deepEqual(images.sizeAfter(1000, 500, 0.5), { width: 500, height: 250 });
  assert.deepEqual(images.sizeAfter(3, 1, 0.1), { width: 1, height: 1 });
  const big = new Uint8Array(70000).map((_, i) => i % 256);
  assert.equal(images.toBase64(big), Buffer.from(big).toString("base64"));
  assert.deepEqual(images.imageChange({ path: "m/a.png" }, new Uint8Array([1, 2, 3])), { op: "put", path: "m/a.png", version: null, base64: "AQID" });
  assert.deepEqual([images.formatBytes(512, "en"), images.formatBytes(84000, "en"), images.formatBytes(2400000, "en")], ["512 byte", "84 kB", "2.4 MB"]);
});

test("access: capabilities from api/me, images need publish too, the login target stays on this site", () => {
  assert.deepEqual(capabilities(NOBODY), { read: false, publish: false, media: false, delete: false });
  assert.deepEqual(capabilities({ permissions: [PERM.read, PERM.media, PERM.delete] }), { read: true, publish: false, media: false, delete: false });
  assert.deepEqual(capabilities({ permissions: Object.values(PERM) }), { read: true, publish: true, media: true, delete: true });
  assert.deepEqual(capabilities(null), { read: false, publish: false, media: false, delete: false });
  assert.deepEqual(account({ user: { id: "u1", email: "" }, logoutUrl: "/api/auth/logout" }), { email: "u1", logoutUrl: "/api/auth/logout" });
  assert.equal(account(NOBODY), null);
  const o = "https://example.com";
  assert.equal(loginTarget({ loginUrl: "/auth/login" }, "/editor/#/edit/x", o), "/auth/login?next=%2Feditor%2F%23%2Fedit%2Fx");
  assert.equal(loginTarget({ loginUrl: "https://evil.example.net/login" }, "/", o), null);
  assert.equal(loginTarget({ loginUrl: "http://[" }, "/", o), null);
  assert.equal(loginTarget({}, "/", o), null);
});
