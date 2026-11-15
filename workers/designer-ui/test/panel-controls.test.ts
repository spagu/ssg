// GO-179: every control kind of the Options tab, both ways — value → draft →
// value — plus the problems a draft reports (public/js/models/controls.js).
import { test } from "node:test";
import assert from "node:assert/strict";
import { addRow, emptyDraft, formNode, fromDraft, kindOf, moveRow, removeRow, toDraft } from "../public/js/models/controls.js";

const n = (type: string, extra: Record<string, unknown> = {}) => ({ path: "x", type, ...extra });
const roundTrip = (node: object, value: unknown) => fromDraft(toDraft(node, value)).value;

test("switch, number, text, select: value → draft → value", () => {
  assert.equal(toDraft(n("bool"), true).kind, "switch");
  assert.equal(roundTrip(n("bool"), true), true);
  assert.equal(roundTrip(n("bool"), false), false);
  assert.equal(roundTrip(n("bool"), undefined), false);
  assert.equal(toDraft(n("int"), 7).kind, "number");
  assert.equal(roundTrip(n("int"), 7), 7);
  assert.equal(roundTrip(n("number"), 1.5), 1.5);
  assert.equal(roundTrip(n("number"), -2e3), -2000);
  assert.equal(toDraft(n("string"), "a").kind, "text");
  assert.equal(roundTrip(n("string"), "Example"), "Example");
  assert.equal(roundTrip(n("string"), ""), "");
  assert.equal(toDraft(n("string"), 42).text, "42", "a value in another form shows as text");
  assert.equal(toDraft(n("string"), null).text, "");
  const sel = n("enum", { enum: ["", "date", "slug"] });
  assert.equal(toDraft(sel, "slug").kind, "select");
  assert.equal(roundTrip(sel, "slug"), "slug");
  assert.equal(roundTrip(sel, ""), "");
});

test("number: what is not a number, not an integer, or missing", () => {
  const d = toDraft(n("int"), 3);
  for (const [text, code] of [["", "required"], ["abc", "number"], ["1.5", "integer"], ["0x10", "number"], ["Infinity", "number"]]) {
    d.text = text;
    assert.deepEqual(fromDraft(d).errors, [{ at: [], code }], text);
  }
  d.text = " 12 ";
  assert.deepEqual(fromDraft(d), { value: 12, errors: [] });
  d.text = "";
  assert.deepEqual(fromDraft(d, { field: true }), { value: undefined, errors: [] }, "an empty field is left out");
});

test("secret: only a reference is ever built, in each format", () => {
  const whole = n("string", { class: "secret", secretFormat: "whole" });
  const d = toDraft(whole, { set: true, reference: "$JWT_SECRET" });
  assert.equal(d.kind, "secret");
  assert.equal(d.name, "JWT_SECRET");
  assert.equal(fromDraft(d).value, "$JWT_SECRET");
  d.name = "hunter2 is my password";
  assert.deepEqual(fromDraft(d).errors, [{ at: [], code: "secretName" }]);
  // set by hand as a literal: shown as set, kept only if replaced
  const lit = toDraft(whole, { set: true, reference: null });
  assert.equal(lit.keep, true);
  assert.deepEqual(fromDraft(lit).errors, [{ at: [], code: "secretReEnter" }]);
  lit.name = "NEW_SECRET";
  assert.equal(fromDraft(lit).value, "$NEW_SECRET");
  // not set, as a field of an object: left out
  const unset = toDraft(whole, { set: false, reference: null });
  assert.deepEqual(fromDraft(unset, { field: true }), { value: undefined, errors: [] });
  const login = toDraft(n("string", { class: "secret", secretFormat: "login" }), { set: true, reference: "admin:${PASS}" });
  assert.deepEqual([login.login, login.name], ["admin", "PASS"]);
  assert.equal(fromDraft(login).value, "admin:$PASS");
  const inline = toDraft(n("string", { class: "secret", secretFormat: "inline" }), "Bearer $TOKEN");
  assert.deepEqual([inline.scheme, inline.name], ["Bearer", "TOKEN"]);
  assert.equal(fromDraft(inline).value, "Bearer $TOKEN");
});

test("lines: a list of scalars, one per line, typed by its items", () => {
  const strings = n("list", { items: n("string") });
  assert.equal(toDraft(strings, ["a", "b"]).kind, "lines");
  assert.deepEqual(roundTrip(strings, ["/blog/", "/docs/"]), ["/blog/", "/docs/"]);
  assert.deepEqual(roundTrip(strings, undefined), []);
  const d = toDraft(strings, []);
  d.text = " a \n\n b \r\n";
  assert.deepEqual(fromDraft(d).value, ["a", "b"]);
  const ints = n("list", { items: n("int") });
  assert.deepEqual(roundTrip(ints, [320, 640]), [320, 640]);
  const bad = toDraft(ints, []);
  bad.text = "320\nwide\n1.5";
  assert.deepEqual(fromDraft(bad).errors, [{ at: [1], code: "integer" }, { at: [2], code: "integer" }]);
  const nums = toDraft(n("list", { items: n("number") }), []);
  nums.text = "1.5\nx";
  assert.deepEqual(fromDraft(nums), { value: [1.5], errors: [{ at: [1], code: "number" }] });
  const en = toDraft(n("list", { items: n("enum", { enum: ["webp", "avif"] }) }), ["webp"]);
  en.text = "webp\ngif";
  assert.deepEqual(fromDraft(en).errors, [{ at: [1], code: "enum" }]);
  const bools = toDraft(n("list", { items: n("bool") }), [true]);
  assert.equal(bools.text, "true");
  bools.text = "true\nfalse\nyes";
  assert.deepEqual(fromDraft(bools), { value: [true, false], errors: [{ at: [2], code: "bool" }] });
});

test("lines of secrets: references only; one set by hand blocks until replaced", () => {
  const users = n("list", { class: "secret", secretFormat: "login", items: n("string") });
  const d = toDraft(users, [{ set: true, reference: "ada:$ADA_PASS" }, { set: true, reference: null }]);
  assert.equal(d.text, "ada:$ADA_PASS\n?");
  assert.deepEqual(fromDraft(d).errors, [{ at: [1], code: "secretLine" }]);
  d.text = "ada:$ADA_PASS\nbob:$BOB_PASS";
  assert.deepEqual(fromDraft(d), { value: ["ada:$ADA_PASS", "bob:$BOB_PASS"], errors: [] });
  assert.equal(toDraft(users, [{ set: false, reference: null }]).text, "");
});

const redirects = n("list", {
  items: n("object", {
    fields: [
      { path: "r.from", key: "from", type: "string" },
      { path: "r.to", key: "to", type: "string" },
      { path: "r.status", key: "status", type: "int" },
      { path: "r.force", key: "force", type: "bool" },
      { path: "r.cache", key: "cache", type: "bool", default: true },
    ],
  }),
});

test("rows: a list of objects, a field per schema field, empty ones left out", () => {
  const value = [{ from: "/old/*", to: "/new/:splat", status: 301 }, { from: "/a", to: "/b", force: true, cache: false }];
  const d = toDraft(redirects, value);
  assert.equal(d.kind, "rows");
  assert.equal(d.rows.length, 2);
  assert.equal(d.rows[0].kind, "fields");
  assert.deepEqual(fromDraft(d).value, value);
  d.rows[0].fields.status.text = "x";
  assert.deepEqual(fromDraft(d).errors, [{ at: [0, "status"], code: "number" }]);
  d.rows[0].fields.status.text = "301";
  addRow(d);
  d.rows[2].fields.from.text = "/c";
  assert.deepEqual(fromDraft(d).value[2], { from: "/c" });
  moveRow(d, 2, -1);
  assert.equal(fromDraft(d).value[1].from, "/c");
  moveRow(d, 0, -1);
  moveRow(d, 2, 1);
  assert.equal(fromDraft(d).value[0].from, "/old/*", "a move past either end does nothing");
  removeRow(d, 1);
  assert.deepEqual(fromDraft(d).value.map((r: { from: string }) => r.from), ["/old/*", "/a"]);
  // unknown keys of a row are carried through untouched
  assert.deepEqual(roundTrip(redirects, [{ from: "/x", note: "kept" }]), [{ from: "/x", note: "kept" }]);
});

test("map: a key and a value per row, typed by the map's values", () => {
  const colors = n("map", { values: n("string") });
  const d = toDraft(colors, { primary: "#0050a6", text: "#0f172a" });
  assert.equal(d.kind, "map");
  assert.deepEqual(fromDraft(d).value, { primary: "#0050a6", text: "#0f172a" });
  addRow(d);
  assert.deepEqual(fromDraft(d).errors, [{ at: [2], code: "emptyKey" }]);
  d.rows[2].key = "primary";
  assert.deepEqual(fromDraft(d).errors, [{ at: [2], code: "duplicateKey" }]);
  d.rows[2].key = "__proto__";
  assert.deepEqual(fromDraft(d).errors, [{ at: [2], code: "badKey" }]);
  d.rows[2].key = " accent ";
  d.rows[2].d.text = "#e07a5f";
  assert.deepEqual(fromDraft(d).value, { primary: "#0050a6", text: "#0f172a", accent: "#e07a5f" });
  assert.deepEqual(roundTrip(colors, undefined), {});
  // a map of lists (hooks), of objects (models) and of secrets (headers)
  const hooks = n("map", { values: n("list", { items: n("string") }) });
  assert.deepEqual(roundTrip(hooks, { pre_build: ["echo a", "echo b"] }), { pre_build: ["echo a", "echo b"] });
  const models = n("map", { values: n("object", { fields: [{ path: "m.url", key: "url", type: "string" }, { path: "m.key", key: "key", type: "string", class: "secret", secretFormat: "whole" }] }) });
  const md = toDraft(models, { "gpt-4.1": { url: "https://example.com/v1", key: { set: true, reference: "$AI_KEY" } } });
  assert.deepEqual(fromDraft(md).value, { "gpt-4.1": { url: "https://example.com/v1", key: "$AI_KEY" } });
  addRow(md);
  md.rows[1].key = "local";
  assert.deepEqual(fromDraft(md).value.local, {}, "a new model with nothing set");
  const headers = n("map", { class: "secret", secretFormat: "inline", values: n("string") });
  const hd = toDraft(headers, { Authorization: { set: true, reference: "Bearer $HOOK_TOKEN" } });
  assert.equal(hd.rows[0].d.kind, "secret");
  assert.deepEqual(fromDraft(hd).value, { Authorization: "Bearer $HOOK_TOKEN" });
  const bools = toDraft(n("map", { values: n("bool") }), { posts: true });
  addRow(bools);
  bools.rows[1].key = "pages";
  assert.deepEqual(fromDraft(bools).value, { posts: true, pages: false });
});

test("fields: an object inside a row, its own fields recursively", () => {
  const workers = n("list", { class: "executes", items: n("object", { fields: [{ path: "w.name", key: "name", type: "string" }, { path: "w.auth", key: "auth", type: "object", fields: [{ path: "w.auth.token", key: "token", type: "string", class: "secret", secretFormat: "whole" }] }] }) });
  const d = toDraft(workers, [{ name: "comments", auth: { token: { set: true, reference: null } } }]);
  assert.deepEqual(fromDraft(d).errors, [{ at: [0, "auth", "token"], code: "secretReEnter" }]);
  d.rows[0].fields.auth.fields.token.name = "COMMENTS_TOKEN";
  assert.deepEqual(fromDraft(d).value, [{ name: "comments", auth: { token: "$COMMENTS_TOKEN" } }]);
  d.rows[0].fields.auth.fields.token.name = "";
  d.rows[0].fields.auth.fields.token.keep = false;
  assert.deepEqual(fromDraft(d).value, [{ name: "comments" }], "an object left empty is left out");
  assert.deepEqual(roundTrip(n("object"), undefined), {});
});

test("json: a free-form value, read back as JSON or as text", () => {
  const any = n("any");
  assert.equal(toDraft(any, "x").kind, "json");
  for (const v of ["plain words", "true", "42", 42, true, { a: [1, 2] }, [1, "x"]]) assert.deepEqual(roundTrip(any, v), v, JSON.stringify(v));
  const d = toDraft(any, undefined);
  assert.equal(d.text, "");
  assert.deepEqual(fromDraft(d).errors, [{ at: [], code: "required" }]);
  assert.deepEqual(fromDraft(d, { field: true }), { value: undefined, errors: [] });
  // a list of lists has no row editor: JSON
  const nested = n("list", { items: n("list", { items: n("string") }) });
  assert.equal(kindOf(nested), "json");
  assert.deepEqual(roundTrip(nested, [["a"], ["b"]]), [["a"], ["b"]]);
});

test("a second accepted form: the value's form wins, an unset value takes the first", () => {
  const story = n("object", { fields: [{ path: "story.enabled", key: "enabled", type: "bool" }], alternate: n("bool") });
  assert.equal(formNode(story, true).type, "bool");
  assert.equal(toDraft(story, true).kind, "switch");
  assert.equal(roundTrip(story, true), true);
  assert.equal(toDraft(story, undefined).kind, "fields");
  assert.equal(toDraft(story, { enabled: true }).kind, "fields");
  const prefix = n("list", { items: n("string"), alternate: n("string") });
  assert.equal(roundTrip(prefix, "/blog/"), "/blog/");
  assert.deepEqual(roundTrip(prefix, ["/a/", "/b/"]), ["/a/", "/b/"]);
  const languages = n("list", { items: n("string"), alternate: n("list", { items: n("object", { fields: [{ path: "l.code", key: "code", type: "string" }] }) }) });
  assert.equal(toDraft(languages, ["en"]).kind, "lines", "both forms are lists: the first is taken");
  assert.equal(kindOf(n("any"), "whole"), "json", "a free-form value inside a secret key is never a secret field");
  const free = n("string", { alternate: n("any") });
  assert.equal(formNode(free, [1]).type, "any", "an alternate that takes anything");
  const bare = toDraft(n("string", { class: "secret" }), "$X");
  assert.deepEqual([bare.kind, bare.format], ["secret", "whole"], "a secret without a format is whole");
});

test("emptyDraft: the draft a new row starts from", () => {
  assert.equal(emptyDraft(n("string")).text, "");
  assert.equal(emptyDraft(n("string", { class: "secret", secretFormat: "whole" })).kind, "secret");
  assert.equal(emptyDraft(n("string"), "login").kind, "secret", "inside a secret key");
  assert.equal(emptyDraft(n("string", { class: "plain" }), "whole").kind, "text", "a plain field of a secret owner is plain");
});
