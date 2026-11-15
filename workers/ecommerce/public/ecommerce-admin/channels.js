// Where an order came from (GO-165): the shop's own checkout, an AI agent
// platform through ACP, or Google's agentic checkout through UCP. The same
// three values as functions/api/shop/_channels.ts.
//
// A channel is always shown as a word — "Web", "ACP", "UCP" — with a full
// description as its tooltip; the colour of the badge only repeats what the
// word says (WCAG 2.2, 1.4.1). The descriptions are plain text, set through
// textContent like everything else in the panel.

import { el, money, td } from "./dom.js";

/** What the panel calls each channel, short and long. */
export const CHANNELS = {
  web: { name: "Web", hint: "Ordered on this site" },
  acp: { name: "ACP", hint: "Ordered through an AI agent (Agentic Commerce Protocol)" },
  ucp: { name: "UCP", hint: "Ordered through Google's agentic checkout (Universal Commerce Protocol)" },
};

/** known: the panel's entry for a channel — own keys only, so a stored value
 *  such as "constructor" is not mistaken for one. */
function known(channel) {
  return Object.hasOwn(CHANNELS, channel) ? CHANNELS[channel] : null;
}

/** channelName: the badge's word. An order from before channels were recorded
 *  came from the site; a channel this panel does not know yet is shown as the
 *  shop stored it rather than hidden. */
export function channelName(channel) {
  return known(channel ?? "web")?.name ?? String(channel);
}

/** channelHint: the sentence behind the word. */
export function channelHint(channel) {
  return known(channel ?? "web")?.hint ?? `Channel “${String(channel)}”`;
}

/** channelBadge: the word, as a badge styled per channel (admin.css). */
export function channelBadge(channel) {
  const value = channel ?? "web";
  return el("span", {
    class: "badge channel",
    "data-channel": known(value) ? value : "other",
    title: channelHint(value),
    text: channelName(value),
  });
}

/** drawChannelStats fills the overview's "by channel" table from the stats'
 *  byChannel rows (admin/stats.ts): orders, gross, refunds and what is left,
 *  per channel and currency. */
export function drawChannelStats(tbody, rows) {
  if (rows.length === 0) {
    tbody.replaceChildren(el("tr", {}, el("td", { colspan: "5", class: "muted", text: "No paid orders yet." })));
    return;
  }
  tbody.replaceChildren(
    ...rows.map((r) =>
      el("tr", {},
        td("Channel", channelBadge(r.channel)),
        td("Orders", { class: "numeric" }, String(r.orders)),
        td("Gross", { class: "numeric" }, money(r.grossMinor, r.currency)),
        td("Refunded", { class: "numeric" }, refunded(r)),
        td("Net", { class: "numeric" }, money(r.netMinor, r.currency)),
      ),
    ),
  );
}

/** refunded: the amount, and how many refunds made it when there were any. */
export function refunded(row) {
  const amount = money(row.refundedMinor, row.currency);
  return row.refunds > 0 ? `${amount} (${row.refunds})` : amount;
}
