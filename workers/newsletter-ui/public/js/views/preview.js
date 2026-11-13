// The preview beside the editor: the mail exactly as the worker renders it
// (POST /api/campaigns/:id/preview), in a chosen language, as HTML or as the
// plain-text part, plus the live recipient count of the segment.
//
// The HTML is shown in a sandboxed frame by mail-frame.js, which keeps the
// mail's styling without relaxing the panel's CSP (see there for how).

import { h } from "../core/vnode.js";
import { errorText, slot } from "./ui.js";
import { mailFrame } from "./mail-frame.js";

export function previewPane(app, getCampaign, langs) {
  const { t, fmt, api } = app;
  const state = { lang: "en", tab: "html", data: null };
  const body = slot("preview-body");
  const count = slot("recipients");
  const head = slot("preview-head");

  const tabs = ["html", "text"];
  const selectTab = (tab, focus = false) => {
    state.tab = tab;
    for (const b of tabList.querySelectorAll("[role=tab]")) {
      const on = b.dataset.tab === tab;
      b.setAttribute("aria-selected", String(on));
      b.tabIndex = on ? 0 : -1;
      if (on && focus) b.focus();
    }
    draw();
  };
  let tabList = null;
  const onTabKey = (e) => {
    const i = tabs.indexOf(state.tab);
    const next = { ArrowRight: (i + 1) % 2, ArrowLeft: (i + 1) % 2, Home: 0, End: 1 }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    selectTab(tabs[next], true);
  };

  function draw() {
    const d = state.data;
    if (!d) return;
    head.set(h("dl", { class: "facts compact" }, h("dt", null, t("preview.subject")), h("dd", null, d.subject), h("dt", null, "List-Unsubscribe"), h("dd", { class: "mono" }, d.headers["List-Unsubscribe"] ?? "—")));
    body.set(
      state.tab === "html"
        ? h("div", { id: "preview-panel", role: "tabpanel", "aria-labelledby": "tab-html", ref: (el) => el.appendChild(mailFrame(el.ownerDocument, { html: d.html, title: t("preview.frame", { lang: t(`pref.lang.${state.lang}`) }) })) })
        : h("pre", { class: "mail-text", id: "preview-panel", role: "tabpanel", "aria-labelledby": "tab-text", tabindex: "0" }, d.text),
    );
  }

  function showCount(r) {
    const by = Object.entries(r.byLanguage ?? {}).sort((a, b) => b[1] - a[1]);
    count.set(
      h(
        "p",
        { class: `recipients-count ${r.total ? "" : "zero"}`.trim() },
        h("strong", null, t("preview.recipients", { count: r.total })),
        by.length ? " — " + by.map(([l, n]) => `${l || t("lang.unknown")} ${fmt.number(n)}`).join(", ") : null,
      ),
    );
  }

  async function refresh() {
    const c = getCampaign();
    if (!c) return;
    try {
      state.data = await api.post(`campaigns/${encodeURIComponent(c.id)}/preview`, { language: state.lang });
      showCount(state.data.recipients);
      draw();
    } catch (e) {
      body.set(h("div", { class: "callout warning", role: "status" }, h("p", null, errorText(app, e))));
    }
  }

  const node = h(
    "section",
    { class: "card preview", "aria-labelledby": "preview-title" },
    h("div", { class: "preview-bar" }, h("h2", { id: "preview-title" }, t("preview.title")),
      h("div", { class: "field inline" }, h("label", { for: "pv-lang" }, t("preview.language")), h("select", { id: "pv-lang", on: { change: (e) => ((state.lang = e.target.value), refresh()) } }, langs.map((l) => h("option", { value: l }, t(`pref.lang.${l}`)))))),
    h(
      "div",
      { role: "tablist", class: "tabs", "aria-label": t("preview.parts"), ref: (el) => (tabList = el), on: { keydown: onTabKey } },
      tabs.map((tab) => h("button", { type: "button", role: "tab", id: `tab-${tab}`, "data-tab": tab, "aria-controls": "preview-panel", "aria-selected": String(tab === "html"), tabindex: tab === "html" ? "0" : "-1", on: { click: () => selectTab(tab) } }, t(`preview.tab.${tab}`))),
    ),
    head.node,
    body.node,
  );
  return {
    node,
    count: count.node,
    refresh,
    empty: () => {
      body.set(h("p", { class: "muted" }, t("preview.afterSave")));
      count.set(h("p", { class: "recipients-count" }, t("preview.countAfterSave")));
    },
  };
}

