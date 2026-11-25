// Content rules (#449), the pure parts: reading the configuration, the text a
// reader gets, and how a banned phrase is found in it — Unicode-aware edges,
// case, NFC, whitespace, and a phrase that is data, never a pattern.
import { test } from "node:test";
import assert from "node:assert/strict";
import { MAX_IMAGE_PATTERNS, MAX_PATTERN_LENGTH, MAX_PHRASES, MAX_PHRASE_LENGTH, rulesConfig, rulesSummary, utmFlag } from "../src/content-rules-config.ts";
import { normalise, phraseHits, phraseRegExp, readerText, scannable } from "../src/content-rules-text.ts";
import { sanitizeHtml } from "../src/sanitize.ts";

const hits = (text: string, ...phrases: string[]) =>
  phraseHits(scannable(text), phrases.map((p) => ({ phrase: normalise(p), re: phraseRegExp(normalise(p)) })));
const found = (text: string, phrase: string) => hits(text, phrase).length === 1;

test("phrases: whole words, any case", () => {
  assert.ok(found("Please Click Here today", "click here"));
  assert.ok(found("CLICK HERE.", "click here"));
  assert.ok(!found("clickhere", "click here"));
  assert.ok(!found("Clicking here", "click here"), "a longer word is not the phrase");
  assert.ok(!found("unclick here", "click here"));
  assert.ok(found("(click here)", "click here"), "punctuation is an edge");
  assert.ok(found("buy now", "buy now") && found("buy now!", "buy now"));
  assert.deepEqual(hits("Act now, act NOW", "act now"), [{ phrase: "act now", match: "Act now", count: 2 }]);
});

test("phrases: Polish letters are letters (\\b would get these wrong)", () => {
  assert.ok(found("Ostatnia szansa!", "ostatnia szansa"));
  assert.ok(found("OSTATNIA SZANSA", "ostatnia szansa"), "upper case with the same letters");
  assert.ok(!found("zakup teraz", "kup teraz"), "the phrase starts inside a word");
  assert.ok(found("Kup teraz", "kup teraz"));
  assert.ok(!found("kup terazniejszy", "kup teraz"));
  // ł and ż are letters: "łódź" is one word, "dź" is not found inside it.
  assert.ok(!found("łódź", "dź"));
  assert.ok(found("Żółw", "żółw"), "case folding of Polish letters");
  assert.ok(!found("żółwie", "żółw"));
});

test("phrases: Hindi vowel signs and viramas are part of the word", () => {
  assert.ok(found("आज ही अभी खरीदें!", "अभी खरीदें"));
  // "कर" (do) is not found in "करें" (please do): the vowel sign ें is a combining mark.
  assert.ok(!found("कृपया करें", "कर"));
  assert.ok(found("काम कर दो", "कर"));
  assert.ok(!found("खरीदेंगे", "खरीदें"));
});

test("phrases: NFC on both sides, whitespace runs, invisible characters", () => {
  const decomposed = "Żółw"; // Ż ó as base + combining marks
  assert.ok(found(decomposed, "Żółw"), "decomposed text, composed phrase");
  assert.ok(found("Żółw", decomposed), "composed text, decomposed phrase");
  assert.ok(found("click\n\n   here", "click here"));
  assert.ok(found("click here", "click  here"), "no-break space, and the phrase's own runs");
  assert.ok(found("cli­ck here", "click here"), "a soft hyphen is not a letter break");
  assert.ok(found("click​ here", "click here"));
});

test("phrases: data, never a pattern; a URL is not text", () => {
  for (const p of ["a.b", "(x)", "x+", "[y]", "a|b", "^$", "\\d", "{2}", "*?", "c/d"]) {
    assert.doesNotThrow(() => phraseRegExp(p), p);
  }
  assert.ok(!found("axb", "a.b"));
  assert.ok(found("go a.b go", "a.b"));
  assert.ok(found("100% free", "100% free"));
  assert.ok(!found("1100% free", "100% free"));
  assert.ok(found("50%!", "%"), "a phrase edge that is no letter needs no boundary");
  assert.ok(!found("see https://example.com/click-here/now", "click"), "only inside a URL: no hit");
  assert.ok(!found("mailto:click@example.com", "click"));
  assert.ok(found("click https://example.com/x", "click"), "the text outside the URL still counts");
});

test("phrases: a long text with many near misses stays fast", () => {
  const text = "a".repeat(200_000) + " ";
  const started = performance.now();
  assert.equal(hits(text, "a".repeat(99) + "b", "a a a a").length, 0);
  assert.ok(performance.now() - started < 1000);
});

test("readerText: words, link texts and alt texts — never addresses or titles", () => {
  const tree = sanitizeHtml(
    '<p>Hello <strong>wor</strong>ld</p><p><a href="https://example.com/secret-word" title="tooltip-word">link text</a></p>' +
      '<ul><li>one</li><li>two</li></ul><img src="https://example.com/i.png" alt="picture words"><img src="https://example.com/j.png">end<br>line',
  );
  const t = scannable(readerText(tree));
  assert.equal(t, "Hello world link text one two picture words end line");
});

test("config: off unless set; split, trim, drop empty, distinct ignoring case", () => {
  const off = rulesConfig({} as never);
  assert.deepEqual(off, { rules: { phrases: [], imagePatterns: [], utmStrict: false }, problem: null });
  assert.equal(rulesSummary({} as never).active, false);
  const on = rulesConfig({ NEWSLETTER_BANNED_PHRASES: " Click  here | | click HERE|buy now ", NEWSLETTER_BLOCKED_IMAGE_PATTERNS: "/wp-content/ | CDN.example.com|cdn.EXAMPLE.com" } as never);
  assert.deepEqual(on.rules.phrases, ["Click here", "buy now"]);
  assert.deepEqual(on.rules.imagePatterns, ["/wp-content/", "CDN.example.com"]);
  assert.deepEqual(rulesSummary({ NEWSLETTER_BANNED_PHRASES: "x" } as never), { active: true, bannedPhrases: 1, blockedImagePatterns: 0, utmStrict: false, problem: null });
});

test("config: bounds and NEWSLETTER_UTM_STRICT fail closed with a reason", () => {
  const many = Array.from({ length: MAX_PHRASES + 1 }, (_, i) => `p${i}`).join("|");
  assert.match(rulesConfig({ NEWSLETTER_BANNED_PHRASES: many } as never).problem ?? "", /has 201 phrases, at most 200/);
  const long = "ż".repeat(MAX_PHRASE_LENGTH + 1);
  assert.match(rulesConfig({ NEWSLETTER_BANNED_PHRASES: long } as never).problem ?? "", /longer than 100 characters: "ż{20}…"/);
  assert.equal(rulesConfig({ NEWSLETTER_BANNED_PHRASES: "ż".repeat(MAX_PHRASE_LENGTH) } as never).problem, null, "characters, not UTF-16 units");
  const patterns = Array.from({ length: MAX_IMAGE_PATTERNS + 1 }, (_, i) => `/p${i}/`).join("|");
  assert.match(rulesConfig({ NEWSLETTER_BLOCKED_IMAGE_PATTERNS: patterns } as never).problem ?? "", /NEWSLETTER_BLOCKED_IMAGE_PATTERNS has 51 patterns/);
  assert.match(rulesConfig({ NEWSLETTER_BLOCKED_IMAGE_PATTERNS: "x".repeat(MAX_PATTERN_LENGTH + 1) } as never).problem ?? "", /pattern longer than 200/);
  assert.match(rulesConfig({ NEWSLETTER_UTM_STRICT: "maybe" } as never).problem ?? "", /must be on or off, not "maybe"/);
  const strictOff = rulesConfig({ NEWSLETTER_UTM_STRICT: "on" } as never);
  assert.match(strictOff.problem ?? "", /NEWSLETTER_UTM_STRICT is on but NEWSLETTER_UTM is off/);
  assert.equal(strictOff.rules.utmStrict, false);
  assert.equal(rulesSummary({ NEWSLETTER_UTM_STRICT: "on" } as never).active, true, "a problem makes the rules visible");
  const both = rulesConfig({ NEWSLETTER_UTM_STRICT: "on", NEWSLETTER_BANNED_PHRASES: many } as never).problem ?? "";
  assert.equal(both.split("; ").length, 2, "every problem is named");
  assert.equal(rulesConfig({ NEWSLETTER_UTM_STRICT: "on", NEWSLETTER_UTM: "on" } as never).rules.utmStrict, true);
  for (const [raw, want] of [[undefined, false], ["", false], ["off", false], ["NO", false], ["0", false], ["false", false], ["on", true], ["Yes", true], ["1", true], ["x", null]] as const) {
    assert.equal(utmFlag(raw), want, String(raw));
  }
});
