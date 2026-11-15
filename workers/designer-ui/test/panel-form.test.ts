// GO-179: the Options tab's form model against the real schema and a real
// answer of GET api/settings — every named key of config-schema.json is in
// the form, in the right group, as a field or a heading; and the search and
// the "only changed" filter (public/js/models/form-model.js).
import { test } from "node:test";
import assert from "node:assert/strict";
import { ADVANCED, buildForm, counts, fieldsOf, filterForm, groupOf, humanise, isSection, matches, normalise } from "../public/js/models/form-model.js";
import { fromDraft, toDraft } from "../public/js/models/controls.js";
import { SCHEMA, namedNodes } from "../src/schema.ts";
import { READ, site } from "./fixtures.ts";
import { get } from "./helpers.ts";

const gh = await site({ ".ssg.designer.json": JSON.stringify({ title: "From the panel", mddb: { lang: "pl" } }) });
const view = (await get(gh, "/api/settings", READ)).body;
const form = buildForm(view);
const placed = new Map<string, string[]>();
for (const g of form.groups) for (const i of g.items) placed.set(i.entry.segments.join("."), [...(placed.get(i.entry.segments.join(".")) ?? []), `${g.id}:${i.type}`]);

test("every named key of the schema is in the form", () => {
  const named = namedNodes(SCHEMA.keys);
  assert.ok(named.length > 300, String(named.length));
  assert.equal(view.entries.length, named.length);
  for (const { segments } of named) assert.ok(placed.has(segments.join(".")), segments.join("."));
});

test("a key is a field once, in its group — or in Advanced when it runs code or is a secret", () => {
  for (const e of view.entries) {
    const where = placed.get(e.segments.join("."))!;
    const fields = where.filter((w) => w.endsWith(":field"));
    if (isSection(e)) {
      assert.equal(fields.length, 0, `${e.path} is a heading`);
      continue;
    }
    assert.deepEqual(fields, [`${groupOf(e)}:field`], e.path);
    if (e.class !== "plain") assert.equal(groupOf(e), ADVANCED, e.path);
  }
  assert.equal(groupOf(form.entries.get("hooks")), ADVANCED);
  assert.equal(groupOf(form.entries.get("jwt_secret")), ADVANCED);
  assert.equal(groupOf(form.entries.get("title")), "site");
  // mddb runs code, mddb.lang does not: the heading appears in both groups
  assert.ok(placed.get("mddb")!.includes(`${ADVANCED}:section`));
  assert.ok(placed.get("mddb")!.includes("sources:section"));
});

test("groups: the schema's, in its order, then Advanced; a group is empty only when all its keys run code", () => {
  assert.deepEqual(form.groups.map((g) => g.id), [...SCHEMA.groups.map((g) => g.id), ADVANCED]);
  const c = counts(form);
  for (const g of form.groups) {
    const plain = view.entries.filter((e: { class: string; node: { group: string } }) => e.class === "plain" && e.node.group === g.id && !isSection(e as never));
    assert.equal(c[g.id].fields > 0, g.id === ADVANCED || plain.length > 0, g.id);
  }
  assert.equal(c.deploy.fields, 0, "every deployment key runs code");
  assert.equal(c.site.overlay, 1, "title comes from the panel");
  assert.equal(fieldsOf(form).length, view.entries.filter((e: never) => !isSection(e)).length);
});

test("headings sit right above their fields, outermost first", () => {
  const items = form.groups.find((g) => g.id === ADVANCED)!.items;
  const i = items.findIndex((x) => x.entry.path === "mcp.git.repo");
  assert.ok(i > 0);
  const heads = items.slice(0, i).filter((x) => x.type === "section").map((x) => x.entry.path);
  assert.deepEqual(heads.slice(-2), ["mcp", "mcp.git"]);
  assert.equal(items[i].depth, 2);
  assert.equal(items[items.indexOf(items.find((x) => x.entry.path === "mcp")!)].depth, 0);
});

test("every field's value turns into a draft and back without a problem", () => {
  for (const e of fieldsOf(form)) {
    const r = fromDraft(toDraft(e.node, e.value));
    // a secret written by hand as a literal (jwt_secret) can only be replaced
    const allowed = new Set(["secretReEnter", "secretName", "required"]);
    // (an unset secret has no name yet; untouched, nothing is published for it)
    for (const err of r.errors) assert.ok(allowed.has(err.code), `${e.path}: ${err.code}`);
  }
});

test("search: by key, by words of the key, by description, in any case", () => {
  const all = (q: string) => filterForm(form, { query: q }).flatMap((g) => g.items.filter((i) => i.type === "field").map((i) => i.entry.path));
  assert.ok(all("pretty_urls").includes("pretty_urls"));
  assert.ok(all("Pretty URLs").includes("pretty_urls"));
  assert.ok(all("table of contents").includes("toc"), "found by its description");
  assert.deepEqual(all("zzz-nothing"), []);
  assert.equal(all("").length, fieldsOf(form).length);
  assert.ok(all("lang").includes("mddb.lang"));
  const groups = filterForm(form, { query: "mddb lang" });
  assert.ok(groups.some((g) => g.items.some((i) => i.type === "section" && i.entry.path === "mddb")), "the heading stays above a match");
  const texts = filterForm(form, { query: "Nazwa strony", textsOf: (e: { path: string }) => (e.path === "title" ? ["Nazwa strony"] : []) });
  assert.deepEqual(texts.flatMap((g) => g.items.map((i) => i.entry.path)), ["title"], "found by a translated label");
});

test("only changed: set in the panel, or edited and not yet published", () => {
  const changed = filterForm(form, { changed: true, pending: new Set(["toc"]) });
  const paths = changed.flatMap((g) => g.items.filter((i) => i.type === "field").map((i) => i.entry.path)).sort();
  assert.deepEqual(paths, ["mddb.lang", "title", "toc"]);
  assert.equal(changed.find((g) => g.id === "site")!.count, 1);
});

test("normalise, matches and humanise", () => {
  assert.equal(filterForm(form).length, form.groups.filter((g) => g.items.some((i) => i.type === "field")).length, "no options: everything");
  assert.equal(normalise("Prétty_URLs.x[]*"), "pretty urls x");
  assert.equal(normalise(null), "");
  assert.equal(matches({ segments: ["a"] }, "  ", []), true);
  assert.equal(humanise("home_posts_limit"), "Home posts limit");
  assert.equal(humanise(""), "");
  assert.equal(humanise(undefined), "");
});

test("a group the schema does not list lands in the first one, never lost", () => {
  const f = buildForm({ groups: [{ id: "site", title: "Site", description: "" }], entries: [{ path: "new_key", segments: ["new_key"], node: { type: "bool", group: "future" }, class: "plain", source: "default" }] });
  assert.deepEqual(f.groups[0].items.map((i: { entry: { path: string } }) => i.entry.path), ["new_key"]);
  assert.equal(f.groups[1].title, "Advanced");
  assert.equal(groupOf({ class: "plain", node: {} }), "site");
});
