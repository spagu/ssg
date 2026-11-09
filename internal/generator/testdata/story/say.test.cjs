// node --test internal/generator/testdata/story/ — the browser side of the
// pronunciation rules: the shared cases and every built-in set's examples,
// the same ones story_say_test.go runs against the Go engine.
const test = require("node:test");
const assert = require("node:assert");
const fs = require("fs");
const path = require("path");
const make = require("../../assets/story/story-say.js");
const shared = require("./pronounce.json");

test("shared cases", () => {
  const say = make(shared.rules, "pl");
  for (const [given, want] of shared.cases) assert.strictEqual(say(given), want, given);
});

test("built-in sets read their own examples", () => {
  const dir = path.join(__dirname, "../../assets/story/pronounce");
  const files = fs.readdirSync(dir).filter(f => f.endsWith(".json"));
  assert.ok(files.length >= 2);
  for (const f of files) {
    const set = JSON.parse(fs.readFileSync(path.join(dir, f), "utf8"));
    const say = make(set.rules, set.lang);
    assert.ok(set.examples.length, f);
    for (const [given, want] of set.examples) assert.strictEqual(say(given), want, f + ": " + given);
  }
});

test("edges", () => {
  assert.strictEqual(make(null, "pl")("100g"), "100g");
  assert.strictEqual(make({ "{num} g": "{num} gram|grams" }, "en")("1 g, 2 g, 1.5 g"), "1 gram, 2 grams, 1.5 grams");
  assert.strictEqual(make({ "{num} cup|cups": "{num:one|two} {word}" }, "en")("2 cups"), "2 cups");
  assert.strictEqual(make({ "szt.": "sztuka|sztuki" }, "pl")("3 szt."), "3 sztuka");
  assert.strictEqual(make({ "{num} g": "{num} gram|gramy" }, "pl")("1,5 g"), "1,5 gramy");
  const words = make({ "{num} x": "{num:jedna|dwie}" }, "pl-PL");
  assert.strictEqual(words("12002 x"), "dwanaście tysięcy dwie");
  assert.strictEqual(make({ "{num} g": "{num} g|gs" })("2 g"), "2 gs");
});
