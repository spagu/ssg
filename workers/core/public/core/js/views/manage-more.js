// The lower half of the Modules view (GO-149): templates not connected, the
// status history, the change log, and moving the settings between
// environments (download, then upload with a preview). Pure trees.

import { h } from "../vnode.js";
import { errorKey, reasonKey } from "../model.js";
import { changeKey, labelsText, previewKey, whoKey } from "../manage.js";

// failure: a refused or failed change, in words, where it happened.
export function failure(app, err) {
  const { t } = app;
  const key = err?.kind === "too_large" ? "manage.import.tooLarge" : errorKey(err);
  return h("div", { class: "callout critical", role: "alert" }, h("p", null, t(key, { seconds: err?.retryAfter ?? 60, detail: err?.detail ?? err?.code ?? "" })));
}

export function availableSection(app, available) {
  const { t } = app;
  return h(
    "section",
    { "aria-labelledby": "available-title" },
    h("h2", { id: "available-title" }, t("manage.available.title")),
    h("p", { class: "note" }, t("manage.available.lead")),
    available.length
      ? h(
          "ul",
          { class: "cards available" },
          available.map((a) =>
            h(
              "li",
              { class: "card", "aria-labelledby": `available-${a.id}` },
              h("h3", { id: `available-${a.id}` }, a.title),
              h("p", { class: "muted small" }, t(`manage.kind.${["pages-function", "worker"].includes(a.kind) ? a.kind : "other"}`)),
              h("p", null, t("manage.available.command")),
              h("pre", { class: "example" }, h("code", null, a.command)),
              h("p", null, t("manage.available.snippet")),
              h("pre", { class: "example" }, h("code", null, a.snippet)),
            ),
          ),
        )
      : h("p", null, t("manage.available.none")),
  );
}

const who = (app, actor) => (whoKey(actor) ? app.t(whoKey(actor)) : actor);

export function historySection(app, v) {
  const { t, fmt } = app;
  const title = (id) => v.items.find((i) => i.id === id)?.title ?? id;
  return h(
    "section",
    { "aria-labelledby": "history-title" },
    h("h2", { id: "history-title" }, t("manage.history.title")),
    h("p", { class: "note" }, t("manage.history.lead", { count: v.retention.historyDays })),
    v.history.length
      ? h(
          "ol",
          { class: "plain log" },
          v.history.map((r) =>
            h(
              "li",
              { class: r.status === "unreachable" ? "down" : "up" },
              r.status === "unreachable"
                ? t("manage.history.down", { time: fmt.dateTime(r.at), module: title(r.module), reason: t(reasonKey(r.reason), { status: "?", detail: "" }) })
                : t("manage.history.up", { time: fmt.dateTime(r.at), module: title(r.module) }),
            ),
          ),
        )
      : h("p", null, t("manage.history.none")),
  );
}

export function changesSection(app, v) {
  const { t, fmt } = app;
  const title = (id) => v.items.find((i) => i.id === id)?.title ?? id ?? "";
  return h(
    "section",
    { "aria-labelledby": "changes-title" },
    h("h2", { id: "changes-title" }, t("manage.changes.title")),
    h("p", { class: "note" }, t("manage.changes.lead", { count: v.retention.changesDays })),
    v.changes.length
      ? h(
          "ol",
          { class: "plain log" },
          v.changes.map((c) => h("li", null, t(changeKey(c), { time: fmt.dateTime(c.at), who: who(app, c.actor), module: title(c.module), changes: fmt.number(c.detail?.changes ?? 0) }))),
        )
      : h("p", null, t("manage.changes.none")),
  );
}

function previewBlock(app, v, preview) {
  const { t } = app;
  const title = (id) => v.items.find((i) => i.id === id)?.title ?? id;
  const line = (c) =>
    t(previewKey(c), { module: title(c.module), to: typeof c.to === "number" ? c.to + 1 : "", labels: labelsText(c.to) ?? "", note: typeof c.to === "string" ? c.to : "" });
  return h(
    "div",
    { class: "card preview", role: "region", "aria-labelledby": "preview-title" },
    h("h3", { id: "preview-title", tabindex: "-1" }, t("manage.import.preview")),
    preview.changes.length ? h("ul", { class: "plain" }, preview.changes.map((c) => h("li", null, line(c)))) : h("p", null, t("manage.import.nothing")),
    h(
      "p",
      { class: "toolbar" },
      preview.changes.length ? h("button", { type: "button", class: "btn btn-primary", id: "import-apply", on: { click: () => void app.manage.applyImport() } }, t("manage.import.apply")) : null,
      h("button", { type: "button", class: "btn", id: "import-cancel", on: { click: () => app.manage.cancelImport() } }, t("manage.import.cancel")),
    ),
  );
}

export function transferSection(app, v, st) {
  const { t } = app;
  const locked = !v.writable;
  return h(
    "section",
    { "aria-labelledby": "transfer-title" },
    h("h2", { id: "transfer-title" }, t("manage.transfer.title")),
    h("p", { class: "note" }, t("manage.transfer.lead")),
    locked
      ? h("p", { class: "muted" }, t("manage.transfer.locked"))
      : [
          h("p", null, h("a", { class: "btn", id: "export", href: "/api/core/manage/export", download: "core-settings.json" }, t("manage.export"))),
          h(
            "div",
            { class: "field" },
            h("label", { for: "import-file" }, t("manage.import.file")),
            h("input", { type: "file", id: "import-file", accept: "application/json,.json", on: { change: (e) => void app.manage.importFile(e.target.files?.[0] ?? null) } }),
          ),
          st.importError ? failure(app, st.importError) : null,
          st.failed?.where === "import" ? failure(app, st.failed.error) : null,
          st.preview ? previewBlock(app, v, st.preview) : null,
        ],
  );
}
