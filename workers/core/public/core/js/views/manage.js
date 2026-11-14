// "Modules" (GO-149, /core/#modules): every module with its switch, its
// place in the order, its name in each language and an admin note — then the
// templates not connected, the status history, the change log and the
// settings file (views/manage-more.js). Pure: each function returns a tree;
// the actions come in through app.manage (js/manage-state.js).
//
// Honest about the limits: next to every switch the page says that hiding is
// cosmetic and how to really switch the module off (its manifest's words).

import { h } from "../vnode.js";
import { prefs, errorBlock, loading } from "./shell.js";
import { LABEL_LANGS, panelKey, readOnlyKey, shownTitle, statusLine, whoKey } from "../manage.js";
import { availableSection, changesSection, historySection, transferSection, failure } from "./manage-more.js";

function head(app) {
  const { t } = app;
  return h(
    "div",
    { class: "page-head" },
    h(
      "div",
      null,
      h("p", { class: "back" }, h("a", { href: "#", id: "manage-back" }, t("manage.back"))),
      h("h1", { tabindex: "-1", id: "manage-title" }, t("manage.title")),
      h("p", { class: "lead" }, t("manage.lead")),
    ),
    h("div", { class: "toolbar" }, prefs(app)),
  );
}

// orderButtons: move up / move down. At an end the button stays focusable
// (aria-disabled, never disabled): a keyboard user who moved a module to the
// top keeps focus where it was and hears that it is already first.
function orderButtons(app, item, index, count, locked) {
  const { t } = app;
  const name = shownTitle(item, app.lang);
  const button = (dir, edge) =>
    h(
      "button",
      {
        type: "button",
        class: "btn btn-quiet move",
        id: `${dir}-${item.id}`,
        "aria-label": t(`manage.${dir}.label`, { module: name }),
        "aria-disabled": edge ? "true" : null,
        disabled: locked,
        on: { click: () => void app.manage.move(item.id, dir === "up" ? -1 : 1) },
      },
      h("span", { "aria-hidden": "true" }, dir === "up" ? "↑ " : "↓ "),
      t(`manage.${dir}`),
    );
  return h(
    "div",
    { class: "move-group", role: "group", "aria-label": t("manage.order") },
    h("span", { class: "position" }, t("manage.position", { position: index + 1, count })),
    button("up", index === 0),
    button("down", index === count - 1),
  );
}

function editForm(app, item, locked) {
  const { t } = app;
  const draft = app.manage.state.drafts[item.id];
  return h(
    "form",
    { class: "edit", on: { submit: (e) => (e.preventDefault(), void app.manage.save(item.id)) } },
    h(
      "fieldset",
      null,
      h("legend", null, t("manage.labels")),
      h("p", { class: "note", id: `labels-hint-${item.id}` }, t("manage.labels.hint", { title: item.title })),
      h(
        "div",
        { class: "label-grid" },
        LABEL_LANGS.map((l) =>
          h(
            "div",
            { class: "field" },
            h("label", { for: `label-${item.id}-${l}` }, t(`pref.lang.${l}`)),
            h("input", {
              type: "text", id: `label-${item.id}-${l}`, lang: l, value: draft.labels[l], maxlength: String(app.manage.state.view.limits.label),
              autocomplete: "off", disabled: locked, "aria-describedby": `labels-hint-${item.id}`, on: { input: (e) => app.manage.edit(item.id, "label", e.target.value, l) },
            }),
          ),
        ),
      ),
    ),
    h("label", { for: `note-${item.id}`, class: "note-label" }, t("manage.note")),
    h("p", { class: "note", id: `note-hint-${item.id}` }, t("manage.note.hint", { count: app.manage.state.view.limits.note })),
    h("textarea", { id: `note-${item.id}`, rows: "3", value: draft.note, maxlength: String(app.manage.state.view.limits.note), disabled: locked, "aria-describedby": `note-hint-${item.id}`, on: { input: (e) => app.manage.edit(item.id, "note", e.target.value) } }),
    h("p", null, h("button", { type: "submit", class: "btn btn-primary", id: `save-${item.id}`, disabled: locked }, t("manage.save"))),
  );
}

export function moduleRow(app, item, index, count, locked) {
  const { t, fmt } = app;
  const titleId = `manage-${item.id}`;
  const name = shownTitle(item, app.lang);
  const status = statusLine(item);
  const failed = app.manage.state.failed;
  return h(
    "li",
    { class: `card manage-item${item.hidden ? " is-hidden" : ""}`, "aria-labelledby": titleId },
    h(
      "div",
      { class: "manage-head" },
      h("h3", { id: titleId }, name, name !== item.title ? h("span", { class: "muted own-name" }, ` (${item.title})`) : null),
      h("span", { class: `chip ${item.hidden ? "chip-hidden" : "chip-shown"}` }, t(item.hidden ? "manage.state.hidden" : "manage.state.shown")),
    ),
    status ? h("p", { class: "muted small" }, t(status.key, { time: fmt.dateTime(status.at) })) : null,
    h(
      "div",
      { class: "manage-controls" },
      h(
        "div",
        { class: "switch" },
        h("input", { type: "checkbox", role: "switch", id: `hide-${item.id}`, checked: item.hidden, disabled: locked, "aria-describedby": `cosmetic-${item.id}`, on: { change: (e) => void app.manage.toggle(item.id, e.target.checked) } }),
        h("label", { for: `hide-${item.id}` }, t("manage.hide")),
      ),
      orderButtons(app, item, index, count, locked),
    ),
    h("p", { class: "note", id: `cosmetic-${item.id}` }, t(panelKey(item), { panel: item.panel ?? "" })),
    h("details", { class: "disable" }, h("summary", null, t("manage.disable.title")), h("p", null, item.disable ?? t("manage.disable.none"))),
    h("details", { class: "edit-box", open: Boolean(app.manage.state.open[item.id]), on: { toggle: (e) => app.manage.setOpen(item.id, e.target.open) } }, h("summary", null, t("manage.edit")), editForm(app, item, locked)),
    item.note ? h("p", { class: "admin-note" }, h("strong", null, t("manage.note"), ": "), item.note) : null,
    item.updatedAt ? h("p", { class: "muted small" }, t("manage.updated", { who: whoKey(item.updatedBy) ? t(whoKey(item.updatedBy)) : item.updatedBy, time: fmt.dateTime(item.updatedAt) })) : null,
    failed?.where === item.id ? failure(app, failed.error) : null,
  );
}

export function manageView(app) {
  const { t } = app;
  const st = app.manage.state;
  if (!st.view && st.error) {
    return [head(app), st.error.code === "forbidden" && st.error.status === 403 ? h("div", { class: "callout critical", role: "alert" }, h("p", null, t("manage.forbidden"))) : errorBlock(app, st.error, () => void app.manage.load())];
  }
  if (!st.view) return [head(app), loading(app)];
  const v = st.view;
  const why = readOnlyKey(v);
  const locked = !v.writable;
  return [
    head(app),
    why ? h("div", { class: "callout sev-warning", role: "status" }, h("p", { class: "callout-title" }, t("manage.readonly.title")), h("p", null, t(why))) : null,
    h(
      "section",
      { "aria-labelledby": "manage-list-title", "aria-busy": st.busy ? "true" : "false" },
      h("h2", { id: "manage-list-title" }, t("manage.list.title")),
      h("p", { class: "note" }, t("manage.cosmetic")),
      v.items.length ? h("ol", { class: "manage-list" }, v.items.map((item, i) => moduleRow(app, item, i, v.items.length, locked))) : h("p", { class: "callout" }, t("manage.empty")),
    ),
    availableSection(app, v.available),
    historySection(app, v),
    changesSection(app, v),
    transferSection(app, v, st),
  ];
}
