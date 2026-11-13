// Click counting (GO-129): the rewrite, the single render path, registration
// by the send engine and the test send, and the public redirect.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { guarded, rewriteText, rewriteTree } from "../src/click-rewrite.ts";
import { MAX_LINKS, linkId, registerLinks, rendererFor, trackClicks } from "../src/click-track.ts";
import { renderMessage } from "../src/render.ts";
import type { CampaignContent } from "../src/render.ts";
import { sanitizeHtml, serialize } from "../src/sanitize.ts";
import { addCampaign, campaignRow, engineEnv, fakeMailer, pump } from "./engine-fixtures.ts";
import type { EngineEnv } from "./engine-fixtures.ts";
import { call, fakeFetch, mailEnv } from "./campaign-fixtures.ts";
import { fakeCtx, req } from "./fakes.ts";

const SITE = { url: "https://example.com", host: "example.com" };
const ON = { NEWSLETTER_TRACK_CLICKS: "true" };
const reader = (token: string, language: string | null = "en") => ({ email: `${token}@example.com`, language, token: token.padEnd(43, "x") });
const BODY = [
  "Read [the post](https://example.com/blog/{{language}}/) and [the shop](https://shop.example.com/a?b=1&c=2).",
  "",
  "Write to [us](mailto:news@example.com) or [leave]({{unsubscribe_url}}).",
  "",
  "![logo](https://example.com/logo.png)",
].join("\n");
const content = (over: Partial<CampaignContent> = {}) => ({ id: "camp-1", subject: "Hi", body: BODY, bodyFormat: "markdown" as const, textOverride: null, ...over });
const CLICK = /https:\/\/example\.com\/newsletter-admin\/hooks\/c\/camp-1\/[0-9a-f]{16}/g;

test("rewrite helpers: http(s) only, unsubscribe never, punctuation kept in text", () => {
  const seen: string[] = [];
  const g = guarded((u) => (seen.push(u), `R(${u})`), "https://example.com/api/newsletter/unsubscribe");
  assert.equal(g("mailto:a@example.com"), null);
  assert.equal(g("https://example.com/api/newsletter/unsubscribe?token=abc"), null);
  assert.equal(g("HTTP://example.com/x"), "R(HTTP://example.com/x)");
  assert.deepEqual(seen, ["HTTP://example.com/x"]);
  const tree = sanitizeHtml('<p><a title="t" href="https://example.com/a">a</a> <img src="https://example.com/i.png"> <a href="mailto:x@example.com">m</a></p>');
  assert.equal(
    serialize(rewriteTree(tree, g)),
    '<p><a title="t" href="R(https://example.com/a)">a</a> <img src="https://example.com/i.png"> <a href="mailto:x@example.com">m</a></p>',
  );
  assert.equal(serialize(tree), '<p><a title="t" href="https://example.com/a">a</a> <img src="https://example.com/i.png"> <a href="mailto:x@example.com">m</a></p>', "the input tree is not mutated");
  assert.equal(rewriteText("See https://example.com/a. Or (http://example.com/b)! mailto:x@example.com", g), "See R(https://example.com/a). Or (R(http://example.com/b))! mailto:x@example.com");
  assert.equal(rewriteText("https://example.com/kept", () => null), "https://example.com/kept");
});

test("tracking off: rendering is byte for byte what it was before GO-129", async () => {
  const c = content();
  const before = renderMessage(c, reader("a"), SITE);
  assert.deepEqual(renderMessage(c, reader("a"), SITE, {}), before);
  assert.deepEqual(renderMessage(c, reader("a"), SITE, { links: () => null }), before, "a rewriter that declines changes nothing");
  for (const off of [{}, { NEWSLETTER_TRACK_CLICKS: "" }, { NEWSLETTER_TRACK_CLICKS: "false" }, { NEWSLETTER_TRACK_CLICKS: "0" }]) {
    const env = await engineEnv(0, off);
    assert.equal(trackClicks(env as never), false);
    assert.deepEqual((await rendererFor(env as never, c, SITE, [reader("a")], true))(reader("a")), before);
    assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM click_links").get()?.n, 0, "nothing registered");
  }
  for (const on of ["1", "true", "YES", " on "]) assert.equal(trackClicks({ NEWSLETTER_TRACK_CLICKS: on } as never), true, on);
  // An invalid base path: the redirect could not be reached, so nothing is rewritten.
  const bad = await engineEnv(0, { ...ON, NEWSLETTER_UI_BASE: "/a b" });
  assert.deepEqual((await rendererFor(bad as never, c, SITE, [reader("a")], true))(reader("a")), before);
});

test("tracking on: every http(s) link rewritten except unsubscribe and mailto, ids stable across recipients", async () => {
  const env = await engineEnv(0, ON);
  const c = content();
  const people = [reader("a"), reader("b"), reader("c", "pl")];
  const render = await rendererFor(env as never, c, SITE, people, true);
  const [a, b, pl] = people.map((p) => render(p));
  for (const [m, p] of [[a, people[0]], [pl, people[2]]] as const) {
    const unsub = `https://example.com/api/newsletter/unsubscribe?token=${p.token}`;
    assert.ok(m.html.includes(`href="${unsub}"`), "unsubscribe link in the body untouched");
    assert.equal(m.headers["List-Unsubscribe"], `<${unsub}>`);
    assert.ok(m.text.endsWith(`: ${unsub}\n`), "footer untouched");
    assert.ok(m.html.includes('href="mailto:news@example.com"'));
    assert.ok(m.html.includes('src="https://example.com/logo.png"'), "images are not links");
    assert.doesNotMatch(m.html, /href="https:\/\/(shop\.)?example\.com\/(blog\/|a\?)/, "no original target left in a link");
    assert.equal(m.html.match(CLICK)?.length, 2);
    assert.equal(m.text.match(CLICK)?.length, 2, "the text part carries the same links");
  }
  assert.deepEqual(a.html.match(CLICK), b.html.match(CLICK), "same links, same ids for every recipient");
  const stored = env.NEWSLETTER_DB.sqlite.prepare("SELECT link_id, url, clicks FROM click_links WHERE campaign_id = 'camp-1' ORDER BY url").all();
  assert.deepEqual(stored.map((r) => r.url), ["https://example.com/blog/en/", "https://example.com/blog/pl/", "https://shop.example.com/a?b=1&c=2"]);
  for (const r of stored) assert.equal(r.link_id, await linkId(r.url as string));
  assert.notDeepEqual(a.html.match(CLICK), pl.html.match(CLICK), "a link with {{language}} in it differs by language");
  // Registering again adds nothing; an empty campaign registers nothing.
  await rendererFor(env as never, c, SITE, people, true);
  assert.equal(stored.length, env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM click_links").get()?.n);
  await registerLinks(env as never, "camp-1", new Map());
  // text_override: bare URLs rewritten, the unsubscribe one not.
  const o = (await rendererFor(env as never, content({ textOverride: "Go http://example.com/x. Leave {{unsubscribe_url}}" }), SITE, people, false))(people[0]);
  assert.match(o.text, /^Go https:\/\/example\.com\/newsletter-admin\/hooks\/c\/camp-1\/[0-9a-f]{16}\. Leave https:\/\/example\.com\/api\/newsletter\/unsubscribe\?token=ax+\n/);
});

test("tracking on: a content error is thrown by the render, not by the preparation; links are capped", async () => {
  const env = await engineEnv(0, ON);
  const broken = await rendererFor(env as never, content({ body: "{{nope}} https://example.com/" }), SITE, [reader("a")], true);
  assert.throws(() => broken(reader("a")), /unknown variable/);
  const many = Array.from({ length: MAX_LINKS + 5 }, (_, i) => `[l${i}](https://example.com/p/${i})`).join(" ");
  const m = (await rendererFor(env as never, content({ body: many }), SITE, [reader("a")], true))(reader("a"));
  assert.equal(m.html.match(CLICK)?.length, MAX_LINKS);
  assert.ok(m.html.includes(`href="https://example.com/p/${MAX_LINKS + 4}"`), "past the cap a link is sent as written");
});

// ---- through the engine and the redirect --------------------------------

const hit = (env: EngineEnv, path: string, method = "GET", init: { auth?: string } = {}) =>
  handleFetch(req("/newsletter-admin" + path, { method, ...init }), env as never, fakeCtx() as never);
const clicks = (env: EngineEnv) =>
  Object.fromEntries(env.NEWSLETTER_DB.sqlite.prepare("SELECT url, clicks FROM click_links").all().map((r) => [r.url, r.clicks]));

test("engine: every recipient gets the same tracked links; the redirect counts GETs, not HEADs, without auth", async () => {
  const env = await engineEnv(3, ON);
  const id = addCampaign(env.NEWSLETTER_DB, { body: "[A](https://example.com/a) [B](https://example.com/b) [out]({{unsubscribe_url}})" });
  const mail = fakeMailer();
  await pump(env, { open: mail.open });
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
  assert.equal(mail.sent.length, 3);
  const re = new RegExp(`https://example\\.com/newsletter-admin/hooks/c/${id}/[0-9a-f]{16}`, "g");
  const links = mail.sent.map((m) => (m.html as string).match(re));
  assert.equal(links[0]?.length, 2);
  assert.ok(links.every((l) => JSON.stringify(l) === JSON.stringify(links[0])), "identical for every recipient");
  for (const m of mail.sent) {
    assert.ok((m.html as string).includes(`/api/newsletter/unsubscribe?token=tok-${m.to.slice(0, 9)}`), "own unsubscribe link kept");
    // Every click URL, in both parts, is exactly {base}/hooks/c/{campaign}/{16 hex}: nothing about the person.
    const all = ((m.html as string) + m.text).match(/[^\s"()]*\/hooks\/[^\s"()]*/g) ?? [];
    assert.equal(all.length, 4);
    assert.ok(all.every((u) => new RegExp(`^https://example\\.com/newsletter-admin/hooks/c/${id}/[0-9a-f]{16}$`).test(u)), all.join(" "));
  }
  const path = new URL((links[0] as string[])[0]).pathname.slice("/newsletter-admin".length);

  const r = await hit(env, path); // no credentials at all
  assert.equal(r.status, 302);
  assert.equal(r.headers.get("location"), "https://example.com/a");
  assert.equal(r.headers.get("cache-control"), "no-store", "never cached: every click reaches the counter");
  assert.equal(r.headers.get("referrer-policy"), "no-referrer");
  assert.deepEqual(clicks(env), { "https://example.com/a": 1, "https://example.com/b": 0 });
  const head = await hit(env, path, "HEAD");
  assert.equal(head.status, 302);
  assert.equal(head.headers.get("location"), "https://example.com/a");
  assert.deepEqual(clicks(env), { "https://example.com/a": 1, "https://example.com/b": 0 }, "HEAD does not count");
  await hit(env, path);
  assert.equal(clicks(env)["https://example.com/a"], 2, "one increment per GET");
  assert.equal((await hit(env, path, "POST")).status, 405);
});

test("redirect: unknown ids, another campaign's link and a non-http target are 404", async () => {
  const env = await engineEnv(0, ON);
  const ins = env.NEWSLETTER_DB.sqlite.prepare("INSERT INTO click_links (campaign_id, link_id, url, clicks, created_at) VALUES (?, ?, ?, 0, '2026-01-01')");
  ins.run("c1", "aaaaaaaaaaaaaaaa", "https://example.com/one");
  ins.run("c2", "bbbbbbbbbbbbbbbb", "https://example.com/two");
  ins.run("c1", "cccccccccccccccc", "javascript:alert(1)"); // never written by the worker
  for (const p of ["/hooks/c/c1/ffffffffffffffff", "/hooks/c/nope/aaaaaaaaaaaaaaaa", "/hooks/c/c1/bbbbbbbbbbbbbbbb", "/hooks/c/c1/cccccccccccccccc", "/hooks/c/c1/https%3A%2F%2Fexample.org"]) {
    for (const method of ["GET", "HEAD"]) {
      const r = await hit(env, p, method);
      assert.equal(r.status, 404, `${method} ${p}`);
      assert.equal(r.headers.get("location"), null);
    }
  }
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT SUM(clicks) AS n FROM click_links").get()?.n, 0, "nothing counted");
  assert.equal((await hit(env, "/hooks/c/c1/aaaaaaaaaaaaaaaa?to=https://example.org")).headers.get("location"), "https://example.com/one", "the query cannot choose the target");
});

test("preview and test send show the tracked links; the test send registers them; delete drops them", async () => {
  const env = mailEnv(ON);
  const created = await call(env, "POST", "", { subject: "S", body: "[go](https://example.com/go)" });
  const id = created.body.id;
  const preview = (await call(env, "POST", `/${id}/preview`, { language: "en" })).body;
  assert.match(preview.html, new RegExp(`href="https://example\\.com/newsletter-admin/hooks/c/${id}/${await linkId("https://example.com/go")}"`));
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM click_links").get()?.n, 0, "a preview stores nothing");
  const f = fakeFetch();
  try {
    assert.equal((await call(env, "POST", `/${id}/test`, { to: "me@example.com", language: "en" })).status, 200);
    assert.equal(f.calls[0].body.html, preview.html, "the test mail is the preview, byte for byte");
    assert.equal(f.calls[0].body.text, preview.text);
  } finally {
    f.restore();
  }
  assert.deepEqual(env.NEWSLETTER_DB.sqlite.prepare("SELECT url FROM click_links WHERE campaign_id = ?").all(id).map((r) => r.url), ["https://example.com/go"]);
  assert.equal((await call(env, "DELETE", `/${id}`)).status, 204);
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM click_links").get()?.n, 0);
});
