// NEWSLETTER_TRACK_SKIP_HOSTS (#464) through the routes and the engine: the
// preview, the test send and a real send show the same URLs; a malformed
// value refuses scheduling like a content rule; the settings report it; the
// strict UTM rule never flags a link the worker leaves as written.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { configProblems } from "../src/module-summary.ts";
import { call, fakeFetch, mailEnv } from "./campaign-fixtures.ts";
import { addCampaign, campaignRow, engineEnv, fakeMailer, pump } from "./engine-fixtures.ts";
import { PASSWORD, fakeCtx, req } from "./fakes.ts";

type Env = ReturnType<typeof mailEnv>;
const ON = { NEWSLETTER_TRACK_CLICKS: "true", NEWSLETTER_TRACK_SKIP_HOSTS: "shop.example" };
const AFFILIATE = "https://www.shop.example/dp/B000?tag=x-21";
const BODY = `Buy [it](${AFFILIATE}) or read [the post](https://example.com/post).`;
const settings = async (env: Env) => (await (await handleFetch(req("/newsletter-admin/api/settings", { auth: PASSWORD }), env as never, fakeCtx() as never)).json()) as Record<string, any>;
const create = async (env: Env, body: Record<string, unknown> = {}) => (await call(env, "POST", "", { subject: "S", body: BODY, ...body })).body;

test("preview, test send and a real send carry the same URLs; only the counted link is stored", async () => {
  const env = mailEnv(ON);
  const { id } = await create(env);
  const preview = (await call(env, "POST", `/${id}/preview`, { language: "en" })).body;
  assert.ok(preview.html.includes(`href="${AFFILIATE}"`), preview.html);
  assert.ok(preview.text.includes(AFFILIATE));
  assert.match(preview.html, new RegExp(`href="https://example\\.com/newsletter-admin/hooks/c/${id}/[0-9a-f]{16}"`));
  const f = fakeFetch();
  try {
    assert.equal((await call(env, "POST", `/${id}/test`, { to: "me@example.com", language: "en" })).status, 200);
    assert.equal(f.calls[0].body.html, preview.html, "the test mail is the preview, byte for byte");
    assert.equal(f.calls[0].body.text, preview.text);
  } finally {
    f.restore();
  }
  assert.deepEqual(env.NEWSLETTER_DB.sqlite.prepare("SELECT url FROM click_links WHERE campaign_id = ?").all(id).map((r) => r.url), ["https://example.com/post"]);

  // The engine renders through the same path: a reader's mail is the preview with their token.
  const eng = await engineEnv(2, ON);
  const cid = addCampaign(eng.NEWSLETTER_DB, { body: BODY });
  const mail = fakeMailer();
  await pump(eng, { open: mail.open });
  assert.equal(campaignRow(eng.NEWSLETTER_DB, cid).status, "sent");
  for (const m of mail.sent) {
    assert.ok((m.html as string).includes(`href="${AFFILIATE}"`));
    assert.ok(m.text.includes(`it (${AFFILIATE})`));
    assert.equal(((m.html as string).match(/\/hooks\/c\//g) ?? []).length, 1);
  }
  assert.deepEqual(eng.NEWSLETTER_DB.sqlite.prepare("SELECT url FROM click_links").all().map((r) => r.url), ["https://example.com/post"]);
});

test("settings: clicks.skipHosts and clicks.skipHostsProblem, additive", async () => {
  const plain = await settings(mailEnv());
  assert.deepEqual(plain.clicks, { tracking: false, unsubscribeWindowDays: 14, skipHosts: [], skipHostsProblem: null });
  const set = await settings(mailEnv({ ...ON, NEWSLETTER_TRACK_SKIP_HOSTS: "Shop.example short.example" }));
  assert.deepEqual(set.clicks, { tracking: true, unsubscribeWindowDays: 14, skipHosts: ["shop.example", "short.example"], skipHostsProblem: null });
  assert.equal(set.contentRules.problem, null);
  const bad = await settings(mailEnv({ NEWSLETTER_TRACK_SKIP_HOSTS: "https://shop.example" }));
  assert.match(bad.clicks.skipHostsProblem, /^NEWSLETTER_TRACK_SKIP_HOSTS must list plain host names/);
  assert.equal(bad.contentRules.problem, bad.clicks.skipHostsProblem, "reported where the content rules report theirs");
  assert.equal(bad.contentRules.active, true);
});

test("a malformed value refuses scheduling and the content check (503); preview and test send still work", async () => {
  const env = mailEnv({ ...ON, NEWSLETTER_TRACK_SKIP_HOSTS: "shop.example, https://short.example/x" });
  const { id } = await create(env);
  const r = await call(env, "POST", `/${id}/schedule`, {});
  assert.equal(r.status, 503);
  assert.equal(r.body.error, "content_rules_invalid");
  assert.match(r.body.detail, /NEWSLETTER_TRACK_SKIP_HOSTS .* not "https:\/\/short\.example\/x"$/);
  assert.equal((env.NEWSLETTER_DB.sqlite.prepare("SELECT status FROM campaigns WHERE id = ?").get(id) as { status: string }).status, "draft");
  const check = await call(env, "GET", `/${id}/content-check`);
  assert.equal(check.status, 503);
  assert.equal(check.body.error, "content_rules_invalid");
  assert.deepEqual(configProblems(env as never, true).map((p) => p.code), ["content_rules_invalid"]);
  const preview = (await call(env, "POST", `/${id}/preview`, {})).body;
  assert.ok(preview.html.includes(`href="${AFFILIATE}"`), "the valid entries still apply");
  const f = fakeFetch();
  try {
    assert.equal((await call(env, "POST", `/${id}/test`, { to: "me@example.com" })).status, 200);
    assert.equal(f.calls[0].body.html, preview.html);
  } finally {
    f.restore();
  }
  // Fixed: scheduling goes on.
  const ok = mailEnv(ON);
  assert.equal((await call(ok, "POST", `/${(await create(ok)).id}/schedule`, {})).status, 200);
});

test("strict UTM: a hand-written utm_* on a skipped host is not a conflict, on a tagged one it still is", async () => {
  const strict = { NEWSLETTER_UTM: "on", NEWSLETTER_UTM_STRICT: "on", NEWSLETTER_UTM_HOSTS: "shop.example" };
  const body = "[a](https://shop.example/a?utm_source=own) [b](https://example.com/b?utm_source=own)";
  const hosts = async (env: Env) => {
    const { id } = await create(env, { body });
    return (await call(env, "GET", `/${id}/content-check`)).body.findings.map((f: { host: string }) => f.host);
  };
  assert.deepEqual(await hosts(mailEnv(strict)), ["shop.example", "example.com"]);
  assert.deepEqual(await hosts(mailEnv({ ...strict, NEWSLETTER_TRACK_SKIP_HOSTS: "shop.example" })), ["example.com"], "skipping wins over NEWSLETTER_UTM_HOSTS");
  const env = mailEnv({ ...strict, NEWSLETTER_TRACK_SKIP_HOSTS: "shop.example example.com" });
  const { id } = await create(env, { body });
  assert.deepEqual((await call(env, "GET", `/${id}/content-check`)).body.findings, []);
  assert.equal((await call(env, "POST", `/${id}/schedule`, {})).status, 200);
});

test("panel: the Settings view lists the hosts, says none, or shows the problem as text", async () => {
  const { createI18n } = await import("../public/js/core/i18n.js");
  const { skipHostsFact } = await import("../public/js/views/settings.js");
  const strings = Object.fromEntries(await Promise.all(["en", "pl", "de", "fr", "hi"].map(async (l) => [l, (await import(`../public/js/strings/${l}.js`)).default])));
  const texts = (n: unknown): string[] => {
    if (n == null) return [];
    if (typeof n === "string") return [n];
    if (Array.isArray(n)) return n.flatMap(texts);
    return texts((n as { children: unknown[] }).children);
  };
  for (const key of ["set.skipHosts", "set.skipHosts.none", "set.skipHosts.broken"]) for (const l of Object.keys(strings)) assert.ok(Object.hasOwn(strings[l], key), `${l} ${key}`);
  const app = { t: createI18n("en", strings, {}).t };
  assert.deepEqual(texts(skipHostsFact(app, { skipHosts: ["shop.example", "short.example"], skipHostsProblem: null })), ["shop.example, short.example"]);
  assert.deepEqual(texts(skipHostsFact(app, { skipHosts: [], skipHostsProblem: null })), ["None: every link is counted and tagged as configured"]);
  assert.deepEqual(texts(skipHostsFact(app, {})), ["None: every link is counted and tagged as configured"], "an older worker without the fields");
  const broken = skipHostsFact(app, { skipHosts: [], skipHostsProblem: '<b>"x"</b>' });
  assert.deepEqual(texts(broken), ['Cannot be applied, so no campaign can be scheduled: <b>"x"</b>'], "the problem is text, never markup");
  assert.equal(broken.props.class, "flag off bad");
});
