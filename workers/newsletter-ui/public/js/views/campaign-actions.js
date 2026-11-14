// The buttons a campaign offers in its current state (models/campaign.js
// decides which), and what each one asks before acting. Anything that starts
// mail or cannot be undone names the campaign and the consequence first.

import { href } from "../core/router.js";
import { h } from "../core/vnode.js";
import { LANGS } from "../core/i18n.js";
import { actionsFor, fromLocalInput, toLocalInput } from "../models/campaign.js";
import { btn, busy, confirmDialog } from "./ui.js";

const path = (c, action = "") => `campaigns/${encodeURIComponent(c.id)}${action ? "/" + action : ""}`;

export function campaignActions(app, c, { save = null } = {}) {
  const { t, fmt, api } = app;
  const name = c.subject || t("campaign.untitled");
  const reload = () => app.go(href("campaign", { id: c.id }));
  const ready = async () => (save ? save({ quiet: true }) : true);

  const move = (action, after = reload) => async (e) => {
    const r = await busy(app, e.currentTarget, () => api.post(path(c, action)));
    if (r) {
      app.notify(t(`act.${action}.done`, { subject: name }));
      after(r);
    }
  };

  const test = async (e) => {
    const button = e.currentTarget;
    if (!(await ready())) return;
    const refs = {};
    const extra = h(
      "div",
      { class: "dialog-fields" },
      h("div", { class: "field" }, h("label", { for: "test-to" }, t("test.to")), h("input", { id: "test-to", type: "email", autocomplete: "email", required: true, ref: (el) => (refs.to = el) })),
      h("div", { class: "field" }, h("label", { for: "test-lang" }, t("preview.language")), h("select", { id: "test-lang", ref: (el) => (refs.lang = el) }, LANGS.map((l) => h("option", { value: l }, t(`pref.lang.${l}`))))),
    );
    const ok = await confirmDialog(app, { title: t("test.title", { subject: name }), body: t("test.body"), confirm: t("test.go"), danger: false, extra, focus: "test-to" });
    if (!ok) return;
    const to = refs.to.value.trim();
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(to)) return app.notify(t("test.badTo"), "critical");
    const r = await busy(app, button, () => api.post(path(c, "test"), { to, language: refs.lang.value }));
    if (r) app.notify(t("test.done", { to, transport: r.transport }));
  };

  const schedule = async (e) => {
    const button = e.currentTarget;
    if (!(await ready())) return;
    const preview = await busy(app, button, () => api.post(path(c, "preview"), {}));
    if (!preview) return;
    const refs = {};
    const soon = toLocalInput(new Date(Date.now() + 3_600_000).toISOString());
    const extra = h(
      "fieldset",
      { class: "dialog-fields" },
      h("legend", null, t("schedule.when")),
      h("label", { class: "radio" }, h("input", { type: "radio", name: "when", value: "now", checked: true, ref: (el) => (refs.now = el) }), t("schedule.now")),
      h("label", { class: "radio" }, h("input", { type: "radio", name: "when", value: "at" }), t("schedule.at")),
      h("div", { class: "field" }, h("label", { for: "sched-at" }, t("schedule.time")), h("input", { id: "sched-at", type: "datetime-local", value: soon, ref: (el) => (refs.at = el), on: { input: () => (refs.now.checked = false) } })),
    );
    const ok = await confirmDialog(app, {
      title: t("schedule.title", { subject: name }),
      body: t("schedule.body", { count: preview.recipients.total, subject: name }),
      confirm: t("schedule.go", { count: preview.recipients.total }),
      danger: false,
      extra,
    });
    if (!ok) return;
    const at = refs.now.checked ? undefined : fromLocalInput(refs.at.value);
    if (!refs.now.checked && !at) return app.notify(t("schedule.badTime"), "critical");
    const r = await busy(app, button, () => api.post(path(c, "schedule"), at ? { at } : {}));
    if (r) {
      app.notify(t("schedule.done", { subject: name, count: r.recipients.total, at: fmt.dateTime(r.scheduledAt) }));
      reload();
    }
  };

  const confirmMove = (action) => async (e) => {
    const button = e.currentTarget;
    const vars = { subject: name, sent: fmt.number(c.sentCount ?? 0) };
    if (!(await confirmDialog(app, { title: t(`act.${action}.title`, vars), body: t(`act.${action}.body`, vars), confirm: t(`act.${action}.go`) }))) return;
    if (action === "delete") {
      if ((await busy(app, button, () => api.del(path(c)))) !== undefined) {
        app.notify(t("act.delete.done", vars));
        app.go(href("campaigns"));
      }
      return;
    }
    await move(action)({ currentTarget: button });
  };

  const handlers = {
    test,
    schedule,
    unschedule: move("unschedule"),
    pause: move("pause"),
    resume: move("resume"),
    cancel: confirmMove("cancel"),
    delete: confirmMove("delete"),
    duplicate: move("duplicate", (copy) => app.go(href("campaign", { id: copy.id }))),
  };
  return actionsFor(c.status)
    .filter((a) => a !== "save" && app.can(a))
    .map((a) => btn(t(`act.${a}`), handlers[a], { variant: a === "schedule" || a === "resume" ? "primary" : a === "delete" || a === "cancel" ? "danger" : "" }));
}
