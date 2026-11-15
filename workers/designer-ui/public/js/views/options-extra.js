// Around the Options tab: what the worker could not read (problems), and the
// settings' history — the revisions of .ssg.designer.json, each viewable and
// restorable as pending changes (published like any other edit).

import { h } from "../core/vnode.js";
import { fromDraft, toDraft } from "../models/controls.js";
import { fieldsOf } from "../models/form-model.js";
import { labelOf } from "../models/labels.js";
import { leaves, restoreChanges } from "../models/revision-restore.js";
import { keyOf } from "../models/settings-changes.js";
import { openDialog } from "../panel/dialog.js";
import { revisionsPanel } from "../panel/revisions.js";
import { busy } from "../panel/ui.js";
import { valueView } from "./value-view.js";

// problemsBlock: the reader's problems, by code, each with what to do.
export function problemsBlock(app, view) {
  const { t } = app;
  const items = [...view.problems];
  if (view.configFormat && view.configFormat !== "yaml") items.unshift({ code: "overlay_needs_yaml", detail: view.configFile ?? "" });
  if (!items.length) return null;
  return h(
    "ul",
    { class: "warnings" },
    items.map((p) => h("li", { class: "callout warning" }, h("p", null, t(`settingsProblem.${p.code}`)), p.detail ? h("p", { class: "hint", lang: "en" }, p.detail) : null)),
  );
}

// settingsHistory opens (or closes) the revisions of the overlay in `side`.
export function settingsHistory(app, side, button) {
  const open = button.getAttribute("aria-expanded") === "true";
  button.setAttribute("aria-expanded", String(!open));
  if (open) return side.set(null);
  const path = app.settings.view.overlayFile;
  side.set(
    h(
      "div",
      { id: "settings-history" },
      revisionsPanel(app, {
        title: app.t("rev.settingsTitle", { path }),
        load: (cursor) => app.api.get("revisions", { path, cursor }),
        onView: (r) => void busy(app, null, () => viewRevision(app, path, r)),
      }),
    ),
  );
}

// viewRevision shows the overlay at a commit and offers to restore it.
async function viewRevision(app, path, rev) {
  const { t } = app;
  const got = await app.api.get(`revisions/${encodeURIComponent(rev.commitId)}`, { path });
  const list = leaves(got.settings);
  const plan = restoreChanges(fieldsOf(app.settings.form), got.settings);
  const can = app.caps.settings;
  const content = [
    h("p", { class: "hint" }, t("rev.by", { author: rev.authorName || rev.authorEmail, date: app.fmt.dateTime(rev.date) }), " \u2014 ", rev.message.split("\n")[0]),
    list.length ? h("dl", { class: "value-map" }, list.map((l) => [h("dt", null, h("code", null, l.path)), h("dd", null, valueView(app, l.value))])) : h("p", { class: "muted" }, t("rev.settingsEmpty")),
    plan.skipped.length ? h("p", { class: "callout warning" }, t("rev.skipped", { keys: plan.skipped.map((e) => labelOf(app, e)).join(", ") })) : null,
    can ? h("p", null, t("rev.restoreWhat", { set: plan.set.length, revert: plan.revert.length })) : null,
  ];
  const actions = [{ label: t("action.close"), value: null }];
  if (can && plan.set.length + plan.revert.length) actions.push({ label: t("rev.restore"), value: "restore", variant: "primary", submit: true });
  const choice = await openDialog({ title: t("rev.settingsAt", { commit: rev.commitId.slice(0, 7) }), content, actions, wide: true });
  if (choice !== "restore") return;
  for (const { entry, value } of plan.set) {
    const k = keyOf(entry.segments);
    if (!app.baselines.has(k)) app.baselines.set(k, fromDraft(toDraft(entry.node, entry.value)).value);
    app.changes.set(entry, value, app.baselines.get(k));
    app.drafts.set(k, toDraft(entry.node, value));
  }
  for (const entry of plan.revert) {
    app.drafts.delete(keyOf(entry.segments));
    app.changes.revert(entry);
  }
  app.changed();
  app.notify(t("rev.restored", { count: plan.set.length + plan.revert.length }));
  app.refresh();
}
