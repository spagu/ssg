// Segments on a seeded database: confirmed only, languages (with regions),
// whole-tag matching, any/all — real SQL on SQLite.
import { test } from "node:test";
import assert from "node:assert/strict";
import { countSegment, parseSegment, segmentWhere } from "../src/segment.ts";
import type { Segment } from "../src/segment.ts";
import { fakeD1, withSubscribers } from "./fakes.ts";
import { seed } from "./campaign-fixtures.ts";
import type { SubscriberSeed } from "./campaign-fixtures.ts";

const ROWS: SubscriberSeed[] = [
  ["a", "confirmed", "en", "news"],
  ["b", "confirmed", "pl", "news,offers"],
  ["c", "confirmed", "pt-BR", "offers"],
  ["d", "confirmed", "pt", "newsletter"],
  ["e", "confirmed", null, ""],
  ["f", "pending", "en", "news"],
  ["g", "unsubscribed", "pl", "news,offers"],
  ["h", "confirmed", "en", "new_s,old-x"],
];

function db() {
  const d1 = withSubscribers(fakeD1());
  seed(d1, ROWS);
  return { NEWSLETTER_DB: d1 } as never;
}

const ids = async (env: never, s: Segment) => {
  const { sql, binds } = segmentWhere(s);
  const { results } = await (env as { NEWSLETTER_DB: ReturnType<typeof fakeD1> }).NEWSLETTER_DB.prepare(`SELECT id FROM subscribers WHERE ${sql} ORDER BY id`)
    .bind(...binds)
    .all<{ id: string }>();
  return results.map((r) => r.id).join("");
};
const seg = (s: Partial<Segment>): Segment => ({ languages: [], tags: [], tagMode: "any", ...s });

test("always confirmed only", async () => {
  const env = db();
  assert.equal(await ids(env, seg({})), "abcdeh");
  assert.deepEqual(await countSegment(env, seg({})), { total: 6, byLanguage: { "": 1, en: 2, pl: 1, pt: 1, "pt-BR": 1 } });
});

test("languages: exact, and a bare language takes its regions", async () => {
  const env = db();
  assert.equal(await ids(env, seg({ languages: ["pt"] })), "cd");
  assert.equal(await ids(env, seg({ languages: ["pt-BR"] })), "c");
  assert.equal(await ids(env, seg({ languages: ["en", "pl"] })), "abh");
  assert.equal(await ids(env, seg({ languages: ["de"] })), "");
});

test("tags: whole words only, any vs all, LIKE wildcards are literal", async () => {
  const env = db();
  assert.equal(await ids(env, seg({ tags: ["news"] })), "ab", "newsletter must not match news");
  assert.equal(await ids(env, seg({ tags: ["new"] })), "");
  assert.equal(await ids(env, seg({ tags: ["news", "offers"], tagMode: "any" })), "abc");
  assert.equal(await ids(env, seg({ tags: ["news", "offers"], tagMode: "all" })), "b");
  assert.equal(await ids(env, seg({ tags: ["new_s"] })), "h");
  assert.equal(await ids(env, seg({ tags: ["new%"] })), "");
  assert.equal(await ids(env, seg({ tags: ["offers"], languages: ["pl"] })), "b");
  assert.deepEqual(await countSegment(env, seg({ tags: ["zzz"] })), { total: 0, byLanguage: {} });
});

test("parseSegment: normalises, refuses what it cannot use", () => {
  assert.deepEqual(parseSegment(undefined), seg({}));
  assert.deepEqual(parseSegment(null), seg({}));
  assert.deepEqual(parseSegment({ languages: ["PT-br", "pt-BR", "EN"], tags: [" News ", "news"], tagMode: "all" }), {
    languages: ["pt-BR", "en"],
    tags: ["news"],
    tagMode: "all",
  });
  for (const bad of [
    "x",
    [],
    { languages: "en" },
    { languages: [1] },
    { tags: Array(51).fill("a") },
    { languages: ["english"] },
    { tags: ["a,b"] },
    { tags: [""] },
    { tagMode: "some" },
  ]) {
    assert.equal(typeof parseSegment(bad), "string", JSON.stringify(bad));
  }
});
