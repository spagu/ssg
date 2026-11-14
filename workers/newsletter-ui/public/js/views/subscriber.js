// One subscriber: the record, the consent they gave, what was sent to them,
// whether they are suppressed; edit tags and language; unsubscribe, re-send
// the confirmation, export (GDPR access) or erase (GDPR erasure).

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { tagsFrom } from "../models/lists.js";
import { btn, busy, chip, confirmDialog, errorBlock, loading, pageHead } from "./ui.js";

const dl = (pairs) => h("dl", { class: "facts" }, pairs.map(([k, v]) => [h("dt", null, k), h("dd", null, v)]));

function sendsTable(app, sends) {
  const { t, fmt } = app;
  if (!sends.length) return h("p", { class: "muted" }, t("sub.noSends"));
  return h(
    "div",
    { class: "table-scroll" },
    h(
      "table",
      { class: "data list" },
      h("caption", { id: "sends-caption" }, t("sub.sends")),
      h("thead", null, h("tr", null, ["col.campaign", "col.status", "col.attempts", "col.sentAt", "col.error"].map((k) => h("th", { scope: "col" }, t(k))))),
      h(
        "tbody",
        null,
        sends.map((s) =>
          h(
            "tr",
            null,
            h("th", { scope: "row" }, s.subject === null ? t("sub.deletedCampaign") : h("a", { href: href("campaign", { id: s.campaignId }) }, s.subject)),
            h("td", { "data-label": t("col.status") }, chip(app, s.status, "send")),
            h("td", { "data-label": t("col.attempts"), class: "num" }, fmt.number(s.attempts)),
            h("td", { "data-label": t("col.sentAt") }, fmt.dateTime(s.sentAt ?? s.claimedAt)),
            h("td", { "data-label": t("col.error") }, s.error ?? "—"),
          ),
        ),
      ),
    ),
  );
}

export default async function subscriber(app, root, route) {
  const { t, fmt, api } = app;
  const id = route.params.id;
  const back = { href: href("subscribers"), label: t("nav.subscribers") };
  mount(root, [pageHead(app, t("sub.loading"), { back }), loading(app)]);
  let s;
  try {
    s = await api.get(`subscribers/${encodeURIComponent(id)}`);
  } catch (e) {
    mount(root, [pageHead(app, t("sub.title"), { back }), errorBlock(app, e, () => subscriber(app, root, route))]);
    return;
  }
  // A reload that fails (network, a 5xx) is shown in the view with a Retry,
  // like the router does for a first load — never a silent unhandled rejection.
  const reload = () => subscriber(app, root, route).catch((e) => mount(root, errorBlock(app, e, reload)));

  const save = async (e) => {
    e.preventDefault();
    const form = e.currentTarget;
    const tags = tagsFrom(form.elements.tags.value);
    if (tags.error) return app.notify(t("sub.badTag", { tag: tags.error }), "critical");
    const language = form.elements.language.value.trim() || null;
    const ok = await busy(app, form.querySelector("[type=submit]"), () => api.patch(`subscribers/${encodeURIComponent(id)}`, { tags: tags.tags, language }));
    if (ok) app.notify(t("sub.saved", { email: s.email }));
  };
  const unsubscribe = async (e) => {
    if (!(await confirmDialog(app, { title: t("sub.unsub.title", { email: s.email }), body: t("sub.unsub.body", { email: s.email }), confirm: t("sub.unsub.go") }))) return;
    if (await busy(app, e.target, () => api.post(`subscribers/${encodeURIComponent(id)}/unsubscribe`))) {
      app.notify(t("sub.unsub.done", { email: s.email }));
      await reload();
    }
  };
  const resend = async (e) => {
    const r = await busy(app, e.target, () => api.post(`subscribers/${encodeURIComponent(id)}/confirmation`));
    if (r) app.notify(t("sub.resend.done", { email: s.email, transport: r.transport }));
  };
  const erase = async (e) => {
    if (!(await confirmDialog(app, { title: t("sub.erase.title", { email: s.email }), body: t("sub.erase.body", { email: s.email }), confirm: t("sub.erase.go") }))) return;
    if ((await busy(app, e.target, () => api.del(`subscribers/${encodeURIComponent(id)}`))) !== undefined) {
      app.notify(t("sub.erase.done", { email: s.email }));
      app.go(href("subscribers"));
    }
  };

  mount(root, [
    pageHead(app, s.email, {
      back,
      actions: [
        s.status === "pending" && app.can("resendConfirmation") ? btn(t("sub.resend"), resend) : null,
        s.status !== "unsubscribed" && app.can("unsubscribe") ? btn(t("sub.unsub"), unsubscribe) : null,
        app.can("exportPerson") ? h("a", { class: "btn", href: api.url(`subscribers/${encodeURIComponent(id)}/export`), download: "" }, t("sub.export")) : null,
        app.can("erase") ? btn(t("sub.erase"), erase, { variant: "danger" }) : null,
      ],
    }),
    s.suppression
      ? h(
          "div",
          { class: "callout warning" },
          h("p", { class: "callout-title" }, t("sub.suppressed")),
          h("p", null, t("sub.suppressed.body", { reason: t(`reason.${s.suppression.reason}`), since: fmt.dateTime(s.suppression.createdAt) })),
          h("a", { href: href("suppressions", {}, { q: s.email }) }, t("sub.suppressed.go")),
        )
      : null,
    h(
      "div",
      { class: "two-col" },
      h(
        "section",
        { class: "card", "aria-labelledby": "rec-title" },
        h("h2", { id: "rec-title" }, t("sub.record")),
        dl([
          [t("col.status"), chip(app, s.status)],
          [t("col.signedUp"), fmt.dateTime(s.createdAt)],
          [t("sub.confirmedAt"), fmt.dateTime(s.confirmedAt)],
          [t("sub.unsubscribedAt"), fmt.dateTime(s.unsubscribedAt)],
          [t("sub.confirmSentAt"), fmt.dateTime(s.confirmSentAt)],
          [t("sub.source"), s.sourcePage ?? "—"],
        ]),
        h("h3", null, t("sub.consent")),
        h("blockquote", { class: "consent" }, s.consentText),
        h("p", { class: "muted" }, t("sub.consentAt", { at: fmt.dateTime(s.consentAt) })),
      ),
      h(
        "section",
        { class: "card", "aria-labelledby": "edit-title" },
        h("h2", { id: "edit-title" }, t("sub.edit")),
        h(
          "form",
          { on: { submit: save } },
          h("div", { class: "field" }, h("label", { for: "s-tags" }, t("col.tags")), h("input", { id: "s-tags", name: "tags", value: s.tags.join(", "), "aria-describedby": "s-tags-hint", autocomplete: "off" }), h("p", { id: "s-tags-hint", class: "hint" }, t("sub.tags.hint"))),
          h("div", { class: "field" }, h("label", { for: "s-lang" }, t("col.language")), h("input", { id: "s-lang", name: "language", value: s.language ?? "", "aria-describedby": "s-lang-hint", autocomplete: "off" }), h("p", { id: "s-lang-hint", class: "hint" }, t("sub.lang.hint"))),
          app.can("editSubscriber") ? h("button", { type: "submit", class: "btn btn-primary" }, t("action.save")) : null,
        ),
      ),
    ),
    h("section", { class: "card", "aria-labelledby": "sends-title" }, h("h2", { id: "sends-title" }, t("sub.sends")), sendsTable(app, s.sends)),
  ]);
}
