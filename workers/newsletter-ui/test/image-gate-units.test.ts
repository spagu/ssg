// The image gate (#448), second part: what GET /api/campaigns/:id/images adds
// for the panel, and the gate's helpers one by one. No real network call.
import { test } from "node:test";
import assert from "node:assert/strict";
import { MAX_PROBED_IMAGES } from "../src/image-check.ts";
import { blockingOf, withRules } from "../src/image-gate-rules.ts";
import { changedWhileChecked, gateLanguage, imageGate } from "../src/image-gate.ts";
import { transition } from "../src/campaign-state.ts";
import { call, mailEnv } from "./campaign-fixtures.ts";
import { IMG, create, net, withNet } from "./image-gate-fixtures.ts";

test("GET images: the enforced rules and each image's blocking problems; a broken rule set blocks nothing itself", async () => {
  const env = mailEnv({ NEWSLETTER_IMAGES_REQUIRE: "alt, load" });
  const urls = Array.from({ length: MAX_PROBED_IMAGES + 1 }, (_, i) => `${IMG}/${i}.png`);
  const c = await create(env, [`![](${urls[0]})`, ...urls.slice(1).map((u) => `![x](${u})`)].join("\n\n"));
  const body = (await withNet(net({ [urls[1]]: { status: 404 } }), () => call(env, "GET", `/${c.id}/images`))).body;
  assert.deepEqual([body.rules, body.rulesProblem], [["alt", "load"], null]);
  assert.deepEqual(body.items[0].blocking, ["no_alt"]);
  assert.deepEqual([body.items[1].problems, body.items[1].blocking], [["unreachable"], ["unreachable"]]);
  assert.deepEqual([body.items[MAX_PROBED_IMAGES].problems, body.items[MAX_PROBED_IMAGES].blocking], [[], ["not_probed"]], "problems keeps its 1.9.15 meaning");
  assert.deepEqual([body.checked, body.skipped], [MAX_PROBED_IMAGES, 1]);
  const broken = mailEnv({ NEWSLETTER_IMAGES_REQUIRE: "alt,nope" });
  const d = await create(broken, `![](${IMG}/a.png)`);
  const b2 = (await withNet(net(), () => call(broken, "GET", `/${d.id}/images`))).body;
  assert.deepEqual([b2.rules, b2.items[0].problems, b2.items[0].blocking], [["alt"], ["no_alt"], []]);
  assert.match(b2.rulesProblem, /unknown rule "nope"/);
});

test("units: blockingOf, withRules, gateLanguage, imageGate without a site, changedWhileChecked, the guarded move", async () => {
  const item = { url: "u", alt: null, stored: false, probe: null, problems: ["not_stored", "no_alt"] as never };
  assert.deepEqual(blockingOf(item, ["store"]), ["not_stored"]);
  assert.deepEqual(blockingOf(item, ["load"]), ["not_probed"]);
  assert.deepEqual(blockingOf(item, []), []);
  const report = { items: [item], checked: 0, skipped: 1 };
  assert.equal(withRules(report, { rules: [], problem: null }), report, "untouched when off");
  const env = mailEnv({ NEWSLETTER_IMAGES_REQUIRE: "alt" });
  const c = await create(env, "text");
  assert.equal(gateLanguage({ ...c, segment: { languages: ["de"], tags: [], tagMode: "any" } }), "de");
  assert.equal(gateLanguage(c), "en");
  const noSite = await imageGate({ NEWSLETTER_IMAGES_REQUIRE: "alt" } as never, c);
  assert.deepEqual([noSite!.status, ((await noSite!.json()) as { error: string }).error], [503, "no_site_url"]);
  // Lost for another reason than an edit: the caller says invalid_transition.
  assert.equal(await changedWhileChecked(mailEnv() as never, c), null, "off: no query");
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET status = 'cancelled' WHERE id = ?").run(c.id);
  assert.equal(await changedWhileChecked(env as never, c), null);
  assert.equal(await changedWhileChecked(env as never, { ...c, id: "gone" }), null);
  // The guarded move: only with the same updated_at.
  const d = await create(env, "text");
  assert.equal(await transition(env as never, d.id, "draft", "scheduled", undefined, "2001-01-01T00:00:00.000Z"), false);
  assert.equal(await transition(env as never, d.id, "draft", "scheduled", undefined, d.updatedAt), true);
});
