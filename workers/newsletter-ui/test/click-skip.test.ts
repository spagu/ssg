// Links sent exactly as written (#464): NEWSLETTER_TRACK_SKIP_HOSTS, its
// parsing and matching, and the single render path with click counting and
// UTM tagging in every combination.
import { test } from "node:test";
import assert from "node:assert/strict";
import { skipHostsConfig, skipHostsOf, skipsLink } from "../src/click-skip.ts";
import { MAX_LINKS, linkId, rendererFor } from "../src/click-track.ts";
import type { CampaignContent } from "../src/render.ts";
import { engineEnv } from "./engine-fixtures.ts";

const SITE = { url: "https://example.com", host: "example.com" };
const SKIP = { NEWSLETTER_TRACK_SKIP_HOSTS: "shop.example" };
const reader = (token: string, language: string | null = "en") => ({ email: `${token}@example.com`, language, token: token.padEnd(43, "x") });
const AFFILIATE = "https://www.shop.example/dp/B000?tag=x-21";
const BODY = `Buy [it](${AFFILIATE}), read [the post](https://example.com/post) or [elsewhere](https://notshop.example/a).`;
const content = (over: Partial<CampaignContent> = {}) => ({ id: "camp-1", subject: "Hi", body: BODY, bodyFormat: "markdown" as const, textOverride: null, ...over });
const CLICK = /https:\/\/example\.com\/newsletter-admin\/hooks\/c\/camp-1\/[0-9a-f]{16}/g;
const rows = (env: Awaited<ReturnType<typeof engineEnv>>) =>
  env.NEWSLETTER_DB.sqlite.prepare("SELECT link_id, url FROM click_links ORDER BY url").all().map((r) => ({ ...r }));
const COMBOS = [{}, { NEWSLETTER_TRACK_CLICKS: "true" }, { NEWSLETTER_UTM: "on" }, { NEWSLETTER_TRACK_CLICKS: "true", NEWSLETTER_UTM: "on" }];

test("config: hosts split on commas and spaces, lower case, distinct; malformed entries are a problem", () => {
  for (const raw of [undefined, "", " , ,\n "]) assert.deepEqual(skipHostsConfig({ NEWSLETTER_TRACK_SKIP_HOSTS: raw } as never), { hosts: [], problem: null });
  assert.deepEqual(skipHostsConfig({ NEWSLETTER_TRACK_SKIP_HOSTS: " SHOP.example, short.example  shop.example,xn--bcher-kva.example " } as never), {
    hosts: ["shop.example", "short.example", "xn--bcher-kva.example"],
    problem: null,
  });
  const bad = ["https://shop.example", "shop.example/x", "me@shop.example", "shop.example:8080", "bücher.example", "localhost", ".shop.example", "a..example"];
  const { hosts, problem } = skipHostsConfig({ NEWSLETTER_TRACK_SKIP_HOSTS: ["ok.example", ...bad].join(" ") } as never);
  assert.deepEqual(hosts, ["ok.example"], "the valid entries are kept");
  assert.match(problem ?? "", /^NEWSLETTER_TRACK_SKIP_HOSTS must list plain host names .*xn-- form\), not "https:\/\/shop\.example", "shop\.example\/x", /);
  assert.match(problem ?? "", /"shop\.example:8080", "bücher\.example", and 3 more$/, "five quoted, the rest counted");
  assert.equal(skipHostsConfig({ NEWSLETTER_TRACK_SKIP_HOSTS: "x".repeat(60) } as never).problem?.includes(`"${"x".repeat(40)}"`), true, "a long entry is cut");
  assert.deepEqual(skipHostsOf({ NEWSLETTER_TRACK_SKIP_HOSTS: "Shop.example" } as never), { skipHosts: ["shop.example"], skipHostsProblem: null });
  assert.deepEqual(skipHostsOf({} as never), { skipHosts: [], skipHostsProblem: null });
});

test("matching: the host and its subdomains at a label boundary, any case, any port", () => {
  const hosts = ["shop.example"];
  for (const yes of ["https://shop.example/", "https://www.shop.example/a", "HTTPS://A.B.SHOP.EXAMPLE/x", "https://shop.example:8443/p", "http://user@shop.example/"])
    assert.equal(skipsLink(hosts, yes), true, yes);
  for (const no of ["https://notshop.example/", "https://shop.example.com/", "https://example.com/?u=shop.example", "not a url", "http://"])
    assert.equal(skipsLink(hosts, no), false, no);
  assert.equal(skipsLink([], "https://shop.example/"), false);
  assert.equal(skipsLink(skipHostsConfig({ NEWSLETTER_TRACK_SKIP_HOSTS: "xn--bcher-kva.example" } as never).hosts, "https://www.bücher.example/"), true, "an IDN link matches its punycode entry");
});

test("unset, empty or only separators: byte for byte today's mail and rows, in every combination", async () => {
  for (const combo of COMBOS) {
    const base = await engineEnv(0, combo);
    const before = (await rendererFor(base as never, content(), SITE, [reader("a")], true))(reader("a"));
    for (const raw of ["", " ,  , "]) {
      const env = await engineEnv(0, { ...combo, NEWSLETTER_TRACK_SKIP_HOSTS: raw });
      assert.deepEqual((await rendererFor(env as never, content(), SITE, [reader("a")], true))(reader("a")), before, JSON.stringify(combo));
      assert.deepEqual(rows(env), rows(base));
    }
  }
});

test("click counting on: the skipped link goes out verbatim in both parts, unstored; the site link is counted", async () => {
  for (const utm of [{}, { NEWSLETTER_UTM: "on" }]) {
    const env = await engineEnv(0, { NEWSLETTER_TRACK_CLICKS: "true", ...SKIP, ...utm });
    const m = (await rendererFor(env as never, content(), SITE, [reader("a")], true))(reader("a"));
    assert.ok(m.html.includes(`href="${AFFILIATE}"`), m.html);
    assert.ok(m.text.includes(`it (${AFFILIATE})`), m.text);
    assert.equal(m.html.match(CLICK)?.length, 2, "the post and notshop.example are still counted");
    assert.equal(m.text.match(CLICK)?.length, 2);
    const post = await linkId("https://example.com/post");
    const stored = rows(env).map((r) => r.url);
    assert.equal(stored.length, 2);
    assert.ok(!stored.some((u) => String(u).includes("shop.example/dp")), "no click_links row for the skipped link");
    assert.ok(m.html.includes(`/hooks/c/camp-1/${post}"`));
    assert.equal(rows(env).find((r) => r.link_id === post)?.url, "NEWSLETTER_UTM" in utm ? `https://example.com/post?utm_source=newsletter&utm_medium=email&utm_campaign=hi&utm_content=${post}` : "https://example.com/post");
  }
  // A hand-written text version: its bare skipped URL is kept too, punctuation and all.
  const env = await engineEnv(0, { NEWSLETTER_TRACK_CLICKS: "true", ...SKIP });
  const o = (await rendererFor(env as never, content({ textOverride: `Buy ${AFFILIATE}. Read https://example.com/post.` }), SITE, [reader("a")], false))(reader("a"));
  assert.match(o.text, new RegExp(`^Buy ${AFFILIATE.replace(/[?.]/g, "\\$&")}\\. Read https://example\\.com/newsletter-admin/hooks/c/camp-1/[0-9a-f]{16}\\.\\n`));
});

test("click counting off, UTM on: skipping wins over the site's host and NEWSLETTER_UTM_HOSTS", async () => {
  const body = "[a](https://example.com/a) [b](https://www.example.com/b) [c](https://shop.example/c) [d](https://other.example/d)";
  const tagged = (u: string, id: string) => `${u}?utm_source=newsletter&utm_medium=email&utm_campaign=hi&utm_content=${id}`;
  const plain = await engineEnv(0, { NEWSLETTER_UTM: "on", NEWSLETTER_UTM_HOSTS: "www.example.com shop.example" });
  const a = (await rendererFor(plain as never, content({ body }), SITE, [reader("a")], true))(reader("a"));
  assert.equal(a.html.match(/utm_medium/g)?.length, 3, "without skipping: the site, www and the shop are tagged");
  const env = await engineEnv(0, { NEWSLETTER_UTM: "on", NEWSLETTER_UTM_HOSTS: "www.example.com shop.example", NEWSLETTER_TRACK_SKIP_HOSTS: "shop.example,example.com" });
  const m = (await rendererFor(env as never, content({ body }), SITE, [reader("a")], true))(reader("a"));
  for (const u of ["https://example.com/a", "https://www.example.com/b", "https://shop.example/c", "https://other.example/d"]) {
    assert.ok(m.html.includes(`href="${u}"`), u);
    assert.ok(m.text.includes(`(${u})`), u);
  }
  assert.doesNotMatch(m.html + m.text, /utm_/);
  assert.deepEqual(rows(env), [], "nothing stored without click counting");
  // Only the shop skipped: the site keeps its tags.
  const shop = await engineEnv(0, { NEWSLETTER_UTM: "on", ...SKIP });
  const s = (await rendererFor(shop as never, content({ body }), SITE, [reader("a")], false))(reader("a"));
  assert.ok(s.html.includes(`href="${tagged("https://example.com/a", await linkId("https://example.com/a")).replaceAll("&", "&amp;")}"`));
  assert.ok(s.html.includes('href="https://shop.example/c"'));
});

test("MAX_LINKS counts tracked links only: skipped links in the mix leave the cap to the rest", async () => {
  const env = await engineEnv(0, { NEWSLETTER_TRACK_CLICKS: "true", ...SKIP });
  const skipped = Array.from({ length: 30 }, (_, i) => `[s${i}](https://www.shop.example/p/${i})`);
  const tracked = Array.from({ length: MAX_LINKS + 2 }, (_, i) => `[l${i}](https://example.com/p/${i})`);
  const body = tracked.flatMap((l, i) => (i < skipped.length ? [skipped[i], l] : [l])).join(" ");
  const m = (await rendererFor(env as never, content({ body }), SITE, [reader("a")], true))(reader("a"));
  assert.equal(m.html.match(CLICK)?.length, MAX_LINKS, "a full cap of tracked links");
  assert.equal(rows(env).length, MAX_LINKS);
  for (let i = 0; i < skipped.length; i++) assert.ok(m.html.includes(`href="https://www.shop.example/p/${i}"`));
  assert.ok(m.html.includes(`href="https://example.com/p/${MAX_LINKS + 1}"`), "past the cap a tracked link is sent as written, as before");
});
