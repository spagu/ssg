// GO-128: what the panel decides without a DOM — the dashboard's warnings
// and charts, the campaign rules, the import mapping and the list helpers.
import { test } from "node:test";
import assert from "node:assert/strict";
import { find, findAll, text } from "../public/js/core/vnode.js";
import { createI18n } from "../public/js/core/i18n.js";
import { createFormat } from "../public/js/core/format.js";
import en from "../public/js/strings/en.js";
import {
  REASON_KEYS, campaignsFigure, growthFigure, languageFigure, languageName, statusFigure, suppressionFigure, warnings,
} from "../public/js/models/dashboard.js";
import {
  CONFIRM, actionsFor, clicksFigure, fieldsOf, fromLocalInput, hasStats, isLive, payloadFrom, resultsFigure, sameFields,
  segmentFrom, shortUrl, splitList, toLocalInput,
} from "../public/js/models/campaign.js";
import {
  cleanFilters, delimiterKey, duplicateColumn, headersOf, ignoredKey, importBody, liftNeedsConfirm, mappingFrom, needsConsent,
  readFileText, sendStatusKey, suppressionName, tagsFrom,
} from "../public/js/models/lists.js";

const fmt = createFormat("en-GB", "UTC");
const { t } = createI18n("en", { en }, {}, fmt.number);
const ctx = { t, fmt, lang: "en" };
const summary = (fig: unknown) => text(find(fig as never, (n) => n.props?.class === "chart-summary"));

const settings = (over: Record<string, unknown> = {}) => ({
  transport: "smtp", siteUrlValid: true, sender: { email: "news@example.com" }, engine: { queue: true }, webhooks: { resend: false, mailchannels: false }, ...over,
});
const engine = (over: Record<string, unknown> = {}) => ({ campaigns: [], deadLetters: { count: 0, lastAt: null }, ...over });

test("warnings: each problem once, most serious first, with a link where there is one", () => {
  assert.deepEqual(warnings({ settings: settings(), engine: engine() }), []);
  assert.deepEqual(warnings({ settings: null, engine: null }), []);
  const w = warnings({
    settings: settings({ transport: "resend", siteUrlValid: false, sender: { email: null }, engine: { queue: false } }),
    engine: engine({ deadLetters: { count: 2 }, campaigns: [{ id: "c1", status: "paused", error: "bounces: 6 of 10", subject: "S" }, { id: "c2", status: "sending", error: null }] }),
    campaigns: [{ id: "c3", subject: "Old", uncertain: 3 }, { id: "c4", uncertain: 0 }],
  });
  assert.deepEqual(w.map((x) => x.key), ["warn.noSiteUrl", "warn.noQueue", "warn.enginePaused", "warn.noSender", "warn.deadLetters", "warn.uncertain", "warn.noWebhook"]);
  assert.equal(w.find((x) => x.key === "warn.uncertain")?.href, "#/campaigns/c3?tab=uncertain");
  assert.equal(w.find((x) => x.key === "warn.deadLetters")?.href, null);
  assert.equal(warnings({ settings: settings({ transport: null }) })[0].key, "warn.noTransport");
  assert.deepEqual(warnings({ settings: settings({ transport: "mailchannels", webhooks: { mailchannels: true } }) }), []);
});

const series = (n: number, f: (i: number) => Record<string, number>) =>
  Array.from({ length: n }, (_, i) => ({ day: new Date(Date.parse("2026-10-15") + i * 86_400_000).toISOString().slice(0, 10), ...f(i) }));

test("growth chart: summary from the data, a quiet period said in words, table newest first", () => {
  const stats = { days: 30, series: series(30, (i) => ({ signups: i === 10 ? 7 : 1, unsubscribes: i % 10 === 0 ? 1 : 0 })) };
  const fig = growthFigure({ ...ctx, stats });
  assert.equal(summary(fig), "In the last 30 days: 36 sign-ups and 3 unsubscribes, a net change of 33. Busiest day: 25 October 2026, with 7 sign-ups.");
  assert.equal(findAll(fig, (n) => n.tag === "li").length, 2, "a legend for two series");
  assert.equal(text(find(fig, (n) => n.tag === "tbody").children[0].children[0]), "13 November 2026");
  const quiet = growthFigure({ ...ctx, stats: { days: 90, series: series(90, () => ({ signups: 0, unsubscribes: 0 })) } });
  assert.equal(summary(quiet), "No sign-ups and no unsubscribes in the last 90 days.");
  assert.ok(find(quiet, (n) => n.tag === "svg"), "an all-zero series still draws its axis");
  const onlyLeft = growthFigure({ ...ctx, stats: { days: 30, series: series(30, () => ({ signups: 0, unsubscribes: 1 })) } });
  assert.doesNotMatch(summary(onlyLeft), /Busiest/);
  const plot = find(fig, (n) => n.props?.class === "plot");
  assert.ok(plot);
});

test("status and language charts: shares, folding beyond eight languages, empty table", () => {
  const stats = {
    total: 10,
    byStatus: { confirmed: 7, pending: 2, unsubscribed: 1 },
    byLanguage: Object.fromEntries(["en", "pl", "de", "fr", "hi", "es", "it", "nl", "pt", "unknown"].map((l, i) => [l, { confirmed: 10 - i, pending: 0 }])),
  };
  assert.equal(summary(statusFigure({ ...ctx, stats })), "10 people in the table: 7 confirmed (70%), 2 pending, 1 unsubscribed.");
  const empty = statusFigure({ ...ctx, stats: { total: 0, byStatus: {}, byLanguage: {} } });
  assert.match(text(empty), /Nobody has signed up yet/);
  const lang = languageFigure({ ...ctx, stats });
  assert.match(summary(lang), /^Most read English \(10 people\); 10 languages in total\.$/);
  const rows = find(lang, (n) => n.tag === "tbody").children;
  assert.equal(rows.length, 9, "eight languages and the rest");
  assert.equal(text(rows[8].children[0]), "2 other languages");
  const tie = languageFigure({ ...ctx, stats: { total: 4, byStatus: {}, byLanguage: { pl: { confirmed: 2 }, de: { confirmed: 2 } } } });
  assert.equal(text(find(tie, (n) => n.tag === "tbody").children[0].children[0]), "German", "a tie is broken by code");
  const none = languageFigure({ ...ctx, stats: { total: 0, byStatus: {}, byLanguage: {} } });
  assert.equal(summary(none), "No languages yet.");
  assert.equal(languageName("pl", "en", t), "Polish");
  assert.equal(languageName("unknown", "en", t), "Not given");
  assert.equal(languageName(null, "en", t), "Not given");
  assert.equal(languageName("!!", "en", t), "!!", "an invalid code is shown as it is");
});

test("campaign and suppression charts", () => {
  const list = { items: [{ id: "a", subject: "", total: 100, sent: 90, failed: 5, skipped: 3, uncertain: 2, unsubscribes: 4, startedAt: "2026-11-01T00:00:00Z" }, { id: "b", subject: "B", total: 50, sent: 50, failed: 0, skipped: 0, uncertain: 0, unsubscribes: 0 }] };
  const c = campaignsFigure({ ...ctx, list });
  assert.equal(summary(c), "The 2 latest started campaigns: 140 messages sent in total; the largest went to 100 people.");
  assert.match(text(c), /\(no subject yet\)/);
  assert.match(text(campaignsFigure({ ...ctx, list: { items: [] } })), /Nothing has been sent yet/);
  const sup = (days: number) => ({ days, total: 9, byReason: { hard: 4, soft: 2, complaint: 1, manual: 2 }, series: series(days, (i) => ({ hard: i % 7 === 0 ? 1 : 0, soft: 0, complaint: 0, manual: 0 })) });
  assert.equal(REASON_KEYS.length, 4);
  for (const [days, first] of [[30, "15 October 2026"], [90, "15 October 2026"], [365, "Oct"]] as const) {
    const f = suppressionFigure({ ...ctx, stats: sup(days) });
    assert.match(summary(f), /^9 addresses are blocked: 4 hard bounces, 2 soft, 1 complaints, 2 by hand; \d+ added in the last \d+ days\.$/);
    const lastRow = find(f, (n) => n.tag === "tbody").children.at(-1);
    assert.equal(text(lastRow.children[0]), first, `${days} days`);
  }
  assert.ok(find(suppressionFigure({ ...ctx, stats: sup(30), headingLevel: "h2" }), (n) => n.tag === "h2"));
});

test("campaign rules: actions per state, segment and payload validation, local times", () => {
  assert.deepEqual(actionsFor("draft"), ["save", "test", "schedule", "duplicate", "delete"]);
  assert.deepEqual(actionsFor("sending"), ["pause", "cancel", "duplicate"]);
  assert.deepEqual(actionsFor("sent"), ["duplicate"]);
  assert.deepEqual(actionsFor("bogus"), []);
  assert.ok(CONFIRM.has("schedule") && CONFIRM.has("delete") && !CONFIRM.has("pause"));
  assert.ok(isLive("paused") && isLive("sending") && !isLive("sent"));
  assert.ok(hasStats({ startedAt: "x" }) && !hasStats({ startedAt: null }));
  assert.deepEqual(splitList("news, Offers;news  weekly"), ["news", "offers", "weekly"]);
  assert.deepEqual(splitList("pt-BR, en", false), ["pt-BR", "en"]);
  assert.deepEqual(splitList(null), []);
  assert.deepEqual(segmentFrom({ languages: "en, pt-BR", tags: "News", tagMode: "all" }), { segment: { languages: ["en", "pt-BR"], tags: ["news"], tagMode: "all" } });
  assert.deepEqual(segmentFrom({ languages: "english!", tags: "", tagMode: "any" }), { error: "segment.badLanguage", value: "english!" });
  assert.deepEqual(segmentFrom({ languages: "", tags: "a+b", tagMode: "x" }), { error: "segment.badTag", value: "a+b" });
  const f = { subject: "Hi\nthere", body: "B", bodyFormat: "html", textOverride: "  ", fromEmail: " ", fromName: "", languages: "", tags: "", tagMode: "any" };
  assert.deepEqual(payloadFrom(f).body, { subject: "Hi there", body: "B", bodyFormat: "html", textOverride: null, fromEmail: null, fromName: null, segment: { languages: [], tags: [], tagMode: "any" } });
  const g = payloadFrom({ ...f, bodyFormat: "x", textOverride: "T", fromEmail: "a@example.com", fromName: "N" }).body;
  assert.deepEqual([g.bodyFormat, g.textOverride, g.fromEmail, g.fromName], ["markdown", "T", "a@example.com", "N"]);
  assert.equal(payloadFrom({ ...f, tags: "!" }).error, "segment.badTag");
  const fields = fieldsOf({ subject: "S", body: "B", bodyFormat: "html", textOverride: null, fromEmail: null, fromName: "N", segment: { languages: ["en"], tags: ["a", "b"], tagMode: "all" } });
  assert.deepEqual(fields, { subject: "S", body: "B", bodyFormat: "html", textOverride: "", fromEmail: "", fromName: "N", languages: "en", tags: "a, b", tagMode: "all" });
  const blank = fieldsOf({});
  assert.deepEqual([blank.bodyFormat, blank.tagMode, blank.languages], ["markdown", "any", ""]);
  assert.ok(sameFields(fields, { ...fields }) && !sameFields(fields, { ...fields, body: "x" }));
  const local = toLocalInput("2026-11-13T10:05:00Z");
  assert.match(local, /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/);
  assert.equal(fromLocalInput(local), "2026-11-13T10:05:00.000Z", "round trip through the browser's zone");
  assert.equal(toLocalInput("nope"), "");
  assert.equal(fromLocalInput("2026-11-13"), null);
  assert.equal(fromLocalInput(undefined), null);
  assert.equal(fromLocalInput("2026-13-45T99:99"), null);
});

test("campaign results and clicks: bars against the total, the caveats in words", () => {
  const stats = {
    recipients: { total: 200, sent: 180, failed: 5, skipped: 10, pending: 0, uncertain: 5 },
    unsubscribes: { count: 4, windowDays: 14 },
    clicks: { tracking: true, total: 30, links: [{ url: "https://example.com/a/", clicks: 20 }, { url: "http://example.com/" + "x".repeat(80), clicks: 10 }] },
  };
  const r = resultsFigure({ ...ctx, stats });
  assert.equal(summary(r), "180 of 200 recipients got it (90%); 5 failed; 4 unsubscribed afterwards.");
  assert.equal(find(r, (n) => n.tag === "tbody").children.length, 6);
  assert.match(text(resultsFigure({ ...ctx, stats: { ...stats, recipients: { total: 0, sent: 0, failed: 0, skipped: 0, pending: 0, uncertain: 0 } } })), /No recipients were counted/);
  const c = clicksFigure({ ...ctx, stats });
  assert.equal(summary(c), "30 clicks on 2 links; most on example.com/a/.");
  assert.equal(clicksFigure({ ...ctx, stats: { ...stats, clicks: { tracking: false, total: 0, links: [] } } }), null);
  const none = clicksFigure({ ...ctx, stats: { ...stats, clicks: { tracking: true, total: 0, links: [] } } });
  assert.equal(summary(none), "No link has been clicked yet.");
  assert.equal(shortUrl("https://example.com/x").length, 13);
  assert.equal(shortUrl("http://" + "y".repeat(100)).length, 60);
});

test("lists: filters, tags, import mapping, consent rule, names of erased rows", async () => {
  assert.deepEqual(cleanFilters({ status: "pending", language: "pt-BR", tag: " News ", q: "  anna ", x: 1 }), { status: "pending", language: "pt-BR", tag: "news", q: "anna" });
  assert.deepEqual(cleanFilters({ status: "nope", language: "english!", tag: "a b", q: " " }), {});
  assert.deepEqual(cleanFilters(), {});
  assert.deepEqual(tagsFrom("News; offers, news"), { tags: ["news", "offers"] });
  assert.deepEqual(tagsFrom("ok, not ok"), { error: "not ok" });
  assert.deepEqual(tagsFrom(undefined), { tags: [] });
  const detected = { columns: { email: "E-mail", status: "Status", signedUpAt: null, unsubscribedAt: null, bouncedAt: null, language: null, tags: "Groups", sourcePage: null }, ignored: [{ header: "First name", reason: "x" }, { header: "Status", reason: "dup" }] };
  assert.deepEqual(headersOf(detected), ["E-mail", "Status", "Groups", "First name"]);
  assert.deepEqual(mappingFrom(detected, { email: "E-mail", tags: "", language: "First name" }), { tags: null, language: "First name" });
  assert.deepEqual(mappingFrom(detected, {}), {});
  assert.equal(duplicateColumn({ email: "A", status: "B", tags: "A" }), "A");
  assert.equal(duplicateColumn({ email: "A", status: "", tags: null }), null);
  assert.equal(delimiterKey(";"), "delimiter.semicolon");
  assert.equal(delimiterKey(","), "delimiter.comma");
  assert.equal(delimiterKey("\t"), "delimiter.tab");
  assert.equal(delimiterKey("|"), "delimiter.other");
  assert.equal(ignoredKey("ignored: this worker does not store names"), "import.reason.names");
  assert.equal(ignoredKey('ignored: "A" was used for email'), "import.reason.lost");
  assert.equal(ignoredKey("ignored: not a recognised column"), "import.reason.other");
  assert.deepEqual(importBody({ csv: "a", consentSource: " form ", confirmed: 1, dateOrder: "dmy", mapping: { a: 1 } }), { csv: "a", consentSource: "form", confirmed: true, mapping: { a: 1 }, dateOrder: "dmy" });
  assert.deepEqual(importBody({ csv: "a", dateOrder: "xyz" }), { csv: "a", consentSource: "", confirmed: false, mapping: {} });
  const cc = (o: Record<string, number>) => ({ classCounts: { toConfirm: 0, toPending: 0, toUnsubscribed: 0, toSuppress: 0, ...o } });
  assert.ok(needsConsent(cc({ toPending: 1 })));
  assert.ok(needsConsent(cc({})), "an import that writes nothing still asks, as the worker does");
  assert.ok(!needsConsent(cc({ toUnsubscribed: 2, toSuppress: 1 })));
  assert.equal(suppressionName({ email: "a@example.com", emailHash: "ff" }, t), "a@example.com");
  assert.equal(suppressionName({ email: null, emailHash: "0123456789abcdef" }, t), "erased address · 0123456789…");
  assert.ok(liftNeedsConfirm({ reason: "complaint" }) && !liftNeedsConfirm({ reason: "hard" }));
  assert.equal(sendStatusKey("sent"), "send.sent");
  const file = (bytes: number[]) => ({ arrayBuffer: async () => new Uint8Array(bytes).buffer });
  assert.equal(await readFileText(file([0xef, 0xbb, 0xbf, 0x61, 0x62])), "ab");
  assert.equal(await readFileText(file([0xff, 0xfe, 0x61, 0, 0x62, 0])), "ab");
  assert.equal(await readFileText(file([0xfe, 0xff, 0, 0x61, 0, 0x62])), "ab");
  assert.equal(await readFileText(file([0x7a])), "z");
});
