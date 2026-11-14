// Invitations — the only way to a new account: the form (address, name,
// roles, groups, language, mail or a one-time link) and the invitations
// already sent, with Send again and Withdraw. A link is shown ONCE.

import { LANGS } from "../core/i18n.js";
import { h, mount } from "../core/vnode.js";
import { allows } from "../models/access.js";
import { act, btn, chip, confirmDialog, copyButton, empty, field, loading, pageHead, slot, tableRegion } from "./ui.js";

const TONE = { pending: "info", accepted: "ok", expired: "warning", revoked: "critical" };

export default async function invitations(app, view, route) {
  const { t, fmt } = app;
  mount(view, loading(app));
  const admin = allows(app.me, "chooseRoles");
  const [list, roles, groups] = await Promise.all([
    app.api.get("admin/invitations"),
    admin ? app.api.get("admin/roles") : { items: [] },
    admin ? app.api.get("admin/groups") : { items: [] },
  ]);
  const once = slot("once-slot");
  const table = slot();
  const all = route.query.all === "1";
  const mail = app.me.mail === "ready";

  function showOnce(email, link) {
    once.set(
      h("div", { class: "callout warning once", role: "alert" },
        h("p", { class: "callout-title" }, t("once.title")),
        h("p", null, t("once.invite", { email })),
        h("label", { for: "once-link" }, t("once.label")),
        h("input", { id: "once-link", type: "text", readonly: true, value: link, class: "mono" }),
        copyButton(app, link, t("action.copy"))),
    );
    document.getElementById("once-link")?.focus();
  }
  const sent = (r, email) => (r.delivery === "link" ? showOnce(email, r.link) : app.notify(t("invite.mailed", { email })));

  async function submit(e) {
    e.preventDefault();
    const f = new FormData(e.target);
    const email = String(f.get("email")).trim();
    const body = { email, name: String(f.get("name") ?? "").trim(), lang: String(f.get("lang")), delivery: String(f.get("delivery")), roleIds: f.getAll("roleIds").map(String), groupIds: f.getAll("groupIds").map(String) };
    const r = await act(app, e.submitter, () => app.api.post("admin/invitations", body));
    if (!r) return;
    e.target.reset();
    sent(r, email);
    await reloadList();
  }
  const checks = (name, legend, items, label) =>
    items.length ? h("fieldset", { class: "checks" }, h("legend", null, legend), items.map((x) => h("label", { class: "check" }, h("input", { type: "checkbox", name, value: x.id }), " ", label(x)))) : null;
  const form = h(
    "form",
    { class: "card form", "aria-labelledby": "invite-title", on: { submit: (e) => void submit(e) } },
    h("h2", { id: "invite-title" }, t("invite.title")),
    field("inv-email", t("invite.email"), h("input", { type: "email", name: "email", required: true, autocomplete: "off", spellcheck: "false" })),
    field("inv-name", t("invite.name"), h("input", { type: "text", name: "name", maxlength: "200", autocomplete: "off" })),
    field("inv-lang", t("invite.lang"), h("select", { name: "lang" }, LANGS.map((l) => h("option", { value: l, selected: l === app.lang, lang: l }, t(`pref.lang.${l}`)))), t("invite.langHint")),
    admin ? checks("roleIds", t("invite.roles"), roles.items, (r) => r.name) : h("p", { class: "hint" }, t("invite.noRoles")),
    admin ? checks("groupIds", t("invite.groups"), groups.items, (g) => g.name) : null,
    h("fieldset", { class: "checks" }, h("legend", null, t("invite.delivery")),
      h("label", { class: "check" }, h("input", { type: "radio", name: "delivery", value: "mail", checked: mail, disabled: !mail }), " ", t("invite.byMail")),
      h("label", { class: "check" }, h("input", { type: "radio", name: "delivery", value: "link", checked: !mail }), " ", t("invite.byLink")),
      mail ? null : h("p", { class: "hint" }, t("invite.noMail"))),
    h("p", { class: "hint" }, t("invite.escalation")),
    h("button", { type: "submit", class: "btn btn-primary" }, t("invite.go")),
  );

  async function resend(e, inv) {
    const delivery = mail ? "mail" : "link";
    if (!(await confirmDialog(app, { title: t("invite.resend.title", { email: inv.email }), body: t("invite.resend.body"), confirm: t("invite.resend.go"), danger: false }))) return;
    const r = await act(app, e.currentTarget, () => app.api.post(`admin/invitations/${inv.id}/resend`, { delivery }));
    if (r) {
      sent(r, inv.email);
      await reloadList();
    }
  }
  async function withdraw(e, inv) {
    if (!(await confirmDialog(app, { title: t("invite.revoke.title", { email: inv.email }), body: t("invite.revoke.body"), confirm: t("invite.revoke.go") }))) return;
    if ((await act(app, e.currentTarget, () => app.api.del(`admin/invitations/${inv.id}`))) !== undefined) {
      app.notify(t("invite.revoke.done", { email: inv.email }));
      await reloadList();
    }
  }
  function drawList(items) {
    const shownItems = all ? items : items.filter((i) => i.status === "pending");
    const rows = shownItems.map((i) =>
      h(
        "tr",
        null,
        h("th", { scope: "row" }, i.email, i.name ? h("span", { class: "sub" }, i.name) : null),
        h("td", null, chip(t(`invite.status.${i.status}`), TONE[i.status])),
        h("td", null, i.invitedByEmail ?? "—"),
        h("td", null, i.status === "pending" ? fmt.relative(i.expiresAt) : fmt.date(i.createdAt)),
        h("td", { class: "row-actions" }, i.status === "accepted" ? null : btn(t("invite.resend.button"), (e) => void resend(e, i), { "aria-label": t("invite.resend.label", { email: i.email }) }), i.status === "pending" ? btn(t("invite.revoke.button"), (e) => void withdraw(e, i), { "aria-label": t("invite.revoke.label", { email: i.email }) }) : null),
      ),
    );
    table.set([
      h("p", null, h("a", { href: all ? "#/invitations" : "#/invitations?all=1" }, all ? t("invite.showPending") : t("invite.showAll"))),
      rows.length ? tableRegion(app, t(all ? "invite.caption.all" : "invite.caption"), [t("invite.col.email"), t("invite.col.status"), t("invite.col.by"), t("invite.col.when"), t("invite.col.actions")], rows) : empty(t("invite.none")),
    ]);
  }
  async function reloadList() {
    drawList((await app.api.get("admin/invitations")).items);
  }

  mount(view, [pageHead(app, t("nav.invitations"), { lead: t("invite.lead") }), once.node, form, h("section", { class: "block", "aria-labelledby": "inv-list" }, h("h2", { id: "inv-list" }, t("invite.listTitle")), table.node)]);
  drawList(list.items);
}
