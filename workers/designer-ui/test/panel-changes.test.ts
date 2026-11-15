// GO-179: pending settings changes and how they read in the publish dialog
// (public/js/models/settings-changes.js), what an account may change
// (access.js), and names and descriptions in the panel's language (labels.js).
import { test } from "node:test";
import assert from "node:assert/strict";
import { commitMessage, createChanges, describe, formatValue, keyOf, sameValue } from "../public/js/models/settings-changes.js";
import { NOBODY, PERM, account, capabilities, editable, loginTarget, needsAdvanced } from "../public/js/models/access.js";
import { TRANSLATED, descriptionOf, groupText, labelOf, searchTexts } from "../public/js/models/labels.js";
import { createI18n } from "../public/js/core/i18n.js";

const t = (key: string, vars: Record<string, unknown> = {}) => `${key}${Object.keys(vars).length ? JSON.stringify(vars) : ""}`;
const entry = (path: string, value: unknown, source = "file", cls = "plain") => ({ path, segments: path.split("."), value, source, class: cls, node: { type: "string", description: "the site's name" } });

test("changes: set, revert, discard, and a no-op is no change", () => {
  const c = createChanges();
  const title = entry("title", "Example");
  c.set(title, "My kitchen", "Example");
  assert.equal(c.size, 1);
  assert.ok(c.has(title));
  assert.deepEqual(c.list(), [{ path: ["title"], value: "My kitchen" }]);
  c.set(title, "Example", "Example");
  assert.equal(c.size, 0, "back to where it started");
  const toc = entry("toc", true, "overlay");
  c.revert(toc);
  assert.deepEqual(c.get(toc).value, null, "a key the overlay holds is removed from it");
  c.revert(entry("toc", true, "file"));
  assert.equal(c.size, 0, "a key not in the overlay has nothing to remove");
  c.set(title, "X", "Example");
  assert.deepEqual([...c.paths()], ["title"]);
  assert.equal(c.items()[0].entry, title);
  c.discard(title);
  assert.equal(c.size, 0);
  c.set(title, "Y", "Example");
  c.clear();
  assert.equal(c.size, 0);
  assert.equal(keyOf(["ai", "models", "gpt-4.1"]), '["ai","models","gpt-4.1"]', "a dot inside a key stays inside it");
  assert.ok(sameValue({ a: 1, b: [1, { c: 2, d: 3 }] }, { b: [1, { d: 3, c: 2 }], a: 1 }));
  assert.ok(!sameValue([1, 2], [2, 1]));
  assert.ok(sameValue(undefined, undefined));
});

test("formatValue: a value in a few words; a secret only as its reference", () => {
  assert.equal(formatValue(undefined, t), "value.unset");
  assert.equal(formatValue(null, t), "value.reverted");
  assert.equal(formatValue(true, t), "value.on");
  assert.equal(formatValue(false, t), "value.off");
  assert.equal(formatValue(20, t), "20");
  assert.equal(formatValue("", t), "value.empty");
  assert.equal(formatValue("Example", t), "“Example”");
  assert.equal(formatValue("x".repeat(60), t), `“${"x".repeat(47)}…”`);
  assert.equal(formatValue([1, 2], t), 'value.items{"count":2}');
  assert.equal(formatValue({ a: 1 }, t), 'value.keys{"count":1}');
  assert.equal(formatValue({ set: true, reference: "$JWT" }, t), "$JWT");
  assert.equal(formatValue({ set: true, reference: null }, t), "value.secretSet");
  assert.equal(formatValue({ set: false, reference: null }, t), "value.secretUnset");
});

test("describe and commitMessage", () => {
  const d = describe({ entry: entry("title", "Example"), value: "My kitchen" }, t, () => "Site name");
  assert.deepEqual(d, { path: "title", label: "Site name", from: "“Example”", to: "“My kitchen”", reverted: false, advanced: false });
  const r = describe({ entry: entry("hooks", { pre_build: [] }, "overlay", "executes"), value: null }, t, () => "Hooks");
  assert.equal(r.to, "value.reverted");
  assert.ok(r.reverted && r.advanced);
  assert.equal(commitMessage([], t), "");
  assert.equal(commitMessage(["A", "B"], t), 'publish.messageFew{"keys":"A, B"}');
  assert.equal(commitMessage(["A", "B", "C", "D", "E"], t), 'publish.messageMore{"keys":"A, B, C","count":2}');
});

test("access: capabilities, what is editable, references, login target", () => {
  const all = capabilities({ permissions: Object.values(PERM) });
  assert.deepEqual(all, { read: true, settings: true, advanced: true, theme: true });
  const reader = capabilities({ permissions: [PERM.read] });
  assert.deepEqual(capabilities(NOBODY), { read: false, settings: false, advanced: false, theme: false });
  assert.deepEqual(capabilities(null), capabilities(NOBODY));
  const plain = { class: "plain" };
  const runs = { class: "executes" };
  assert.ok(editable(plain, all) && editable(runs, all));
  assert.ok(!editable(plain, reader));
  const settings = capabilities({ permissions: [PERM.read, PERM.settings] });
  assert.ok(editable(plain, settings) && !editable(runs, settings) && !editable({ class: "secret" }, settings));
  assert.ok(needsAdvanced("Bearer $TOKEN") && needsAdvanced(["a", "${B}"]) && needsAdvanced({ x: { y: "$Z" } }));
  assert.ok(!needsAdvanced("costs $5") && !needsAdvanced(3) && !needsAdvanced(null) && !needsAdvanced({ a: "b" }));
  assert.deepEqual(account({ user: { id: "u1", email: "ada@example.com" }, logoutUrl: "/api/auth/logout" }), { email: "ada@example.com", logoutUrl: "/api/auth/logout" });
  assert.equal(account({ user: { id: "u1", email: "" } })!.email, "u1");
  assert.equal(account(NOBODY), null);
  const o = "https://example.com";
  assert.equal(loginTarget({ loginUrl: "/auth/login" }, "/designer/#/files", o), "/auth/login?next=%2Fdesigner%2F%23%2Ffiles");
  assert.equal(loginTarget({ loginUrl: "https://evil.example.net/login" }, "/", o), null, "never off this site");
  assert.equal(loginTarget({ loginUrl: "http://[bad" }, "/", o), null);
  assert.equal(loginTarget({}, "/", o), null);
  assert.equal(loginTarget(null, "/", o), null);
});

test("labels: translated where the strings have them, English words and description otherwise", () => {
  const tables = { en: { "key.title": "Site name", "group.site.title": "Site" }, pl: { "key.title": "Nazwa strony", "desc.title": "Nazwa w <title>", "group.site.title": "Strona", "group.site.desc": "Nazwa i adres" } };
  const pl = createI18n("pl", tables);
  const en = createI18n("en", tables);
  const title = entry("title", "x");
  assert.equal(labelOf(pl, title), "Nazwa strony");
  assert.deepEqual(descriptionOf(pl, title), { text: "Nazwa w <title>", lang: "pl" });
  assert.deepEqual(descriptionOf(en, title), { text: "The site's name", lang: "en" }, "English always from the schema");
  const other = entry("home_posts_limit", 0);
  assert.equal(labelOf(pl, other), "Home posts limit");
  assert.deepEqual(descriptionOf(pl, other), { text: "The site's name", lang: "en" });
  assert.equal(descriptionOf(pl, { ...other, node: { type: "int" } }), null);
  assert.deepEqual(groupText(pl, { id: "site", title: "Site", description: "the site" }), { title: "Strona", description: "Nazwa i adres" });
  assert.deepEqual(groupText(en, { id: "site", title: "Site", description: "the site" }), { title: "Site", description: "the site" });
  assert.deepEqual(groupText(en, { id: "advanced", title: "", description: "" }), { title: "Advanced", description: "" });
  assert.deepEqual(groupText(en, { id: "page-tools", title: "", description: "" }).title, "Page tools");
  assert.deepEqual(searchTexts(pl, title), ["Nazwa strony", "Title", "Nazwa w <title>", "the site's name"]);
  assert.deepEqual(searchTexts(en, { ...other, node: { type: "int" } }), ["Home posts limit", "Home posts limit", "", ""]);
  assert.ok(TRANSLATED.length >= 30 && new Set(TRANSLATED).size === TRANSLATED.length);
});
