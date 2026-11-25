// Content rules (#449) through the real pipeline on real SQL: the read-only
// check, the gate in front of scheduling, the settings and the module
// summary. No network: nothing here fetches (the image gate is not set).
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { contentFindings, gateLanguage } from "../src/content-rules.ts";
import { rulesConfig } from "../src/content-rules-config.ts";
import { configProblems } from "../src/module-summary.ts";
import { siteOf } from "../src/render.ts";
import { call, mailEnv } from "./campaign-fixtures.ts";
import { PASSWORD, fakeCtx, req } from "./fakes.ts";

type Env = ReturnType<typeof mailEnv>;
const create = async (env: Env, body: Record<string, unknown>) => (await call(env, "POST", "", { subject: "Hello", body: "Hi there", ...body })).body;
const check = (env: Env, id: string, query = "") => call(env, "GET", `/${id}/content-check${query}`);
const statusOf = (env: Env, id: string) => (env.NEWSLETTER_DB.sqlite.prepare("SELECT status FROM campaigns WHERE id = ?").get(id) as { status: string }).status;
const settings = async (env: Env) => (await (await handleFetch(req("/newsletter-admin/api/settings", { auth: PASSWORD }), env as never, fakeCtx() as never)).json()) as Record<string, unknown>;

const RULES = {
  NEWSLETTER_BANNED_PHRASES: "click here|last chance|ostatnia szansa",
  NEWSLETTER_BLOCKED_IMAGE_PATTERNS: "/wp-content/|CDN.EXAMPLE.NET",
  NEWSLETTER_UTM: "on",
  NEWSLETTER_UTM_STRICT: "on",
  NEWSLETTER_UTM_HOSTS: "shop.example.com",
};
const BAD = [
  "Read on: [click here](https://example.com/post?utm_source=blog&utm_medium=x) or [here](https://other.example.org/?utm_source=x).",
  "![Last Chance](https://example.com/wp-content/uploads/a.png) ![ok](https://cdn.example.net/b.png) ![fine](https://example.com/c.png)",
  "A URL with the phrase: https://example.com/click-here is not a hit.",
].join("\n\n");

test("unset: every answer and every mail is what it was before #449", async () => {
  const plain = mailEnv();
  const empty = mailEnv({ NEWSLETTER_BANNED_PHRASES: "", NEWSLETTER_BLOCKED_IMAGE_PATTERNS: " | ", NEWSLETTER_UTM_STRICT: "off" });
  for (const env of [plain, empty]) {
    const c = await create(env, { subject: "Click here", body: BAD });
    const r = await check(env, c.id);
    assert.deepEqual(r.body, { findings: [], rules: { active: false, bannedPhrases: 0, blockedImagePatterns: 0, utmStrict: false, problem: null } });
    assert.equal((await call(env, "POST", `/${c.id}/schedule`, {})).status, 200);
    assert.deepEqual((await settings(env)).contentRules, r.body.rules);
    assert.deepEqual(configProblems(env as never, true), []);
  }
  // The rules only judge: a preview with every rule on is byte for byte the preview without.
  const on = mailEnv({ ...RULES, NEWSLETTER_UTM_STRICT: undefined });
  const off = mailEnv({ NEWSLETTER_UTM: "on", NEWSLETTER_UTM_HOSTS: "shop.example.com" });
  const [a, b] = [await create(on, { body: BAD }), await create(off, { body: BAD })];
  const pa = await call(on, "POST", `/${a.id}/preview`, {});
  const pb = await call(off, "POST", `/${b.id}/preview`, {});
  assert.equal(pa.body.html.replaceAll(a.id, "ID"), pb.body.html.replaceAll(b.id, "ID"));
  assert.equal(pa.body.text.replaceAll(a.id, "ID"), pb.body.text.replaceAll(b.id, "ID"));
});

test("check: every rule names what it found and where", async () => {
  const env = mailEnv(RULES);
  const c = await create(env, { subject: "Last  chance: news", body: BAD, textOverride: "Plain: click HERE https://shop.example.com/x?UTM_campaign=y&utm_campaign=z" });
  const r = await check(env, c.id);
  assert.equal(r.status, 200);
  assert.deepEqual(r.body.rules, { active: true, bannedPhrases: 3, blockedImagePatterns: 2, utmStrict: true, problem: null });
  assert.deepEqual(r.body.findings, [
    { rule: "banned_phrase", where: "subject", value: "last chance", match: "Last chance", count: 1 },
    { rule: "banned_phrase", where: "body", value: "click here", match: "click here", count: 1 },
    { rule: "banned_phrase", where: "body", value: "last chance", match: "Last Chance", count: 1 },
    { rule: "banned_phrase", where: "text", value: "click here", match: "click HERE", count: 1 },
    { rule: "blocked_image", where: "body", value: "https://example.com/wp-content/uploads/a.png", pattern: "/wp-content/" },
    { rule: "blocked_image", where: "body", value: "https://cdn.example.net/b.png", pattern: "CDN.EXAMPLE.NET" },
    { rule: "utm_conflict", where: "body", value: "https://example.com/post?utm_source=blog&utm_medium=x", host: "example.com", parameters: ["utm_source", "utm_medium"] },
    { rule: "utm_conflict", where: "text", value: "https://shop.example.com/x?UTM_campaign=y&utm_campaign=z", host: "shop.example.com", parameters: ["UTM_campaign", "utm_campaign"] },
  ]);
  assert.equal(statusOf(env, c.id), "draft", "a check changes nothing");
  // The phrase list itself is never in the answer, only what was found.
  assert.ok(!JSON.stringify(r.body).includes("ostatnia szansa"));
  assert.ok(!JSON.stringify(await settings(env)).includes("ostatnia szansa"));
});

test("check: the unsubscribe link, mailto, repeats and the footer are not judged", async () => {
  const env = mailEnv({ NEWSLETTER_UTM: "on", NEWSLETTER_UTM_STRICT: "on", NEWSLETTER_BANNED_PHRASES: "unsubscribe|wypisz się" });
  const body = "[a](https://example.com/?utm_source=x) [b](https://example.com/?utm_source=x) [mail](mailto:a@example.com?utm_source=x) [u]({{unsubscribe_url}}&utm_source=x)";
  const c = await create(env, { body, segment: { languages: ["pl"] } });
  const r = await check(env, c.id, "?language=pl");
  assert.deepEqual(r.body.findings.map((f: { rule: string; value: string }) => [f.rule, f.value]), [["utm_conflict", "https://example.com/?utm_source=x"]]);
  const site = siteOf(env) as never;
  const loaded = (await call(env, "GET", `/${c.id}`)).body;
  assert.equal(gateLanguage(loaded), "pl");
  assert.equal(gateLanguage({ ...loaded, segment: { languages: [], tags: [], tagMode: "any" } }), "en");
  const { rules } = rulesConfig(env as never);
  assert.deepEqual(contentFindings(env as never, { ...loaded, body: "Hi", textOverride: "see https://[x]/?utm_source=1" }, site, "en", rules), [], "an unparsable href is not a link to a tagged host");
  const imagesOnly = { phrases: [], imagePatterns: ["/x/"], utmStrict: false };
  assert.deepEqual(contentFindings(env as never, { ...loaded, body: "unsubscribe ![a](https://example.com/x/a.png) [b](https://example.com/?utm_source=x)" }, site, "en", imagesOnly), [
    { rule: "blocked_image", where: "body", value: "https://example.com/x/a.png", pattern: "/x/" },
  ], "each rule runs only when configured");
});

test("check: refusals in their usual shapes", async () => {
  const env = mailEnv(RULES);
  const c = await create(env, {});
  assert.equal((await check(env, c.id, "?language=<x>")).status, 400);
  assert.equal((await check(env, "missing")).status, 404);
  const noSite = mailEnv({ ...RULES, NEWSLETTER_SITE_URL: "http://example.com" });
  const c2 = await create(noSite, {});
  assert.deepEqual([(await check(noSite, c2.id)).status, (await check(noSite, c2.id)).body.error], [503, "no_site_url"]);
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET body = ? WHERE id = ?").run("{{nope}}", c.id);
  const unknown = await check(env, c.id);
  assert.deepEqual([unknown.status, unknown.body], [400, { error: "unknown_variable", detail: "{{nope}}" }]);
  const broken = mailEnv({ NEWSLETTER_UTM_STRICT: "on" });
  const c3 = await create(broken, {});
  const r = await check(broken, c3.id);
  assert.equal(r.status, 503);
  assert.equal(r.body.error, "content_rules_invalid");
});

test("schedule: a finding refuses with 409 and the findings; the draft stays a draft", async () => {
  const env = mailEnv(RULES);
  const c = await create(env, { body: "Do not miss it — click here." });
  const r = await call(env, "POST", `/${c.id}/schedule`, {});
  assert.equal(r.status, 409);
  assert.deepEqual(r.body, {
    error: "content_check_failed",
    detail: "1 finding against the content rules",
    findings: [{ rule: "banned_phrase", where: "body", value: "click here", match: "click here", count: 1 }],
  });
  assert.equal(statusOf(env, c.id), "draft");
  const two = await create(env, { subject: "Last chance", body: "click here" });
  assert.equal((await call(env, "POST", `/${two.id}/schedule`, {})).body.detail, "2 findings against the content rules");
  // Saving and a test send are never affected.
  assert.equal((await call(env, "PATCH", `/${c.id}`, { body: "Still click here, saved." })).status, 200);
  const clean = await create(env, { body: "Nothing to see." });
  assert.equal((await call(env, "POST", `/${clean.id}/schedule`, {})).status, 200);
});

test("schedule: a configuration that cannot be honoured refuses with 503 and is a configuration problem", async () => {
  const env = mailEnv({ NEWSLETTER_BANNED_PHRASES: "x".repeat(101) });
  const c = await create(env, {});
  const r = await call(env, "POST", `/${c.id}/schedule`, {});
  assert.equal(r.status, 503);
  assert.equal(r.body.error, "content_rules_invalid");
  assert.match(r.body.detail, /NEWSLETTER_BANNED_PHRASES has a phrase longer than 100 characters/);
  assert.equal(statusOf(env, c.id), "draft");
  const s = await settings(env);
  assert.equal((s.contentRules as { problem: string }).problem, r.body.detail);
  const problem = configProblems(env as never, true).find((p) => p.code === "content_rules_invalid");
  assert.ok(problem);
  for (const text of Object.values(problem.text)) assert.ok(text.endsWith(r.body.detail));
});
