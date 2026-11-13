// One campaign. A draft opens in the editor; anything else shows where it
// stands: live progress while it sends, results once it has started, the
// uncertain sends that need a decision, and the mail as it was rendered.

import { href } from "../core/router.js";
import { LANGS } from "../core/i18n.js";
import { createPoller } from "../core/tasks.js";
import { h, mount } from "../core/vnode.js";
import { progress } from "../charts/bars.js";
import { clicksFigure, isLive, resultsFigure } from "../models/campaign.js";
import { languageName } from "../models/dashboard.js";
import { campaignActions } from "./campaign-actions.js";
import editor from "./editor.js";
import { previewPane } from "./preview.js";
import { uncertainSection } from "./uncertain.js";
import { chip, errorBlock, loading, pageHead, slot } from "./ui.js";

const facts = (pairs) => h("dl", { class: "facts" }, pairs.map(([k, v]) => [h("dt", null, k), h("dd", null, v)]));

function segmentText(app, s) {
  const { t, fmt } = app;
  const parts = [t("seg.confirmed")];
  if (s.languages.length) parts.push(t("seg.languages", { list: fmt.list(s.languages.map((l) => languageName(l, app.lang, t))) }));
  if (s.tags.length) parts.push(t(`seg.tags.${s.tagMode}`, { list: fmt.list(s.tags) }));
  return parts.join(" · ");
}

function progressCard(app, p) {
  const { t } = app;
  return h(
    "section",
    { class: "card", "aria-labelledby": "prog-title" },
    h("h2", { id: "prog-title" }, t("prog.title")),
    progress({ p, t, fmt: app.fmt }),
    h("p", { class: "muted" }, t("prog.detail", { claimed: app.fmt.number(p.claimed), remaining: app.fmt.number(p.remaining) })),
    p.error ? h("div", { class: "callout critical" }, h("p", { class: "callout-title" }, t("prog.paused")), h("p", null, p.error), h("p", null, t("prog.paused.next"))) : null,
  );
}

function statsCard(app, s) {
  const { t, fmt } = app;
  const clicks = clicksFigure({ stats: s, t, fmt });
  return h(
    "section",
    { class: "card", "aria-labelledby": "stats-title" },
    h("h2", { id: "stats-title" }, t("stats.title")),
    h("div", { class: "chart-grid tight" }, resultsFigure({ stats: s, t, fmt }), clicks),
    h("p", { class: "note" }, t("stats.unsubNote", { days: s.unsubscribes.windowDays })),
    clicks ? h("p", { class: "note" }, t("stats.clickNote")) : h("p", { class: "note" }, t("stats.noClicks")),
    h("p", { class: "note" }, t("stats.bounceNote")),
    facts([
      [t("timing.scheduled"), fmt.dateTime(s.timing.scheduledAt)],
      [t("timing.started"), fmt.dateTime(s.timing.startedAt)],
      [t("timing.finished"), fmt.dateTime(s.timing.finishedAt)],
      [t("timing.duration"), fmt.duration(s.timing.durationSeconds)],
    ]),
  );
}

export default async function campaign(app, root, route) {
  const { t, fmt, api } = app;
  const id = route.params.id;
  const back = { href: href("campaigns"), label: t("nav.campaigns") };
  mount(root, [pageHead(app, t("camp.loading"), { back }), loading(app)]);
  let c;
  try {
    c = await api.get(`campaigns/${encodeURIComponent(id)}`);
  } catch (e) {
    mount(root, [pageHead(app, t("camp.title"), { back }), errorBlock(app, e, () => campaign(app, root, route))]);
    return;
  }
  if (c.status === "draft") return editor(app, root, route, c);

  const prog = slot();
  const stats = slot();
  const uncertain = slot();
  const preview = previewPane(app, () => c, [...new Set([...c.segment.languages.map((l) => l.split("-")[0]), ...LANGS])].filter((l) => LANGS.includes(l)));
  mount(root, [
    pageHead(app, c.subject || t("campaign.untitled"), { back, actions: campaignActions(app, c) }),
    h(
      "section",
      { class: "card", "aria-labelledby": "about-title" },
      h("h2", { id: "about-title", class: "visually-hidden" }, t("camp.about")),
      facts([
        [t("col.status"), chip(app, c.status)],
        [t("camp.segment"), segmentText(app, c.segment)],
        [t("editor.sender"), c.fromEmail ? `${c.fromName ? c.fromName + " " : ""}<${c.fromEmail}>` : t("camp.defaultSender")],
        [t("timing.scheduled"), fmt.dateTime(c.scheduledAt)],
        [t("camp.recipients"), c.totalCount ? fmt.number(c.totalCount) : "—"],
      ]),
    ),
    prog.node,
    stats.node,
    uncertain.node,
    preview.node,
  ]);
  void preview.refresh(); // draws its own error state

  const loadStats = async () => {
    if (!c.startedAt) return;
    try {
      stats.set(statsCard(app, await api.get(`campaigns/${encodeURIComponent(id)}/stats`)));
    } catch (e) {
      stats.set(errorBlock(app, e, loadStats));
    }
  };
  const loadProgress = async () => {
    const p = await api.get(`campaigns/${encodeURIComponent(id)}/progress`);
    prog.set(progressCard(app, p));
    if (p.status !== c.status) {
      // The engine finished it (or paused it) while we watched: show the new state.
      app.notify(t("camp.statusChanged", { status: t(`status.${p.status}`) }));
      app.go(href("campaign", { id }));
      return true;
    }
    return p.status !== "sending";
  };

  if (isLive(c.status)) {
    try {
      const done = await loadProgress();
      if (!done && c.status === "sending") {
        const poller = createPoller({ tick: loadProgress, interval: 3000, visible: () => document.visibilityState === "visible", onError: (e) => prog.set(errorBlock(app, e)) });
        setTimeout(() => poller.start(), 3000);
        app.visibility = () => poller.wake();
        app.onLeave(() => (poller.stop(), (app.visibility = null)));
      }
    } catch (e) {
      prog.set(errorBlock(app, e, loadProgress));
    }
  }
  await loadStats();
  if (c.startedAt) uncertain.set(await uncertainSection(app, c, loadStats));
  if (route.query.tab === "uncertain") document.getElementById("unc-title")?.focus();
}
