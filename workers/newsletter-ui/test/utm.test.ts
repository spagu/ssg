// UTM tagging of links to the site (#443): the tagging itself, the single
// render path with click counting off and on, and what the settings report.
import { test } from "node:test";
import assert from "node:assert/strict";
import { linkId, rendererFor } from "../src/click-track.ts";
import { renderMessage } from "../src/render.ts";
import type { CampaignContent } from "../src/render.ts";
import { settingsOf } from "../src/settings.ts";
import { campaignSlug, tagUrl, utmFor, utmHosts, utmOn, utmSource } from "../src/utm.ts";
import { engineEnv } from "./engine-fixtures.ts";

const SITE = { url: "https://example.com", host: "example.com" };
const UTM = { NEWSLETTER_UTM: "on" };
const reader = (token: string, language: string | null = "en") => ({ email: `${token}@example.com`, language, token: token.padEnd(43, "x") });
const BODY = [
  "Read [the post](https://example.com/blog/{{language}}/#top), [the shop](https://shop.example.com/a?b=1)",
  "and [the tagged one](https://example.com/x?utm_source=own).",
  "",
  "Write to [us](mailto:news@example.com) or [leave]({{unsubscribe_url}}).",
  "",
  "![logo](https://example.com/logo.png)",
].join("\n");
const content = (over: Partial<CampaignContent> = {}) => ({
  id: "camp-1",
  subject: "Zażółć gęślą jaźń: Łódź & Co!",
  body: BODY,
  bodyFormat: "markdown" as const,
  textOverride: null,
  ...over,
});
const SLUG = "zazolc-gesla-jazn-lodz-co";

test("utm settings: off by default, source and hosts from the variables", () => {
  for (const off of [{}, { NEWSLETTER_UTM: "" }, { NEWSLETTER_UTM: "off" }, { NEWSLETTER_UTM: "0" }]) {
    assert.equal(utmOn(off as never), false);
    assert.equal(utmFor(off as never, SITE, content()), null);
  }
  for (const on of ["1", "true", "YES", " on "]) assert.equal(utmOn({ NEWSLETTER_UTM: on } as never), true, on);
  assert.equal(utmSource({} as never), "newsletter");
  assert.equal(utmSource({ NEWSLETTER_UTM_SOURCE: " weekly " } as never), "weekly");
  assert.deepEqual([...utmHosts({} as never, SITE)], ["example.com"]);
  assert.deepEqual([...utmHosts({ NEWSLETTER_UTM_HOSTS: " WWW.example.com, shop.example.com  example.com " } as never, SITE)], [
    "example.com",
    "www.example.com",
    "shop.example.com",
  ]);
  assert.deepEqual(utmFor({ ...UTM, NEWSLETTER_UTM_SOURCE: "weekly" } as never, SITE, content()), {
    source: "weekly",
    campaign: SLUG,
    hosts: new Set(["example.com"]),
  });
});

test("campaignSlug: ASCII lower case, accents dropped, capped, the fallback when nothing is left", () => {
  assert.equal(campaignSlug("Zażółć gęślą jaźń: Łódź & Co!", "id"), SLUG);
  assert.equal(campaignSlug("  Straße №5 — été  ", "id"), "strasse-no5-ete");
  assert.equal(campaignSlug("नमस्ते", "camp-9"), "camp-9");
  assert.equal(campaignSlug("", "camp-9"), "camp-9");
  const long = campaignSlug("ab ".repeat(40), "id");
  assert.equal(long.length, 59, "cut at 60, the dash left at the end removed");
  assert.match(long, /^ab(-ab)+$/);
});

test("tagUrl: own hosts only, existing utm_* kept, the rest of the URL untouched", () => {
  const utm = { source: "news letter", campaign: "may", hosts: new Set(["example.com"]) };
  const tail = "utm_source=news%20letter&utm_medium=email&utm_campaign=may&utm_content=abc";
  assert.equal(tagUrl("https://example.com/a", utm, "abc"), `https://example.com/a?${tail}`);
  assert.equal(tagUrl("https://EXAMPLE.com/a?b=1&c=%20x#frag?y", utm, "abc"), `https://EXAMPLE.com/a?b=1&c=%20x&${tail}#frag?y`);
  assert.equal(tagUrl("https://example.com/a?", utm, "abc"), `https://example.com/a?${tail}`);
  assert.equal(tagUrl("https://example.com/a?b=1&", utm, "abc"), `https://example.com/a?b=1&${tail}`);
  assert.equal(tagUrl("https://example.com/a#f", utm, "abc"), `https://example.com/a?${tail}#f`);
  assert.equal(tagUrl("https://example.com/a?UTM_Term=x", utm, "abc"), null, "its own utm_* wins");
  assert.equal(tagUrl("https://www.example.com/a", utm, "abc"), null, "another host");
  assert.equal(tagUrl("https://example.com.evil.example/a", utm, "abc"), null);
  assert.equal(tagUrl("http://", utm, "abc"), null, "not a URL");
});

test("utm off: rendering is byte for byte what it was", async () => {
  const c = content();
  const before = renderMessage(c, reader("a"), SITE);
  const env = await engineEnv(0, { NEWSLETTER_UTM: "off", NEWSLETTER_UTM_SOURCE: "x", NEWSLETTER_UTM_HOSTS: "shop.example.com" });
  assert.deepEqual((await rendererFor(env as never, c, SITE, [reader("a")], true))(reader("a")), before);
});

test("utm on, click counting off: own-host links tagged in both parts, everything else as written", async () => {
  const env = await engineEnv(0, UTM);
  const c = content();
  const people = [reader("a"), reader("b"), reader("c", "pl")];
  const render = await rendererFor(env as never, c, SITE, people, true);
  const [a, b, pl] = people.map((p) => render(p));
  const id = await linkId("https://example.com/blog/en/#top");
  const tagged = `https://example.com/blog/en/?utm_source=newsletter&utm_medium=email&utm_campaign=${SLUG}&utm_content=${id}#top`;
  assert.ok(a.html.includes(`href="${tagged.replaceAll("&", "&amp;")}"`), a.html);
  assert.ok(a.text.includes(tagged), "the text part carries the same link");
  assert.ok(a.html.includes('href="https://shop.example.com/a?b=1"'), "another host is not tagged");
  assert.ok(a.html.includes('href="https://example.com/x?utm_source=own"'), "a link with its own utm_* is kept");
  assert.ok(a.html.includes('href="mailto:news@example.com"'));
  assert.ok(a.html.includes('src="https://example.com/logo.png"'), "images are not links");
  const unsub = `https://example.com/api/newsletter/unsubscribe?token=${people[0].token}`;
  assert.ok(a.html.includes(`href="${unsub}"`), "unsubscribe link in the body untouched");
  assert.equal(a.headers["List-Unsubscribe"], `<${unsub}>`);
  assert.ok(a.text.endsWith(`: ${unsub}\n`), "footer untouched");
  assert.equal(a.html.match(/utm_medium=email/g)?.length, 1);
  assert.equal(b.html.replaceAll(people[1].token, people[0].token), a.html, "the same tagged links for every recipient");
  assert.ok(pl.html.includes(`utm_content=${await linkId("https://example.com/blog/pl/#top")}#top`), "the id follows the link as written");
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM click_links").get()?.n, 0, "nothing stored without click counting");
  // text_override: bare URLs tagged, the unsubscribe one not; more hosts on request.
  const more = await engineEnv(0, { ...UTM, NEWSLETTER_UTM_HOSTS: "shop.example.com", NEWSLETTER_UTM_SOURCE: "weekly" });
  const o = (await rendererFor(more as never, content({ subject: "नमस्ते", textOverride: "Go https://shop.example.com/a. Leave {{unsubscribe_url}}" }), SITE, people, false))(people[0]);
  const shopId = await linkId("https://shop.example.com/a");
  assert.ok(o.text.startsWith(`Go https://shop.example.com/a?utm_source=weekly&utm_medium=email&utm_campaign=camp-1&utm_content=${shopId}. Leave ${unsub}\n`), o.text);
});

test("utm on, click counting on: the stored redirect target is tagged, the id is the link's as written", async () => {
  const env = await engineEnv(0, { ...UTM, NEWSLETTER_TRACK_CLICKS: "true" });
  const c = content({ body: "[post](https://example.com/p) [shop](https://shop.example.com/a)" });
  const m = (await rendererFor(env as never, c, SITE, [reader("a")], true))(reader("a"));
  const post = await linkId("https://example.com/p");
  const shop = await linkId("https://shop.example.com/a");
  assert.ok(m.html.includes(`href="https://example.com/newsletter-admin/hooks/c/camp-1/${post}"`));
  assert.ok(m.html.includes(`href="https://example.com/newsletter-admin/hooks/c/camp-1/${shop}"`));
  assert.doesNotMatch(m.html, /utm_/, "the mail carries the counting links only");
  const stored = env.NEWSLETTER_DB.sqlite.prepare("SELECT link_id, url FROM click_links WHERE campaign_id = 'camp-1' ORDER BY url").all();
  assert.deepEqual(stored.map((r) => ({ ...r })), [
    { link_id: post, url: `https://example.com/p?utm_source=newsletter&utm_medium=email&utm_campaign=${SLUG}&utm_content=${post}` },
    { link_id: shop, url: "https://shop.example.com/a" },
  ]);
  // A preview does not store anything.
  const fresh = await engineEnv(0, { ...UTM, NEWSLETTER_TRACK_CLICKS: "true" });
  await rendererFor(fresh as never, c, SITE, [reader("a")], false);
  assert.equal(fresh.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM click_links").get()?.n, 0);
});

test("settings report the tagging, with no hosts while the site URL is unusable", () => {
  assert.deepEqual(settingsOf({} as never).utm, { enabled: false, source: "newsletter", hosts: [] });
  const s = settingsOf({ ...UTM, NEWSLETTER_SITE_URL: "https://example.com", NEWSLETTER_UTM_HOSTS: "www.example.com" } as never);
  assert.deepEqual(s.utm, { enabled: true, source: "newsletter", hosts: ["example.com", "www.example.com"] });
});
