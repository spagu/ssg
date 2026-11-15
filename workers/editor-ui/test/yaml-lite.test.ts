// The minimal YAML reader: what a hand-written .ssg.yaml uses is read as the
// Go YAML library reads it; anything beyond the subset, inside a wanted key,
// is refused with its line — never guessed. Unwanted keys are skipped.
import { test } from "node:test";
import assert from "node:assert/strict";
import { YamlUnsupported, readKeys } from "../portable/yaml-lite.ts";
import { inlineValue, stripComment, typed } from "../portable/yaml-scalar.ts";

const ALL = new Set(["a", "b", "c", "list", "map", "languages", "mddb"]);
const read = (text: string, keys: Set<string> = ALL) => readKeys(text, keys);
const refused = (text: string, reason: RegExp, line?: number) => {
  try {
    read(text);
  } catch (err) {
    assert.ok(err instanceof YamlUnsupported, String(err));
    assert.match(err.reason, reason);
    if (line !== undefined) assert.equal(err.line, line);
    return;
  }
  assert.fail(`expected a refusal: ${JSON.stringify(text)}`);
};

test("scalars: plain, quoted, typed as YAML 1.2's core schema", () => {
  assert.deepEqual(read("---\na: hello world # note\nb: 'it''s'\nc: \"tab\\there \\u00e9\\x41 \\\" \\\\ \\/\"\n"), { a: "hello world", b: "it's", c: "tab\there éA \" \\ /" });
  assert.deepEqual(read("a: true\nb: ~\nc: -12.5e3\n"), { a: true, b: null, c: -12500 });
  assert.deepEqual([typed("False"), typed("NULL"), typed(""), typed("+7"), typed(".5"), typed("yes"), typed("1.2.3")], [false, null, null, 7, 0.5, "yes", "1.2.3"]);
  assert.equal(stripComment("a: 'x # y' # z"), "a: 'x # y'");
  assert.equal(stripComment('a: "q \\" # still" # gone'), 'a: "q \\" # still"');
  assert.equal(stripComment("a: x#y"), "a: x#y");
  assert.equal(stripComment("# all"), "");
});

test("block sequences and mappings, nested, column-0 sequences, items that are mappings", () => {
  const text = [
    "list:",
    "- one",
    "-   two",
    "- name: x",
    "  type: post",
    "- ",
    "  nested: true",
    "-",
    "map:",
    "  inner:",
    "    - k: 1",
    "      deep:",
    "        z: [1, 'two', {q: \"r\"}]",
    "  empty:",
    "  seq:",
    "  - a",
    "  flow: {tags: [a, b], n: 2}",
    "  emptyFlow: []",
    "  emptyMap: {}",
  ].join("\n");
  assert.deepEqual(read(text), {
    list: ["one", "two", { name: "x", type: "post" }, { nested: true }, null],
    map: { inner: [{ k: 1, deep: { z: [1, "two", { q: "r" }] } }], empty: null, seq: ["a"], flow: { tags: ["a", "b"], n: 2 }, emptyFlow: [], emptyMap: {} },
  });
});

test("unwanted keys are skipped whatever they hold; a wanted key with nothing is null", () => {
  const text = "skip: |\n  anything &a *b\n  - here\nother: &anchor\n  <<: *x\na:\nlist2:\n- skipped\nb: 1\n";
  assert.deepEqual(read(text), { a: null, b: 1 });
  assert.deepEqual(read("\uFEFFa: 1\r\nb: 2\r\n"), { a: 1, b: 2 });
});

test("refused inside a wanted key, with the line", () => {
  refused("a: |\n  text\n", /block value/, 1);
  refused("a: >-\n  text\n", /block value/);
  refused("a: &x 1\n", /anchor, alias or tag/);
  refused("a: *x\n", /anchor, alias or tag/);
  refused("a: !!str 1\n", /anchor, alias or tag/);
  refused("map:\n  <<: *base\n", /merge key/, 2);
  refused("map:\n  ? complex\n", /cannot follow/, 2);
  refused("list:\n  - <<: x\n", /merge key/);
  refused("a: first\n  continued\n", /continued on the next line/, 2);
  refused("map:\n  k: v\n    more\n", /continued on the next line/, 3);
  refused("list:\n  - x\n    y\n", /continued on the next line/, 3);
  refused("a: [1,\n  2]\n", /continued on the next line|flow collection/);
  refused("a: [1, 2\n", /flow collection that continues/);
  refused("a: 'open\n", /quoted value that continues/);
  refused("a: \"bad \\q\"\n", /escape this reader does not know/);
  refused("a: \"bad \\u12G4\"\n", /malformed escape/);
  refused("a: 'x' y\n", /text after the end/);
  refused("a: {k}\n", /without a value/);
  refused("a: {k: 1, k: 2}\n", /the key "k" twice/);
  refused("a: [[1] 2]\n", /cannot follow/);
  assert.deepEqual(read("a: [1 2]\n"), { a: ["1 2"] });
  refused("a: [&x 1]\n", /anchor, alias or tag/);
  refused("map:\n  k: 1\n  k: 2\n", /the key "k" twice/, 3);
  refused("a: 1\na: 2\n", /the key "a" twice/, 2);
  refused("map:\n    k: 1\n  j: 2\n", /indentation/, 3);
  refused("list:\n  - a\n    - b\n", /continued on the next line/);
  refused("list:\n    - a\n  - b\n", /indentation/);
  refused("map:\n  k:\n      - a\n    - b\n", /indentation/);
});

test("refused anywhere: tabs, several documents, directives, lines it cannot follow", () => {
  refused("zzz:\n\tx: 1\n", /tab/, 2);
  refused("a: 1\n---\nb: 2\n", /several YAML documents/, 2);
  refused("a: 1\n...\n", /several YAML documents/);
  refused("%YAML 1.2\na: 1\n", /directive/);
  refused('"quoted": 1\n', /cannot follow/);
  refused(" a: 1\n", /indentation/);
});

test("inlineValue: whole-line values", () => {
  assert.deepEqual(inlineValue(" [a, [b, c], {d: e}] ", 1), ["a", ["b", "c"], { d: "e" }]);
  assert.deepEqual(inlineValue("{a: 'x', \"b\": [1]}", 1), { a: "x", b: [1] });
  assert.equal(inlineValue("http://example.com:8080/x", 1), "http://example.com:8080/x");
});

test("keys named like Object properties are stored as data, never as the prototype", () => {
  const out = readKeys("mddb:\n  __proto__:\n    polluted: true\n  toString: plain\n  constructor: x\n", new Set(["mddb"]));
  const mddb = out.mddb as Record<string, unknown>;
  assert.equal(Object.getPrototypeOf(mddb), Object.prototype);
  assert.deepEqual(Object.keys(mddb), ["__proto__", "toString", "constructor"]);
  assert.deepEqual(Object.getOwnPropertyDescriptor(mddb, "__proto__")?.value, { polluted: true });
  assert.equal(mddb.toString, "plain");
  assert.equal(({} as Record<string, unknown>).polluted, undefined);
  assert.throws(() => readKeys("mddb:\n  toString: a\n  toString: b\n", new Set(["mddb"])), YamlUnsupported);
});
