// #463: NEWSLETTER_HOOK_FROM_DOMAINS read and reported — the parser, the
// sender-domain reader, GET /api/settings and the panel's Settings line.
import { test } from "node:test";
import assert from "node:assert/strict";
import { fromUs, hookOrigin, originSettings, senderDomain } from "../src/hook-origin.ts";
import { find, text } from "../public/js/core/vnode.js";
import { createI18n } from "../public/js/core/i18n.js";
import { createFormat } from "../public/js/core/format.js";
import en from "../public/js/strings/en.js";
import pl from "../public/js/strings/pl.js";
import { hookFromFact } from "../public/js/views/webhook-activity.js";
import { hookEnv } from "./hook-fixtures.ts";
import { api } from "./subscriber-seed.ts";

const origin = (v: string | undefined) => hookOrigin({ NEWSLETTER_HOOK_FROM_DOMAINS: v } as never);

test("hookOrigin: unset, empty or only separators is no list", () => {
  for (const v of [undefined, "", "  ", " , ,\n"]) assert.deepEqual(origin(v), { domains: [], problem: null }, JSON.stringify(v));
});

test("hookOrigin: commas and/or whitespace, any case, repeats once, order kept", () => {
  assert.deepEqual(origin(" News.Example.COM, example.org\texample.net,,news.example.com "), {
    domains: ["news.example.com", "example.org", "example.net"],
    problem: null,
  });
  assert.deepEqual(origin("mail-1.example.co.uk").domains, ["mail-1.example.co.uk"]);
});

test("hookOrigin: anything but a plain domain is a problem naming the entries", () => {
  const bad = ["news@example.com", "https://example.com", "example.com/news", "example.com:25", "localhost", "exa_mple.com", "a..example.com", ".example.com", "example.com.", "exämple.com"];
  for (const v of bad) {
    const o = origin(`example.org, ${v}`);
    assert.ok(o.problem?.startsWith("NEWSLETTER_HOOK_FROM_DOMAINS: "), v);
    assert.ok(o.problem?.includes(JSON.stringify(v.toLowerCase())), v);
    assert.ok(o.domains.includes("example.org"), "the list is still reported as read");
  }
  const two = origin("a@example.com x").problem as string;
  assert.match(two, /"a@example\.com", "x" is not a plain domain/);
  assert.ok(origin("x".repeat(60)).problem?.includes(`"${"x".repeat(40)}"`), "quoted entries are cut to 40 characters");
});

test("senderDomain: bare address, display name, case, spacing; null without an address", () => {
  assert.equal(senderDomain("news@example.com"), "example.com");
  assert.equal(senderDomain("Example Name <Newsletter@Example.COM>"), "example.com");
  assert.equal(senderDomain('"A @ B" <news@News.Example.com >  '), "news.example.com");
  assert.equal(senderDomain(" news@example.com "), "example.com");
  for (const v of [undefined, null, 42, ["a@example.com"], "", "Example Name", "news@", "news@ >"]) assert.equal(senderDomain(v), null, JSON.stringify(v));
});

test("fromUs: no list lets everything through; a list needs an exact domain match", () => {
  const none = origin(undefined);
  assert.equal(fromUs(none, undefined), true);
  assert.equal(fromUs(none, "a@example.org"), true);
  const list = origin("example.com, news.example.org");
  assert.equal(fromUs(list, "Example <A@EXAMPLE.com>"), true);
  assert.equal(fromUs(list, "a@news.example.org"), true);
  assert.equal(fromUs(list, "a@sub.example.com"), false, "no subdomain matching");
  assert.equal(fromUs(list, "a@example.org"), false, "no parent matching either");
  assert.equal(fromUs(list, "a@example.net"), false);
  assert.equal(fromUs(list, undefined), false, "no sender: not provably ours");
  assert.equal(fromUs(list, "no address"), false);
});

test("originSettings and GET /api/settings report the list and its problem", async () => {
  assert.deepEqual(originSettings({} as never), { fromDomains: [], fromDomainsProblem: null });
  const unset = await (await api(await hookEnv(), "GET", "/api/settings")).json();
  assert.deepEqual(unset.webhooks, { resend: true, mailchannels: true, fromDomains: [], fromDomainsProblem: null });
  const set = await (await api(await hookEnv({ NEWSLETTER_HOOK_FROM_DOMAINS: "Example.com news.example.org" }), "GET", "/api/settings")).json();
  assert.deepEqual(set.webhooks.fromDomains, ["example.com", "news.example.org"]);
  assert.equal(set.webhooks.fromDomainsProblem, null);
  const broken = await (await api(await hookEnv({ NEWSLETTER_HOOK_FROM_DOMAINS: "news@example.com" }), "GET", "/api/settings")).json();
  assert.match(broken.webhooks.fromDomainsProblem, /"news@example\.com" is not a plain domain/);
});

const fmt = createFormat("en-GB", "UTC");
const app = (lang = "en") => ({ t: createI18n(lang, { en, pl }, {}, fmt.number).t, fmt });
const cls = (n: unknown) => String((n as { props?: { class?: string } }).props?.class ?? "");

test("panel: the sender domain line shows the list, 'all', or the problem", () => {
  const list = hookFromFact(app(), { fromDomains: ["example.com", "news.example.org"], fromDomainsProblem: null });
  assert.equal(text(list), "example.com, news.example.org");
  assert.ok(find(list as never, (x) => x.tag === "code"));
  const all = hookFromFact(app(), { fromDomains: [], fromDomainsProblem: null });
  assert.match(text(all), /^All: events from every sending domain are applied$/);
  assert.equal(cls(all), "flag off");
  assert.match(text(hookFromFact(app("pl"), {})), /^Wszystkie/, "an older worker without the fields reads as unset");
  const broken = hookFromFact(app(), { fromDomains: ["x"], fromDomainsProblem: "NEWSLETTER_HOOK_FROM_DOMAINS: bad" });
  assert.equal(text(broken), "Cannot be applied, the webhooks answer 503: NEWSLETTER_HOOK_FROM_DOMAINS: bad");
  assert.equal(cls(broken), "flag off bad");
});
