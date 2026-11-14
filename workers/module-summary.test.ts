// The module summary contract (GO-148) on its own: the reader
// (readSummary in module-manifest.ts) against module-summary.schema.json —
// caps, languages, links, the contract rule — and the kit every summarising
// template carries a byte-identical copy of. Run from the repository root:
//   node --test 'workers/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { SUMMARY_CONTRACT, SUMMARY_MAX_BYTES, readSummary } from "./module-manifest.ts";
import { SUMMARY_LIMITS, SUMMARY_CONTRACT as KIT_CONTRACT, counted, summaryOf } from "./module-summary-kit.ts";

const schema = JSON.parse(readFileSync(new URL("./module-summary.schema.json", import.meta.url), "utf8"));
const text = (s: string) => ({ en: s, pl: s, hi: s, de: s, fr: s });
const valid = () => ({
  contract: 1,
  generatedAt: "2026-11-13T10:00:00.000Z",
  metrics: [{ key: "pending", label: text("Pending"), value: 3 }, { key: "share", label: text("Share"), value: 12.5, unit: "percent" }],
  attention: [{ severity: "action", text: text("3 comments await moderation."), href: "/comments-admin.html" }, { severity: "info", text: text("x"), href: "/#/campaigns?status=paused" }],
  problems: [{ code: "no_turnstile_secret", text: text("No secret.") }],
  series: { label: text("Sign-ups"), points: [{ day: "2026-11-12", value: 0 }, { day: "2026-11-13", value: 4 }] },
});
const errorsOf = (s: unknown) => {
  const r = readSummary(s, schema);
  return r.ok ? [] : r.errors;
};
const many = <T>(n: number, f: (i: number) => T) => Array.from({ length: n }, (_, i) => f(i));

test("summary reader: a valid summary is returned as is; unknown fields and languages are ignored", () => {
  const s = { ...valid(), later: true, problems: [{ code: "x", text: { ...text("t"), es: "t" } }] };
  const r = readSummary(s, schema);
  assert.ok(r.ok);
  assert.equal(r.summary, s);
  const { series: _, ...noSeries } = valid();
  assert.deepEqual(errorsOf(noSeries), [], "series is optional");
  assert.equal(SUMMARY_CONTRACT, KIT_CONTRACT, "the kit writes the contract the reader reads");
});

test("summary reader: a newer contract is refused first, with what to do", () => {
  assert.deepEqual(errorsOf({ contract: 2 }), ["this summary uses contract 2, but this reader understands contract 1 at most — upgrade ssg (and the core worker) to read it"]);
  assert.match(errorsOf("nope")[0], /^summary: must be object, not string/);
  assert.match(errorsOf(undefined).join("\n"), /summary: must be object, not undefined/);
});

test("summary reader: hard caps — four metrics, ten items, ten problems, 90 points, the byte size", () => {
  const cases: [Record<string, unknown>, RegExp][] = [
    [{ metrics: many(5, (i) => ({ key: `k${i}`, label: text("l"), value: i })) }, /summary\.metrics: at most 4 entries/],
    [{ attention: many(11, () => valid().attention[0]) }, /summary\.attention: at most 10 entries/],
    [{ problems: many(11, () => valid().problems[0]) }, /summary\.problems: at most 10 entries/],
    [{ series: { label: text("s"), points: many(91, (i) => ({ day: `2026-01-${String(i).padStart(2, "0")}`, value: 1 })) } }, /summary\.series\.points: at most 90 entries/],
    [{ problems: [{ code: "x", text: text("t".repeat(401)) }] }, /summary\.problems\[0\]\.text\.en: longer than 400 characters/],
  ];
  for (const [patch, want] of cases) assert.match(errorsOf({ ...valid(), ...patch }).join("\n"), want);
  const big = { ...valid(), padding: "x".repeat(SUMMARY_MAX_BYTES) };
  assert.deepEqual(errorsOf(big), [`summary: ${JSON.stringify(big).length} bytes, more than ${SUMMARY_MAX_BYTES}`]);
  assert.deepEqual(SUMMARY_LIMITS, { metrics: 4, attention: 10, problems: 10, points: 90 }, "the kit's caps are the schema's");
});

test("summary reader: five languages in every text, none blank", () => {
  const { pl: _, ...noPl } = text("t");
  assert.match(errorsOf({ ...valid(), problems: [{ code: "x", text: noPl }] }).join("\n"), /summary\.problems\[0\]\.text\.pl: is required/);
  assert.match(errorsOf({ ...valid(), metrics: [{ key: "k", label: { ...text("t"), hi: "" }, value: 1 }] }).join("\n"), /label\.hi: must not be empty/);
  assert.match(errorsOf({ ...valid(), attention: [{ severity: "info", text: { ...text("t"), de: 3 }, href: "/" }] }).join("\n"), /text\.de: must be string/);
});

test("summary reader: an attention link is a panel path, never a URL", () => {
  for (const href of ["/", "/comments-admin.html", "/#/", "/#/subscribers?status=pending", "/admin/"]) {
    assert.deepEqual(errorsOf({ ...valid(), attention: [{ severity: "warning", text: text("t"), href }] }), [], href);
  }
  for (const href of ["https://example.com/", "//example.com/", "javascript:alert(1)", "/a/../b", "/./a", "relative", "/a?x=1", "/a b", "", "/#<script>", "/a\\b"]) {
    assert.match(errorsOf({ ...valid(), attention: [{ severity: "warning", text: text("t"), href }] }).join("\n"), /href: .* does not match/, href);
  }
});

test("summary reader: field rules — severity, keys, codes, numbers, dates, series order", () => {
  const cases: [Record<string, unknown>, RegExp][] = [
    [{ attention: [{ severity: "critical", text: text("t"), href: "/" }] }, /severity: must be one of "action", "warning", "info"/],
    [{ attention: [{ text: text("t"), href: "/" }] }, /attention\[0\]\.severity: is required/],
    [{ metrics: [{ key: "Bad-Key", label: text("l"), value: 1 }] }, /metrics\[0\]\.key: "Bad-Key" does not match/],
    [{ metrics: [{ key: "k", label: text("l"), value: -1 }] }, /metrics\[0\]\.value: must be at least 0/],
    [{ metrics: [{ key: "k", label: text("l"), value: "3" }] }, /metrics\[0\]\.value: must be number, not string/],
    [{ metrics: [{ key: "k", label: text("l"), value: 1, unit: "euro" }] }, /metrics\[0\]\.unit: must be one of/],
    [{ metrics: [{ key: "k", label: text("l"), value: 1 }, { key: "k", label: text("l"), value: 2 }] }, /summary\.metrics: key "k" appears twice/],
    [{ problems: [{ code: "NoDB", text: text("t") }] }, /problems\[0\]\.code: "NoDB" does not match/],
    [{ generatedAt: "yesterday" }, /generatedAt: "yesterday" does not match/],
    [{ generatedAt: "2026-13-45T99:99:99Z" }, /generatedAt: "2026-13-45T99:99:99Z" is not a date/],
    [{ contract: 0 }, /summary\.contract: must be at least 1/],
    [{ series: { label: text("s"), points: [{ day: "2026-11-13", value: 1 }, { day: "2026-11-12", value: 1 }] } }, /days must be in ascending order, each once/],
    [{ series: { label: text("s"), points: [{ day: "13.11.2026", value: 1 }] } }, /day: "13\.11\.2026" does not match/],
  ];
  for (const [patch, want] of cases) assert.match(errorsOf({ ...valid(), ...patch }).join("\n"), want, JSON.stringify(patch));
  const { metrics: _, ...noMetrics } = valid();
  assert.match(errorsOf(noMetrics).join("\n"), /summary\.metrics: is required/);
});

test("summary kit: what it builds passes the reader; plural forms per language", () => {
  const phrase = {
    en: { one: "{count} comment", other: "{count} comments" },
    pl: { one: "{count} komentarz", few: "{count} komentarze", many: "{count} komentarzy", other: "{count} komentarza" },
    hi: { one: "{count} टिप्पणी", other: "{count} टिप्पणियाँ" },
    de: { one: "{count} Kommentar", other: "{count} Kommentare" },
    fr: { one: "{count} commentaire", other: "{count} commentaires" },
  };
  assert.deepEqual(counted(phrase, 1), { en: "1 comment", pl: "1 komentarz", hi: "1 टिप्पणी", de: "1 Kommentar", fr: "1 commentaire" });
  assert.deepEqual([2, 5, 22, 25].map((n) => counted(phrase, n).pl), ["2 komentarze", "5 komentarzy", "22 komentarze", "25 komentarzy"]);
  assert.equal(counted(phrase, 1500).en, "1,500 comments");
  assert.equal(counted({ ...phrase, en: { other: "{count}" } }, 1).en, "1", "a missing form falls back to other");
  const s = summaryOf(
    {
      metrics: many(6, (i) => ({ key: `k${i}`, label: text("l"), value: i })),
      attention: [{ severity: "info", text: text("i"), href: "/" }, { severity: "warning", text: text("w"), href: "/" }, { severity: "action", text: text("a"), href: "/" }],
      problems: many(12, (i) => ({ code: `p${i}`, text: text("t") })),
      series: { label: text("s"), points: many(95, (i) => ({ day: new Date(Date.UTC(2026, 0, 1 + i)).toISOString().slice(0, 10), value: i })) },
    },
    new Date("2026-11-13T10:00:00.000Z"),
  );
  assert.deepEqual(s.attention.map((a) => a.severity), ["action", "warning", "info"]);
  assert.equal(s.series!.points[0].value, 5, "the newest 90 days are kept");
  assert.deepEqual(errorsOf(s), []);
  assert.deepEqual(errorsOf(summaryOf({ metrics: [] })), []);
});

test("summary kit: every template's copy is byte for byte the kit", () => {
  const kit = readFileSync(new URL("./module-summary-kit.ts", import.meta.url), "utf8");
  for (const copy of ["comments/functions/api/comments/_summary-kit.ts", "newsletter/functions/api/newsletter/_summary-kit.ts", "newsletter-ui/src/summary-kit.ts"]) {
    assert.equal(readFileSync(new URL(`./${copy}`, import.meta.url), "utf8"), kit, `workers/${copy} differs from workers/module-summary-kit.ts — copy it again`);
  }
});
