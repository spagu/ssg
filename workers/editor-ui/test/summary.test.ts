// The module summary: numbers, the branch, the repository's and the
// configuration's problems — aggregates and fixed sentences only.
import { test } from "node:test";
import assert from "node:assert/strict";
import { BASE, GIT_VARS, folderHistory, makeEnv, request, run, site } from "./fixtures.ts";
import { handleFetch } from "../portable/app.ts";
import { PERM } from "../src/permissions.ts";
import { MAX_HISTORY, PANEL } from "../src/summary.ts";
import { LABELS, PROBLEMS } from "../src/summary-texts.ts";
import { SUMMARY_LANGUAGES, counted, summaryOf } from "../src/summary-kit.ts";

async function summary(env: Record<string, unknown> = makeEnv(), files: Record<string, string | null> = {}, tweak?: (gh: Awaited<ReturnType<typeof site>>) => void) {
  const gh = await site(files);
  tweak?.(gh);
  const res = await run(await request(BASE + "/api/module/summary", [PERM.read]), env, gh);
  assert.equal(res.status, 200);
  return res.json();
}
const codes = (s: { problems: { code: string }[] }) => s.problems.map((p) => p.code);

test("a healthy site: days since the content changed, content folders, the branch — nobody's data", async () => {
  const gh = await site();
  await gh.store.commitFiles("main", { "docs/guide.md": "# Guide v2\n" }, "Ada Lovelace <ada@example.com> rewrote the guide");
  // deps.now is the verifier's clock too: the session is issued at that time.
  const now = Date.UTC(2026, 0, 11, 12);
  const at = Math.floor(now / 1000);
  const r = await request(BASE + "/api/module/summary", [PERM.read], { claims: { iat: at, exp: at + 600 } });
  const res = await handleFetch(r, makeEnv() as never, { fetch: folderHistory(gh), now: () => now });
  const s = await res.json();
  assert.equal(s.generatedAt, new Date(now).toISOString());
  assert.deepEqual(s.metrics, [
    { key: "daysSinceContentChange", label: LABELS.daysSinceContentChange, value: 10, unit: "days" },
    { key: "contentFolders", label: LABELS.contentFolders, value: 2 },
  ]);
  assert.deepEqual(s.attention.map((a: { severity: string; href: string }) => [a.severity, a.href]), [["info", PANEL]]);
  for (const l of SUMMARY_LANGUAGES) assert.match(s.attention[0].text[l], / main/);
  assert.deepEqual(s.problems, []);
  const text = JSON.stringify(s);
  for (const leak of ["@", "Ada", "Lovelace", "rewrote", "Seed", GIT_VARS.EDITOR_GIT_TOKEN]) assert.ok(!text.includes(leak), leak);
});

test("no history for any folder: the days metric is left out", async () => {
  const gh = await site();
  const fetch = async (url: string, init?: RequestInit) => (url.includes("/commits?") ? Response.json([]) : gh.fetch(url, init));
  const s = await (await handleFetch(await request(BASE + "/api/module/summary", [PERM.read]), makeEnv() as never, { fetch })).json();
  assert.deepEqual(s.metrics.map((m: { key: string }) => m.key), ["contentFolders"]);
});

test("history is asked for at most MAX_HISTORY folders", async () => {
  const gh = await site({ ".ssg.yaml": "content_sources:\n" + ["a", "b", "c", "d", "e"].map((d) => `  - path: ${d}\n`).join("") });
  const asked: string[] = [];
  const fetch = async (url: string, init?: RequestInit) => {
    if (url.includes("/commits?")) asked.push(new URL(url).searchParams.get("path")!);
    return gh.fetch(url, init);
  };
  await handleFetch(await request(BASE + "/api/module/summary", [PERM.read]), makeEnv() as never, { fetch });
  assert.deepEqual(asked, ["a", "b", "c"].slice(0, MAX_HISTORY));
});

test("repository problems, each in its own words", async () => {
  assert.deepEqual(codes(await summary(makeEnv({ EDITOR_GIT_REPO: undefined }))), ["repo_not_configured"]);
  assert.deepEqual(codes(await summary(makeEnv({ EDITOR_GIT_BRANCH: "nope" }))), ["repo_not_found"]);
  const status = (n: number) => (gh: Awaited<ReturnType<typeof site>>) => {
    gh.override = () => Response.json({ message: "x" }, { status: n });
  };
  assert.deepEqual(codes(await summary(makeEnv(), {}, status(401))), ["repo_token_rejected"]);
  assert.deepEqual(codes(await summary(makeEnv(), {}, status(500))), ["repo_unreachable"]);
  const noBranch = await summary(makeEnv({ EDITOR_GIT_BRANCH: "" }), {}, status(500));
  assert.deepEqual([codes(noBranch), noBranch.attention], [["repo_unreachable"], []], "the default branch is unknown when the host fails");
});

test("configuration problems: unreadable, no content folders, a bad override, overridden, partial", async () => {
  assert.deepEqual(codes(await summary(makeEnv(), { ".ssg.yaml": "source: *x\n" })), ["site_config_unreadable"]);
  assert.deepEqual(codes(await summary(makeEnv(), { ".ssg.yaml": "title: x\n" })), ["no_content_dirs"]);
  assert.deepEqual(codes(await summary(makeEnv({ EDITOR_CONTENT_DIRS: "../x" }))), ["bad_override"]);
  const overridden = await summary(makeEnv({ EDITOR_CONTENT_DIRS: "docs" }), { ".ssg.yaml": "source: *x\n" });
  assert.deepEqual([codes(overridden), overridden.metrics.at(-1).value], [["site_config_overridden"], 1]);
  assert.deepEqual(codes(await summary(makeEnv(), { ".ssg.yaml": "source: blog\nstatic_dir: $STATIC\n" })), ["site_config_partial"]);
});

test("every problem code has its text in all five languages, within the 400-character limit", () => {
  for (const [code, text] of Object.entries(PROBLEMS)) {
    assert.match(code, /^[a-z]+(?:_[a-z]+)*$/);
    for (const l of SUMMARY_LANGUAGES) assert.ok(text[l].length > 0 && text[l].length <= 400, `${code} ${l}`);
  }
});

test("a failure that is not the repository's is a 500 without its text", async () => {
  const gh = await site();
  const fetch = async (url: string, init?: RequestInit) => {
    if (!url.includes("/git/blobs/")) return gh.fetch(url, init);
    const body = new ReadableStream({ pull: (c) => c.error(new TypeError("stream broke")) });
    return new Response(body, { status: 200 });
  };
  const log = console.error;
  console.error = () => {};
  try {
    const res = await handleFetch(await request(BASE + "/api/module/summary", [PERM.read]), makeEnv() as never, { fetch });
    assert.deepEqual([res.status, await res.json()], [500, { error: "internal_error" }]);
  } finally {
    console.error = log;
  }
});

test("the kit's counted() in this copy: plural forms per language", () => {
  const phrase = Object.fromEntries(SUMMARY_LANGUAGES.map((l) => [l, { one: "{count} file", other: "{count} files" }])) as Parameters<typeof counted>[0];
  assert.equal(counted(phrase, 1).en, "1 file");
  assert.equal(counted(phrase, 1200).de, "1.200 files");
  assert.equal(counted({ ...phrase, pl: { other: "{count} plików" } }, 2).pl, "2 plików");
});

test("the kit's summaryOf() in this copy: attention by severity, optional parts, series capped", () => {
  const t = Object.fromEntries(SUMMARY_LANGUAGES.map((l) => [l, "x"])) as Parameters<typeof summaryOf>[0]["metrics"][0]["label"];
  const s = summaryOf({ metrics: [], attention: [{ severity: "info", text: t, href: "/" }, { severity: "action", text: t, href: "/" }] }, new Date(0));
  assert.deepEqual([s.attention.map((a) => a.severity), s.problems, "series" in s], [["action", "info"], [], false]);
  const points = Array.from({ length: 95 }, (_, i) => ({ day: `d${i}`, value: i }));
  const bare = summaryOf({ metrics: [], series: { label: t, points } });
  assert.deepEqual([bare.attention, bare.series?.points.length, bare.series?.points[0].day], [[], 90, "d5"]);
});
