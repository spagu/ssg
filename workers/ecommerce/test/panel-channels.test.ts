// GO-165: the panel's words for a channel (channels.js) and the order filter's
// choices, against the server's list (_channels.ts). The badge and the
// overview table are drawn on a few lines of stand-in DOM — enough for dom.js's
// el(); how they look is checked in a browser (panel-shots.mjs).
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import {
  CHANNELS as PANEL, channelBadge, channelHint, channelName, drawChannelStats, refunded,
} from "../public/ecommerce-admin/channels.js";
import { CHANNELS } from "../functions/api/shop/_channels";

describe("the panel's channels", () => {
  it("name every channel the server knows, and only those", () => {
    assert.deepEqual(Object.keys(PANEL), [...CHANNELS]);
    assert.deepEqual(CHANNELS.map(channelName), ["Web", "ACP", "UCP"]);
  });

  it("call an order without a channel a web order", () => {
    assert.equal(channelName(null), "Web");
    assert.equal(channelName(undefined), "Web");
    assert.equal(channelHint(null), "Ordered on this site");
  });

  it("show a channel they do not know as stored, never an inherited property", () => {
    assert.equal(channelName("later"), "later");
    assert.equal(channelHint("later"), "Channel “later”");
    assert.equal(channelName("constructor"), "constructor");
    assert.equal(channelHint("toString"), "Channel “toString”");
  });

  it("describe each channel in words, not colour", () => {
    assert.match(channelHint("acp"), /AI agent/);
    assert.match(channelHint("ucp"), /Google/);
  });

  it("add the number of refunds to the amount only when there were any", () => {
    assert.match(refunded({ refundedMinor: 500, refunds: 2, currency: "EUR" }), /5\.00.*\(2\)$/);
    assert.doesNotMatch(refunded({ refundedMinor: 0, refunds: 0, currency: "EUR" }), /\(/);
  });

  it("offer exactly those channels in the order filter", () => {
    const html = readFileSync(fileURLToPath(new URL("../public/ecommerce-admin/index.html", import.meta.url).href), "utf8");
    const select = html.match(/<select id="filter-channel"[^>]*>([\s\S]*?)<\/select>/)![1]!;
    const values = [...select.matchAll(/value="([^"]*)"/g)].map((m) => m[1]);
    assert.deepEqual(values, ["", ...CHANNELS]);
  });
});

/** Just what el() and replaceChildren touch. */
class FakeNode {
  tag: string;
  attrs: Record<string, string> = {};
  children: FakeNode[] = [];
  textContent = "";
  className = "";
  dataset: Record<string, string> = {};
  constructor(tag: string) {
    this.tag = tag;
  }
  setAttribute(k: string, v: string) {
    this.attrs[k] = v;
  }
  append(...c: FakeNode[]) {
    this.children.push(...c);
  }
  replaceChildren(...c: FakeNode[]) {
    this.children = c;
  }
  addEventListener() {}
  get text(): string {
    return this.textContent + this.children.map((c) => c.text).join("");
  }
}
const g = globalThis as Record<string, unknown>;
g.Node = FakeNode;
g.document = {
  createElement: (t: string) => new FakeNode(t),
  createTextNode: (t: string) => Object.assign(new FakeNode("#text"), { textContent: t }),
};

describe("drawing channels", () => {
  it("draws a badge with the word, the sentence and a style hook", () => {
    const badge = channelBadge("acp") as unknown as FakeNode;
    assert.deepEqual([badge.className, badge.attrs["data-channel"], badge.textContent], ["badge channel", "acp", "ACP"]);
    assert.match(badge.attrs.title!, /AI agent/);
    const old = channelBadge(null) as unknown as FakeNode;
    assert.deepEqual([old.attrs["data-channel"], old.textContent], ["web", "Web"]);
    const later = channelBadge("later") as unknown as FakeNode;
    assert.deepEqual([later.attrs["data-channel"], later.textContent], ["other", "later"]);
  });

  it("fills the overview table, one row per channel and currency", () => {
    const tbody = new FakeNode("tbody");
    drawChannelStats(tbody, [
      { channel: "web", currency: "EUR", orders: 3, grossMinor: 6000, refunds: 1, refundedMinor: 1000, netMinor: 5000 },
      { channel: "ucp", currency: "EUR", orders: 1, grossMinor: 900, refunds: 0, refundedMinor: 0, netMinor: 900 },
    ]);
    assert.equal(tbody.children.length, 2);
    const cells = tbody.children[0]!.children;
    assert.deepEqual(cells.map((c) => c.attrs["data-label"]), ["Channel", "Orders", "Gross", "Refunded", "Net"]);
    assert.equal(cells[0]!.text, "Web");
    assert.match(cells[3]!.text, /\(1\)$/);
  });

  it("says so when nothing was paid", () => {
    const tbody = new FakeNode("tbody");
    drawChannelStats(tbody, []);
    assert.equal(tbody.children[0]!.text, "No paid orders yet.");
    assert.equal(tbody.children[0]!.children[0]!.attrs.colspan, "5");
  });
});
