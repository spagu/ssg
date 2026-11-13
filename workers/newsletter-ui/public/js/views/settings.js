// Settings: what the worker reports about its own configuration, read-only.
// Configuration lives in wrangler.toml ([vars]) and in `wrangler secret put`;
// the panel cannot change it, and it never sees a secret's value — the SMTP
// password included: only whether one is set.

import { h, mount } from "../core/vnode.js";
import { busy, errorBlock, loading, pageHead } from "./ui.js";

const yes = (app, v) => h("span", { class: `flag ${v ? "on" : "off"}` }, app.t(v ? "set.yes" : "set.no"));
const onOff = (app, v) => h("span", { class: `flag ${v ? "on" : "off"}` }, app.t(v ? "set.on" : "set.off"));
const code = (v) => (v === null || v === undefined || v === "" ? "—" : h("code", null, String(v)));

function group(id, title, rows, note = null) {
  return h(
    "section",
    { class: "card", "aria-labelledby": id },
    h("h2", { id }, title),
    h("dl", { class: "facts" }, rows.map(([k, v]) => [h("dt", null, k), h("dd", null, v)])),
    note ? h("p", { class: "note" }, note) : null,
  );
}

function smtpBox(app, s) {
  const { t, api } = app;
  const out = { el: null };
  const run = async (e) => {
    const r = await busy(app, e.currentTarget, () => api.post("settings/smtp-test"));
    if (!r) return;
    mount(
      out.el,
      h(
        "div",
        { class: `callout ${r.ok ? "ok" : "critical"}`, role: "status" },
        h("p", { class: "callout-title" }, t(r.ok ? "smtp.ok" : "smtp.fail", { stage: r.stage })),
        h("p", null, t("smtp.tls", { tls: t(`smtp.tls.${r.tls}`) }), " · ", t(r.authenticated ? "smtp.auth.yes" : "smtp.auth.no")),
        r.detail ? h("p", null, r.detail) : null,
        r.reply ? h("details", null, h("summary", null, t("smtp.reply")), h("pre", { class: "mono", tabindex: "0" }, r.reply)) : null,
      ),
    );
    app.notify(t(r.ok ? "smtp.ok" : "smtp.fail", { stage: r.stage }), r.ok ? "ok" : "critical");
  };
  return h(
    "div",
    { class: "smtp-test" },
    h("p", null, t("smtp.lead")),
    h("button", { type: "button", class: "btn btn-primary", disabled: s.transport !== "smtp", on: { click: run } }, t("smtp.button")),
    s.transport !== "smtp" ? h("p", { class: "hint" }, t("smtp.onlySmtp")) : null,
    h("div", { ref: (el) => (out.el = el) }),
  );
}

export default async function settings(app, root) {
  const { t, api } = app;
  mount(root, [pageHead(app, t("set.title"), { lead: t("set.lead") }), loading(app)]);
  let s;
  try {
    s = await api.get("settings");
  } catch (e) {
    mount(root, [pageHead(app, t("set.title")), errorBlock(app, e, () => settings(app, root))]);
    return;
  }
  const smtp = s.smtp;
  mount(root, [
    pageHead(app, t("set.title"), { lead: t("set.lead") }),
    h(
      "div",
      { class: "settings-grid" },
      group("set-mail", t("set.mail"), [
        [t("set.transport"), s.transport ? h("strong", null, t(`transport.${s.transport}`)) : h("span", { class: "flag off bad" }, t("set.none"))],
        [t("set.sender"), s.sender.email ? `${s.sender.name ? s.sender.name + " " : ""}<${s.sender.email}>` : h("span", { class: "flag off bad" }, t("set.none"))],
        [t("set.site"), s.siteUrl ? h("span", null, code(s.siteUrl), " ", s.siteUrlValid ? null : h("span", { class: "flag off" }, t("set.invalid"))) : h("span", { class: "flag off bad" }, t("set.none"))],
        ["Resend", yes(app, s.keys.resend)],
        ["MailChannels", yes(app, s.keys.mailchannels)],
      ], t("set.mail.note")),
      h(
        "section",
        { class: "card", "aria-labelledby": "set-smtp" },
        h("h2", { id: "set-smtp" }, t("set.smtp")),
        smtp
          ? h("dl", { class: "facts" }, [
              [t("set.host"), code(smtp.host)],
              [t("set.port"), code(smtp.port)],
              [t("set.user"), code(smtp.user)],
              [t("set.password"), h("span", null, yes(app, smtp.passwordSet), " ", t("set.password.secret"))],
              [t("set.helo"), code(smtp.helo)],
            ].map(([k, v]) => [h("dt", null, k), h("dd", null, v)]))
          : h("p", null, t("set.smtp.none")),
        smtpBox(app, s),
      ),
      group("set-engine", t("set.engine"), [
        [t("set.rate"), t("set.rate.value", { count: s.engine.rate })],
        [t("set.batch"), app.fmt.number(s.engine.batchSize)],
        [t("set.stale"), t("set.minutes", { count: s.engine.staleMinutes })],
        [t("set.attempts"), app.fmt.number(s.engine.maxAttempts)],
        [t("set.queue"), onOff(app, s.engine.queue)],
        [t("set.dlq"), code(s.engine.deadLetterQueue)],
        [t("set.confirmBatch"), app.fmt.number(s.confirmBatch)],
      ]),
      group("set-stats", t("set.stats"), [
        [t("set.clicks"), onOff(app, s.clicks.tracking)],
        [t("set.window"), t("set.days", { count: s.clicks.unsubscribeWindowDays })],
        [t("set.softLimit"), app.fmt.number(s.bounces.softLimit)],
        [t("set.hookResend"), yes(app, s.webhooks.resend)],
        [t("set.hookMc"), yes(app, s.webhooks.mailchannels)],
        [t("set.feed"), code(s.feedUrl)],
        [t("set.importMax"), app.fmt.number(s.importMaxRows)],
      ]),
      group("set-worker", t("set.worker"), [
        [t("set.auth"), t(`set.auth.${s.auth}`)],
        [t("set.base"), code(s.base)],
        [t("set.schema"), code(s.schemaVersion)],
      ], t("set.worker.note")),
    ),
  ]);
}
