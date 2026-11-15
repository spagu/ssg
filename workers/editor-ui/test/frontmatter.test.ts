// frontmatter.ts / frontmatter-fields.ts against a corpus: a save without a
// change gives the file back byte for byte; a change to one key changes only
// its lines; what the editor cannot change safely is refused by name.
import { test } from "node:test";
import assert from "node:assert/strict";
import { NEW_FILE, applyEdit, joinDocument, readFields, splitDocument } from "../src/frontmatter.ts";
import { fieldsOf, linesOf, setFields } from "../src/frontmatter-fields.ts";

const CORPUS: Record<string, string> = {
  plain: "---\ntitle: Hello\nstatus: publish\n---\n# Hello\n\nText.\n",
  comments: "---\n# a comment first\ntitle: Hello # trailing\n\n# between\ndate: 2026-07-14\n---\nBody\n",
  quotes: `---\ntitle: "Say \\"hi\\" \\u00e9"\nslug: 'it''s'\ndescription: "Sensors: humidity"\n---\nx\n`,
  lists: "---\ntags: [a, \"b, c\", 3]\ncategories:\n  - Guides\n  - 'News'\nkeys:\n- x\n- y\n---\n",
  nested: "---\ntitle: T\nauthor:\n  name: Ada\n  email: ada@example.com\nseo: {robots: noindex}\nnote: |\n  line one\n  line two\n---\nB\n",
  crlf: "---\r\ntitle: Windows\r\ntags:\r\n  - a\r\n---\r\nBody\r\nline\r\n",
  bom: "\uFEFF---\ntitle: BOM\n---\nBody\n",
  dashesInBody: "---\ntitle: Rules\n---\nAbove\n---\nBelow\n\n---\n",
  none: "# Just a heading\n\ntext --- with dashes\n---\n",
  toml: "+++\ntitle = \"T\"\n+++\nBody\n",
  unclosed: "---\ntitle: open\nno end\n",
  lead: "\n\n---\ntitle: Late\n---\nb",
  anchors: "---\nbase: &b x\nalias: *b\ntwice: 1\ntwice: 2\n\"quoted key\": v\n? complex\nblock: >\n  folded\n---\n",
  empty: "---\n---\n",
  noEol: "---\ntitle: x\n---",
};

test("split/join and an empty edit give every file back byte for byte", () => {
  for (const [name, text] of Object.entries(CORPUS)) {
    assert.equal(joinDocument(splitDocument(text)), text, name);
    const d = splitDocument(text);
    const r = applyEdit(text, { fields: readFields(d).fields, body: d.body });
    assert.deepEqual(r, { text, refused: [] }, name);
    assert.deepEqual(applyEdit(text, {}), { text, refused: [] }, name);
  }
});

test("formats: yaml, toml (kept, body editable), none, unclosed; BOM and leading blank lines", () => {
  const f = (k: string) => splitDocument(CORPUS[k]).format;
  assert.deepEqual([f("plain"), f("toml"), f("none"), f("unclosed"), f("lead"), f("bom")], ["yaml", "toml", "none", "unclosed", "yaml", "yaml"]);
  assert.equal(splitDocument(CORPUS.bom).bom, true);
  assert.equal(splitDocument(CORPUS.dashesInBody).body, "Above\n---\nBelow\n\n---\n");
  assert.equal(splitDocument(CORPUS.lead).head, "\n\n---\n");
  assert.equal(splitDocument("").format, "none");
  assert.equal(splitDocument("---").format, "unclosed");
  assert.equal(splitDocument("\uFEFF \n").format, "none", "a BOM is not blank (Go's TrimSpace keeps it)");
  assert.equal(splitDocument(" --- \t\n a\n---  \n").format, "yaml", "fences are trimmed as ssg trims them");
  const toml = applyEdit(CORPUS.toml, { body: "New\n" });
  assert.equal(toml.text, "+++\ntitle = \"T\"\n+++\nNew\n");
  assert.deepEqual(applyEdit(CORPUS.toml, { fields: { title: "x" } }).refused, [{ key: "title", reason: "toml_frontmatter" }]);
  assert.equal(applyEdit(CORPUS.toml, { frontmatterRaw: "title = \"U\"" }).text, "+++\ntitle = \"U\"\n+++\nBody\n");
  assert.deepEqual(applyEdit(CORPUS.none, { fields: { title: "x" } }).refused, [{ key: "title", reason: "no_frontmatter" }]);
  assert.deepEqual(applyEdit(CORPUS.unclosed, { frontmatterRaw: "a: 1\n" }).refused, [{ key: "frontmatterRaw", reason: "unclosed_frontmatter" }]);
  assert.equal(applyEdit(CORPUS.none, { body: "x" }).text, "x");
  assert.deepEqual(readFields(splitDocument(CORPUS.toml)), { fields: {}, fixed: [] });
});

test("readFields: simple keys with their values; everything else fixed", () => {
  const r = (k: string) => readFields(splitDocument(CORPUS[k]));
  assert.deepEqual(r("comments").fields, { title: "Hello", date: "2026-07-14" });
  assert.deepEqual(r("quotes"), { fields: { title: 'Say "hi" é', slug: "it's", description: "Sensors: humidity" }, fixed: [] });
  assert.deepEqual(fieldsOf("description: Sensors: humidity\n").fixed, ["description"], "an unquoted ': ' is YAML's own trap — not simple");
  assert.deepEqual(r("lists").fields, { tags: ["a", "b, c", "3"], categories: ["Guides", "News"], keys: ["x", "y"] });
  assert.deepEqual(r("nested"), { fields: { title: "T" }, fixed: ["author", "seo", "note"] });
  assert.deepEqual(r("crlf").fields, { title: "Windows", tags: ["a"] });
  assert.deepEqual(r("anchors"), { fields: {}, fixed: ["base", "alias", "twice", "block"] });
  assert.deepEqual(fieldsOf("draft: true\nweight: 1.5\nempty:\nnil: ~\n"), { fields: { draft: true, weight: 1.5, empty: null, nil: null }, fixed: [] });
  assert.deepEqual(fieldsOf("a:\n  - x\n  # c\n  - y\nb:\n  - x # c\nc:\n  - x\n   - y\nd:\n  - k: v\ne:\n  - [x]\nf: [[x]]\ng: x\n  continued\n").fixed, ["a", "b", "c", "d", "e", "f", "g"]);
  assert.deepEqual(fieldsOf("__proto__: [x]\nconstructor: c\n"), { fields: { constructor: "c" }, fixed: ["__proto__"] });
  assert.deepEqual(fieldsOf("  stray: 1\nk: v\n"), { fields: { k: "v" }, fixed: [] }, "an indented line before any key is no key");
});

test("a change to one key changes only its lines — comments, order and line ends kept", () => {
  const one = applyEdit(CORPUS.comments, { fields: { title: "Bye" } });
  assert.equal(one.text, CORPUS.comments.replace("title: Hello # trailing", "title: Bye # trailing"));
  const crlf = applyEdit(CORPUS.crlf, { fields: { tags: ["a", "b"] } });
  assert.equal(crlf.text, "---\r\ntitle: Windows\r\ntags:\r\n  - a\r\n  - b\r\n---\r\nBody\r\nline\r\n");
  const added = applyEdit(CORPUS.crlf, { fields: { draft: false } });
  assert.equal(added.text, "---\r\ntitle: Windows\r\ntags:\r\n  - a\r\ndraft: false\r\n---\r\nBody\r\nline\r\n");
  const lists = applyEdit(CORPUS.lists, { fields: { tags: ["x y", "a,b"], categories: [], keys: "solo" } });
  assert.equal(lists.text, "---\ntags: [x y, \"a,b\"]\ncategories: []\nkeys: solo\n---\n");
  const quotes = applyEdit(CORPUS.quotes, { fields: { title: "New", slug: "o'k" } });
  assert.equal(quotes.text, `---\ntitle: "New"\nslug: 'o''k'\ndescription: "Sensors: humidity"\n---\nx\n`);
  const bom = applyEdit(CORPUS.bom, { fields: { title: "Kept" } });
  assert.equal(bom.text, "\uFEFF---\ntitle: Kept\n---\nBody\n");
  assert.equal(applyEdit(CORPUS.nested, { fields: { title: "U" } }).text, CORPUS.nested.replace("title: T", "title: U"));
});

test("values are written so they read back as themselves", () => {
  const cases: [unknown, string][] = [
    ["plain words", "plain words"], ["yes", '"yes"'], ["true", '"true"'], ["12", '"12"'], ["a: b", '"a: b"'], ["- x", '"- x"'],
    [" pad", '" pad"'], ["", '""'], ["line\nbreak\t\u0001\u2028", '"line\\nbreak\\t\\u0001\\u2028"'], ['q"\\', 'q"\\'], ['"q', '"\\"q"'],
    ["x #y", '"x #y"'], [7, "7"], [true, "true"], [null, ""],
  ];
  for (const [value, written] of cases) {
    const r = setFields("k: old\n", { k: value }, "\n");
    assert.equal(r.raw, `k:${written === "" ? "" : " " + written}\n`, String(value));
    assert.deepEqual(fieldsOf(r.raw).fields.k, value, String(value));
  }
  assert.equal(setFields("k: 'old'\n", { k: "a\nb" }, "\n").raw, 'k: "a\\nb"\n', "single quotes cannot hold a line break");
  assert.equal(setFields("k: \"old\"\n", { k: "x" }, "\n").raw, 'k: "x"\n');
  assert.equal(setFields("k:  # note\n", { k: "v" }, "\n").raw, "k:  v # note\n");
  assert.equal(setFields("k: v # note\n", { k: null }, "\n").raw, "k: # note\n");
  assert.deepEqual(setFields("k:# odd\n", { k: null }, "\n").refused, [{ key: "k", reason: "not_simple" }], "no second k");
  assert.equal(setFields("k: [a]\n", { k: ["a"] }, "\n").raw, "k: [a]\n");
  assert.equal(setFields("t:\n  - a\n", { t: "s" }, "\n").raw, "t: s\n");
  assert.equal(setFields("", { n: null, l: ["[x]"] }, "\n").raw, 'n:\nl: ["[x]"]\n');
  assert.equal(setFields("t: a", { t: "b" }, "\r\n").raw, "t: b\r\n", "a last line without its end takes the file's");
  assert.equal(setFields("t:\n  - a", { t: ["b"] }, "\r\n").raw, "t:\n  - b\n", "list items take the key line's");
});

test("refusals: nothing changes and every refused key is named", () => {
  const r = applyEdit(CORPUS.anchors, { fields: { base: "x", twice: 3, "bad key": 1, quoted: 1, k: { a: 1 }, n: Infinity, s: "\uD800", l: [1] } });
  assert.equal(r.text, CORPUS.anchors);
  assert.deepEqual(r.refused, [
    { key: "base", reason: "not_simple" }, { key: "twice", reason: "duplicate_key" }, { key: "bad key", reason: "bad_key" },
    { key: "k", reason: "bad_value" }, { key: "n", reason: "bad_value" }, { key: "s", reason: "bad_value" }, { key: "l", reason: "bad_value" },
  ]);
  assert.deepEqual(setFields("'quoted': 1\n", { quoted: 2 }, "\n").refused, [{ key: "quoted", reason: "not_simple" }]);
  assert.deepEqual(applyEdit(CORPUS.plain, { fields: { a: 1 }, frontmatterRaw: "a: 1" }).refused, [{ key: "frontmatterRaw", reason: "fields_and_raw" }]);
});

test("frontmatterRaw replaces the block as text; a new file starts from NEW_FILE", () => {
  assert.equal(applyEdit(CORPUS.crlf, { frontmatterRaw: "title: R" }).text, "---\r\ntitle: R\r\n---\r\nBody\r\nline\r\n");
  assert.equal(applyEdit(CORPUS.plain, { frontmatterRaw: "" }).text, "---\n---\n# Hello\n\nText.\n");
  assert.equal(applyEdit(NEW_FILE, { fields: { title: "New post", tags: ["a"], status: "draft" }, body: "Hi\n" }).text, "---\ntitle: New post\ntags: [a]\nstatus: draft\n---\nHi\n");
});

// linesOf's pattern was regrouped (SonarCloud S5850: the "$" belongs to the
// second alternative only, and now says so). The split must not move by a
// byte: the old ungrouped pattern is kept here as the reference.
test("linesOf splits exactly as the ungrouped pattern did", () => {
  const before = (text: string): string[] => text.match(/[^\n]*\n|[^\n]+$/g) ?? [];
  const cases = ["", "\n", "\n\n", "a", "a\n", "a\nb", "a\nb\n", "a\r\nb\r\n", "\r", "x\n\n\ny", ...Object.values(CORPUS)];
  for (const text of cases) assert.deepEqual(linesOf(text), before(text), JSON.stringify(text));
  assert.deepEqual(linesOf("a\nb"), ["a\n", "b"]);
  assert.deepEqual(linesOf(""), []);
});
