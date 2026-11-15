// slug.ts against ssg's own models.Slugify, through a sample generated once
// from the Go function (test/slug-samples.json): phrases in several scripts,
// and every Unicode code point that Slugify keeps (the table slug.ts writes
// out). No Go code lives in the repository; to regenerate the sample, put
// this file in scratch space as internal/models/zz_slugsample_test.go via an
// overlay and run, from the repository root:
//
//   SLUG_SAMPLE_OUT=$PWD/workers/editor-ui/test/slug-samples.json \
//     go test -overlay overlay.json -run TestSlugSample -count=1 ./internal/models
//
// with overlay.json = {"Replace": {"<repo>/internal/models/zz_slugsample_test.go": "<scratch>/zz_slugsample_test.go"}}
// and the test body:
//
//   func TestSlugSample(t *testing.T) {
//     phrases := []string{ /* the "phrases" column of the JSON */ }
//     out := map[string]any{}
//     pairs := [][2]string{}
//     for _, p := range phrases { pairs = append(pairs, [2]string{p, Slugify(p)}) }
//     out["phrases"] = pairs
//     runes := [][2]any{}
//     for r := rune(0); r <= utf8.MaxRune; r++ {
//       if r >= 0xD800 && r <= 0xDFFF { continue }
//       if s := Slugify(string(r)); s != "" { runes = append(runes, [2]any{int(r), s}) }
//     }
//     out["runes"] = runes
//     b, _ := json.MarshalIndent(out, "", " ")
//     _ = os.WriteFile(os.Getenv("SLUG_SAMPLE_OUT"), append(b, '\n'), 0o600)
//   }
import { test } from "node:test";
import assert from "node:assert/strict";
import sample from "./slug-samples.json" with { type: "json" };
import { existsSync, readFileSync } from "node:fs";
import { FALLBACK, MAX_SLUG, fileSlug, freeName, slugify, slugifyTranslit } from "../src/slug.ts";
import { BASE, SITE_YAML, makeEnv, request, run, site } from "./fixtures.ts";
import { PERM } from "../src/permissions.ts";
import { clearListingCache } from "../src/content-list.ts";

// ssg's own table for slug_transliterate (#426), shared with the Go test. A
// template scaffolded on its own has no ssg tree beside it: skipped there.
const TRANSLIT = new URL("../../../internal/models/testdata/slug-translit.json", import.meta.url);
const translit = existsSync(TRANSLIT)
  ? (JSON.parse(readFileSync(TRANSLIT, "utf8")) as { phrases: [string, string][]; runes: [number, string][] })
  : null;
const noTable = translit ? false : "internal/models/testdata/slug-translit.json is not next to this template";

test("slugify equals ssg's Slugify on every sampled phrase", () => {
  assert.ok(sample.phrases.length >= 20);
  for (const [phrase, want] of sample.phrases) assert.equal(slugify(phrase), want, JSON.stringify(phrase));
});

test("slugify keeps exactly the code points ssg keeps — every one of them", () => {
  const kept = new Map(sample.runes.map(([cp, s]) => [cp as number, s as string]));
  for (const [cp, want] of kept) assert.equal(slugify(String.fromCodePoint(cp)), want, cp.toString(16));
  for (let cp = 0; cp <= 0x10ffff; cp++) {
    if ((cp >= 0xd800 && cp <= 0xdfff) || kept.has(cp)) continue;
    if (slugify(String.fromCodePoint(cp)) !== "") assert.fail(`U+${cp.toString(16)} is kept here, not by ssg`);
  }
});

test("fileSlug: never empty, cut at a hyphen when long; freeName: the first free name", () => {
  assert.equal(fileSlug("नमस्ते"), FALLBACK);
  assert.equal(fileSlug("Hello World"), "hello-world");
  const long = fileSlug("word ".repeat(40));
  assert.ok(long.length <= MAX_SLUG && !long.endsWith("-") && long.startsWith("word-word"));
  assert.equal(fileSlug("x".repeat(100)), "x".repeat(MAX_SLUG), "no hyphen to cut at");
  const taken = new Set(["c/a.md", "c/a-2.md"]);
  assert.equal(freeName("c", "a", taken), "c/a-3.md");
  assert.equal(freeName("c", "b", taken), "c/b.md");
});

test("slugifyTranslit equals ssg's SlugifyTranslit on every phrase and every code point", { skip: noTable }, () => {
  const t = translit!;
  assert.ok(t.phrases.length >= 30);
  for (const [phrase, want] of t.phrases) assert.equal(slugifyTranslit(phrase), want, JSON.stringify(phrase));
  const kept = new Map(t.runes);
  for (let cp = 0; cp <= 0x10ffff; cp++) {
    if (cp >= 0xd800 && cp <= 0xdfff) continue;
    const got = slugifyTranslit(String.fromCodePoint(cp));
    if (got !== (kept.get(cp) ?? "")) assert.fail(`U+${cp.toString(16)}: "${got}" here, "${kept.get(cp) ?? ""}" in ssg`);
  }
});

test("fileSlug follows the switch; new-path uses it when the site sets slug_transliterate", async () => {
  assert.equal(fileSlug("Zażółć gęślą"), "za-g-l");
  assert.equal(fileSlug("Zażółć gęślą", true), "zazolc-gesla");
  assert.equal(fileSlug("नमस्ते", true), FALLBACK, "other scripts are not romanised");
  clearListingCache();
  const gh = await site({ ".ssg.yaml": SITE_YAML + "slug_transliterate: true\n" });
  const res = await run(await request(BASE + "/api/content/new-path?section=content%2Fblog%2Fposts&title=Stra%C3%9Fe", [PERM.read]), makeEnv(), gh);
  assert.deepEqual(await res.json(), { path: "content/blog/posts/strasse.md", slug: "strasse" });
});
