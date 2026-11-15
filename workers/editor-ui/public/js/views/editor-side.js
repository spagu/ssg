// Beside the editor (below it on a narrow screen): the preview, or the
// article's history (GO-172). The preview is redrawn a moment after typing
// stops, from models/preview.js — nodes, never HTML — and says under it
// that it is approximate. History lists the revisions with "View".

import { h, mount } from "../core/vnode.js";
import { renderPreview } from "../models/preview.js";
import { revisionsPanel } from "../panel/revisions.js";
import { slot } from "../panel/ui.js";
import { viewRevision } from "./revision-view.js";

export function sidePanel(app, ed) {
  const { t } = app;
  const body = slot("side-body");
  let tab = "preview";
  let previewEl = null;
  let timer = 0;
  const preview = () => {
    if (!previewEl) return;
    mount(previewEl, renderPreview(ed.doc.body, { t, images: ed.previewImages(), title: String(ed.doc.inputs.title ?? "").trim() }));
  };
  const history = () => {
    if (ed.doc.isNew) return h("p", { class: "muted" }, t("rev.none"));
    const path = encodeURIComponent(ed.doc.path);
    return revisionsPanel(app, {
      title: t("rev.title"),
      // Each revision remembers the page it was listed on: reading it asks
      // the worker to look from there (GO-172 membership).
      load: async (cursor) => {
        const page = await app.api.get(`revisions/${path}`, { cursor });
        return { ...page, items: page.items.map((r) => ({ ...r, cursor })) };
      },
      onView: (r) => void viewRevision(app, ed, r).catch((e) => app.notify(String(e?.message ?? e), "critical")),
    });
  };
  const tabs = h("div", { class: "side-tabs", role: "group", "aria-label": t("side.label") },
    ["preview", "history"].map((name) => h("button", { type: "button", class: "btn side-tab", id: `side-${name}`, "aria-pressed": String(name === tab), on: { click: () => choose(name) } }, t(`side.${name}`))));
  function choose(name) {
    tab = name;
    for (const b of document.querySelectorAll(".side-tab")) b.setAttribute("aria-pressed", String(b.id === `side-${name}`));
    if (name === "preview") {
      body.set([h("div", { class: "preview", role: "region", "aria-label": t("side.preview"), ref: (el) => (previewEl = el) }), h("p", { class: "hint preview-note" }, t("preview.note"))]);
      preview();
    } else {
      previewEl = null;
      body.set(history());
    }
  }
  ed.changedText = () => {
    clearTimeout(timer);
    timer = setTimeout(preview, 200);
  };
  queueMicrotask(() => choose("preview"));
  return h("section", { class: "editor-side card", "aria-label": t("side.label") }, tabs, body.node);
}
