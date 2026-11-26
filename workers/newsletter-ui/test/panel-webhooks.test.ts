// #460: the Settings card for webhook health — totals per kind, the latest
// verified webhook, and the warning when the transport has been silent.
import { test } from "node:test";
import assert from "node:assert/strict";
import { find, text } from "../public/js/core/vnode.js";
import { createI18n } from "../public/js/core/i18n.js";
import { createFormat } from "../public/js/core/format.js";
import en from "../public/js/strings/en.js";
import pl from "../public/js/strings/pl.js";
import { webhookCard } from "../public/js/views/webhook-activity.js";

const fmt = createFormat("en-GB", "UTC");
const app = (lang = "en") => ({ t: createI18n(lang, { en, pl }, {}, fmt.number).t, fmt });
const counts = { delivered: 1200, hard: 3, soft: 1, complaint: 0, other: 7 };
const activity = (over: Record<string, unknown> = {}) => ({
  lastReceivedAt: "2026-11-26T10:15:00.000Z",
  last24h: counts,
  last7d: { ...counts, delivered: 5000 },
  reportsByWebhook: true,
  lastCampaignStartedAt: "2026-11-26T09:00:00.000Z",
  warning: null,
  ...over,
});
const callout = (n: unknown) => find(n as never, (x) => String(x.props?.class ?? "").startsWith("callout"));

test("webhook card: the latest time and the counts per kind, no warning when webhooks arrive", () => {
  const card = webhookCard(app(), activity());
  const all = text(card);
  assert.match(all, /26 Nov 2026, 10:15/);
  assert.match(all, /delivered: 1,200 · hard bounces: 3 · soft bounces: 1 · complaints: 0 · other: 7/);
  assert.match(all, /delivered: 5,000/);
  assert.equal(callout(card), null);
});

test("webhook card: none received yet, and the warning names the campaign's start", () => {
  const card = webhookCard(app(), activity({ lastReceivedAt: null, warning: "no_webhook_since_campaign" }));
  assert.match(text(card), /None received yet/);
  const w = callout(card);
  assert.equal(w?.props.class, "callout warning");
  assert.match(text(w), /No webhook since the last campaign.*started 26 Nov 2026, 09:00/);
  assert.match(text(webhookCard(app("pl"), activity({ warning: "no_webhook_since_campaign" }))), /Brak webhooka od ostatniej kampanii/);
});
