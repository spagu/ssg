// The image gate (#448): NEWSLETTER_IMAGES_REQUIRE turns chosen findings of
// the image check into a refusal to schedule. Real SQL, the whole pipeline;
// never a real network call (fetch is replaced for every request).
import { test } from "node:test";
import assert from "node:assert/strict";
import { MAX_PROBED_IMAGES } from "../src/image-check.ts";
import { imageRules } from "../src/image-gate-rules.ts";
import { configProblems } from "../src/module-summary.ts";
import { settingsOf } from "../src/settings.ts";
import { call, mailEnv } from "./campaign-fixtures.ts";
import { fakeR2 } from "./fakes.ts";
import { IMG, KEY, OWN, SITE, create, net, row, schedule, withNet } from "./image-gate-fixtures.ts";

test("unset or empty: today's behaviour, byte for byte — no request, no guard", async () => {
  const bodies: string[] = [];
  for (const value of [undefined, "", " , \t"]) {
    const env = mailEnv({ NEWSLETTER_IMAGES_REQUIRE: value });
    const c = await create(env, `![](${IMG}/a.png)`);
    const n = net({ [`${IMG}/a.png`]: { status: 403, type: "text/html" } });
    const images = await withNet(n, () => call(env, "GET", `/${c.id}/images`));
    assert.equal(
      JSON.stringify(images.body),
      `{"items":[{"url":"${IMG}/a.png","alt":"","stored":false,"probe":{"via":"fetch","ok":false,"status":403,"contentType":"text/html","error":null,"agent":"GoogleImageProxy"},"problems":["no_alt","unreachable"]}],"checked":1,"skipped":0}`,
    );
    const quiet = net();
    const r = await withNet(quiet, () => schedule(env, c.id));
    assert.equal(r.status, 200);
    assert.deepEqual(quiet.calls, [], "scheduling makes no request");
    assert.ok(env.NEWSLETTER_DB.calls.some((q: string) => q.startsWith("UPDATE campaigns SET status")) && env.NEWSLETTER_DB.calls.every((q: string) => !q.includes("updated_at = ?6")), "the move is today's UPDATE");
    bodies.push(JSON.stringify({ ...r.body, id: "", createdAt: "", updatedAt: "", scheduledAt: "" }));
    assert.deepEqual(settingsOf(env as never).imageRules, { rules: [], problem: null });
  }
  assert.equal(new Set(bodies).size, 1);
});

test("alt: missing, empty and blank alt in Markdown and HTML fail; real alt passes; no network", async () => {
  const env = mailEnv({ NEWSLETTER_IMAGES_REQUIRE: "alt" });
  const md = await create(env, [`![](${IMG}/m1.png)`, `![  ](${IMG}/m2.png)`, `![A cat](${IMG}/ok1.png)`].join("\n\n"));
  const html = await create(
    env,
    [
      `<img src="${IMG}/h1.png">`,
      `<img src="${IMG}/h2.png" alt>`,
      `<img src="${IMG}/h3.png" alt="">`,
      `<img src="${IMG}/h4.png" alt=''>`,
      `<img src="${IMG}/h5.png" alt="  ">`,
      `<img src="${IMG}/h6.png" alt=>`,
      `<IMG SRC="${IMG}/h7.png" ALT="&nbsp;">`,
      `<img alt="Logo" src="${IMG}/ok2.png">`,
      `<img src=${IMG}/ok3.png alt=Chart>`,
    ].join("\n"),
    "html",
  );
  const n = net();
  const a = await withNet(n, () => schedule(env, md.id));
  assert.equal(a.status, 409);
  assert.equal(a.body.error, "image_check_failed");
  assert.deepEqual(a.body.rules, ["alt"]);
  assert.equal(a.body.detail, "2 images break the enforced rules (alt)");
  assert.deepEqual(a.body.items.map((i: { url: string }) => i.url), [`${IMG}/m1.png`, `${IMG}/m2.png`]);
  assert.deepEqual(a.body.items[1], { url: `${IMG}/m2.png`, alt: "  ", stored: false, probe: null, problems: ["no_alt"], blocking: ["no_alt"] });
  const b = await withNet(net(), () => schedule(env, html.id));
  assert.deepEqual(b.body.items.map((i: { url: string }) => i.url.slice(IMG.length + 1)), ["h1.png", "h2.png", "h3.png", "h4.png", "h5.png", "h6.png", "h7.png"]);
  assert.ok(b.body.items.every((i: { problems: string[] }) => i.problems.join() === "no_alt"));
  assert.deepEqual(n.calls, [], "alt needs no network");
  // Refused: the campaign did not move and was not touched.
  assert.equal(row(env, md.id).status, "draft");
  assert.deepEqual(row(env, html.id), { status: "draft", updated_at: html.updatedAt });
  // One image: the singular detail. Fixed: it schedules.
  const one = await create(env, `![](${IMG}/x.png)`);
  assert.equal((await withNet(net(), () => schedule(env, one.id))).body.detail, "1 image breaks the enforced rules (alt)");
  const fine = await create(env, `![A cat](${IMG}/ok1.png) and text`);
  assert.equal((await withNet(net(), () => schedule(env, fine.id))).status, 200);
});

test("never gated: a test send, saving a draft, resume", async () => {
  const env = mailEnv({ NEWSLETTER_IMAGES_REQUIRE: "alt,load" });
  const c = await create(env, `![](${IMG}/x.png)`);
  const original = globalThis.fetch;
  globalThis.fetch = (async () => new Response("{}", { status: 200 })) as typeof fetch;
  try {
    assert.equal((await call(env, "POST", `/${c.id}/test`, { to: "me@example.com" })).status, 200);
  } finally {
    globalThis.fetch = original;
  }
  assert.equal((await call(env, "PATCH", `/${c.id}`, { body: `![](${IMG}/y.png)` })).status, 200);
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET status = 'paused' WHERE id = ?").run(c.id);
  const n = net();
  assert.equal((await withNet(n, () => call(env, "POST", `/${c.id}/resume`))).body.status, "sending");
  assert.deepEqual(n.calls, []);
});

test("store: an image off the store's base fails; a store that is off or broken refuses with 503", async () => {
  const env = mailEnv({ NEWSLETTER_IMAGES_REQUIRE: "store", NEWSLETTER_ASSETS: fakeR2() });
  const c = await create(env, `![Logo](${OWN}) ![Elsewhere](${IMG}/x.png)`);
  const r = await withNet(net(), () => schedule(env, c.id));
  assert.equal(r.status, 409);
  assert.deepEqual(r.body.items.map((i: { url: string; problems: string[] }) => [i.url, i.problems]), [[`${IMG}/x.png`, ["not_stored"]]]);
  for (const [over, detail] of [
    [{}, /image store is off/],
    [{ NEWSLETTER_ASSETS: fakeR2(), NEWSLETTER_ASSETS_URL: "http://cdn.example.com" }, /no usable address: NEWSLETTER_ASSETS_URL must be an https/],
  ] as const) {
    const e = mailEnv({ NEWSLETTER_IMAGES_REQUIRE: "store", ...over });
    const d = await create(e, `![Logo](${IMG}/x.png)`);
    const refused = await schedule(e, d.id);
    assert.deepEqual([refused.status, refused.body.error], [503, "image_rules_unusable"]);
    assert.match(refused.body.detail, detail);
    assert.equal(row(e, d.id).status, "draft");
    assert.match(settingsOf(e as never).imageRules.problem!, detail);
    const p = configProblems(e as never, true).find((x) => x.code === "image_rules_unusable");
    assert.ok(p && p.text.en.endsWith(refused.body.detail) && p.text.pl.includes("NEWSLETTER_IMAGES_REQUIRE"));
  }
});

test("load: probe failures and network errors refuse; images beyond the bound fail not_probed", async () => {
  const bucket = fakeR2();
  await bucket.put(KEY, new Uint8Array([1]), { httpMetadata: { contentType: "image/png" } });
  const env = mailEnv({ NEWSLETTER_IMAGES_REQUIRE: "load", NEWSLETTER_ASSETS: bucket });
  const c = await create(env, `![a](${OWN}) ![b](${IMG}/ok.png) ![c](${IMG}/403.png) ![d](${IMG}/down.png)`);
  const n = net({ [`${IMG}/403.png`]: { status: 403, type: "text/html" }, [`${IMG}/down.png`]: new TypeError("network") });
  const r = await withNet(n, () => schedule(env, c.id));
  assert.equal(r.status, 409);
  assert.deepEqual(r.body.items.map((i: { url: string; probe: { error: string | null; status: number | null } }) => [i.url, i.probe.status, i.probe.error]), [
    [`${IMG}/403.png`, 403, null],
    [`${IMG}/down.png`, null, "fetch_failed"],
  ]);
  assert.ok(!n.calls.includes(OWN), "a stored image is judged from R2");
  // More distinct images than one check probes: the rest cannot be claimed to load.
  const urls = Array.from({ length: MAX_PROBED_IMAGES + 2 }, (_, i) => `${IMG}/${i}.png`);
  const many = await create(env, urls.map((u, i) => `![i${i}](${u})`).join("\n\n"));
  const m = net();
  const r2 = await withNet(m, () => schedule(env, many.id));
  assert.deepEqual(r2.body.items.map((i: { url: string; problems: string[]; probe: unknown }) => [i.url, i.problems, i.probe]), urls.slice(MAX_PROBED_IMAGES).map((u) => [u, ["not_probed"], null]));
  assert.equal(new Set(m.calls).size, MAX_PROBED_IMAGES);
  // All loading, within the bound: scheduled.
  const ok = await create(env, `![b](${IMG}/ok.png)`);
  assert.equal((await withNet(net(), () => schedule(env, ok.id))).status, 200);
});

test("rules: commas or spaces, any case, repeats; an unknown word fails closed", async () => {
  const on = { NEWSLETTER_SITE_URL: SITE, NEWSLETTER_ASSETS: fakeR2() };
  assert.deepEqual(imageRules({ ...on, NEWSLETTER_IMAGES_REQUIRE: " Load ,ALT  store,alt" } as never), { rules: ["alt", "store", "load"], problem: null });
  const bad = imageRules({ NEWSLETTER_IMAGES_REQUIRE: "alt, pictures, PICTURES,x" } as never);
  assert.deepEqual(bad.rules, ["alt"]);
  assert.equal(bad.problem, 'NEWSLETTER_IMAGES_REQUIRE: unknown rule "pictures", "x"; use alt, store or load, separated by commas');
  const env = mailEnv({ NEWSLETTER_IMAGES_REQUIRE: "alt;load" });
  const c = await create(env, "text");
  const r = await schedule(env, c.id);
  assert.deepEqual([r.status, r.body.error], [503, "image_rules_unusable"]);
  assert.match(r.body.detail, /unknown rule "alt;load"/);
  assert.equal(row(env, c.id).status, "draft");
  // A campaign that passes, with a mail that has no images at all.
  const fine = mailEnv({ NEWSLETTER_IMAGES_REQUIRE: "alt load" });
  const d = await create(fine, "just text");
  assert.equal((await withNet(net(), () => schedule(fine, d.id))).status, 200);
});

test("the check takes time: an edit saved meanwhile refuses with campaign_changed; two clicks, one schedule", async () => {
  const env = mailEnv({ NEWSLETTER_IMAGES_REQUIRE: "load" });
  const c = await create(env, `![a](${IMG}/a.png)`);
  let edited = false;
  const n = net({}, () => {
    if (edited) return;
    edited = true;
    env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET body = 'changed', updated_at = '2099-01-01T00:00:00.000Z' WHERE id = ?").run(c.id);
  });
  const r = await withNet(n, () => schedule(env, c.id));
  assert.deepEqual([r.status, r.body.error], [409, "campaign_changed"]);
  assert.equal(row(env, c.id).status, "draft");
  const results = await withNet(net(), () => Promise.all([schedule(env, c.id), schedule(env, c.id)]));
  assert.deepEqual(results.map((x) => [x.status, x.body.error ?? null]).sort(), [[200, null], [409, "invalid_transition"]]);
});
