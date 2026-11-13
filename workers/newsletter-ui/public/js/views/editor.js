// The campaign editor (drafts only — the worker refuses edits to anything
// else): subject, body, sender, segment, with the real rendered mail beside
// it. A draft saves itself a moment after typing stops, so the recipient
// count and the preview always describe what is stored.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { fieldsOf, payloadFrom, sameFields } from "../models/campaign.js";
import { LANGS } from "../core/i18n.js";
import { errorText, pageHead, slot } from "./ui.js";
import { previewPane } from "./preview.js";
import { campaignActions } from "./campaign-actions.js";

const AUTOSAVE_MS = 1200;

function readForm(form) {
  const v = (n) => form.elements[n].value;
  return {
    subject: v("subject"),
    body: v("body"),
    bodyFormat: form.elements.bodyFormat.value,
    textOverride: v("textOverride"),
    fromEmail: v("fromEmail"),
    fromName: v("fromName"),
    languages: v("languages"),
    tags: v("tags"),
    tagMode: form.elements.tagMode.value,
  };
}

const input = (name, label, value, attrs = {}, hint = null) =>
  h(
    "div",
    { class: "field" },
    h("label", { for: `c-${name}` }, label),
    h("input", { id: `c-${name}`, name, value, autocomplete: "off", ...(hint ? { "aria-describedby": `c-${name}-hint` } : {}), ...attrs }),
    hint ? h("p", { id: `c-${name}-hint`, class: "hint" }, hint) : null,
  );

export default async function editor(app, root, route, campaign = null) {
  const { t, api } = app;
  let c = campaign;
  let saved = fieldsOf(c ?? {});
  const status = slot("save-state");
  const preview = previewPane(app, () => c, LANGS);
  let timer = null;
  let formEl = null;
  app.onLeave(() => clearTimeout(timer));

  // save writes the form; a new campaign is created on its first save.
  const save = async ({ quiet = false } = {}) => {
    const f = readForm(formEl);
    const p = payloadFrom(f);
    if (p.error) {
      status.set(h("span", { class: "bad" }, t(p.error, { value: p.value })));
      return false;
    }
    if (c && sameFields(f, saved)) return true;
    try {
      if (!c) {
        c = await api.post("campaigns", p.body);
        saved = f;
        app.notify(t("editor.created", { subject: c.subject || t("campaign.untitled") }));
        app.go(href("campaign", { id: c.id }));
        return true;
      } else {
        c = await api.patch(`campaigns/${encodeURIComponent(c.id)}`, p.body);
        saved = f;
        if (!quiet) app.notify(t("editor.saved"));
      }
      status.set(t("editor.savedAt", { at: new Date().toLocaleTimeString(app.lang, { hour: "2-digit", minute: "2-digit" }) }));
      void preview.refresh(); // draws its own error state; a save is not undone by it
    } catch (e) {
      status.set(h("span", { class: "bad" }, errorText(app, e)));
      return false;
    }
  };
  const changed = () => {
    clearTimeout(timer);
    status.set(t("editor.unsaved"));
    if (c) timer = setTimeout(() => save({ quiet: true }), AUTOSAVE_MS);
  };

  const f = saved;
  const settings = await api.get("settings").catch(() => null);
  const defaults = settings?.sender ?? { email: null, name: null };
  const form = h(
    "form",
    { class: "editor-form", ref: (el) => (formEl = el), on: { input: changed, submit: (e) => (e.preventDefault(), save()) }, "aria-labelledby": "view-title" },
    input("subject", t("editor.subject"), f.subject, { required: true, maxlength: "250" }),
    h(
      "fieldset",
      { class: "segmented" },
      h("legend", null, t("editor.format")),
      ["markdown", "html"].map((v) => h("label", null, h("input", { type: "radio", name: "bodyFormat", value: v, checked: f.bodyFormat === v }), h("span", null, t(`editor.format.${v}`)))),
    ),
    h("div", { class: "field" }, h("label", { for: "c-body" }, t("editor.body")), h("textarea", { id: "c-body", name: "body", rows: "16", class: "code", "aria-describedby": "c-body-hint", spellcheck: "true" }, f.body), h("p", { id: "c-body-hint", class: "hint" }, t("editor.body.hint"))),
    h(
      "details",
      { open: !!f.textOverride },
      h("summary", null, t("editor.text")),
      h("div", { class: "field" }, h("label", { for: "c-textOverride" }, t("editor.text.label")), h("textarea", { id: "c-textOverride", name: "textOverride", rows: "8", class: "code", "aria-describedby": "c-text-hint" }, f.textOverride), h("p", { id: "c-text-hint", class: "hint" }, t("editor.text.hint"))),
    ),
    h(
      "fieldset",
      { class: "group" },
      h("legend", null, t("editor.sender")),
      input("fromEmail", t("editor.fromEmail"), f.fromEmail, { type: "email", placeholder: defaults.email ?? "news@example.com" }, defaults.email ? t("editor.fromEmail.default", { email: defaults.email }) : t("editor.fromEmail.none")),
      input("fromName", t("editor.fromName"), f.fromName, { placeholder: defaults.name ?? "" }),
    ),
    h(
      "fieldset",
      { class: "group" },
      h("legend", null, t("editor.segment")),
      h("p", { class: "hint" }, t("editor.segment.hint")),
      input("languages", t("editor.languages"), f.languages, { placeholder: "en, pl" }, t("editor.languages.hint")),
      input("tags", t("editor.tags"), f.tags, { placeholder: "news, offers" }, t("editor.tags.hint")),
      h(
        "fieldset",
        { class: "segmented" },
        h("legend", null, t("editor.tagMode")),
        ["any", "all"].map((v) => h("label", null, h("input", { type: "radio", name: "tagMode", value: v, checked: f.tagMode === v }), h("span", null, t(`editor.tagMode.${v}`)))),
      ),
      preview.count,
    ),
    h("div", { class: "form-actions" }, h("button", { type: "submit", class: "btn btn-primary" }, t(c ? "editor.save" : "editor.create")), status.node),
  );

  const actions = c ? campaignActions(app, c, { save }) : null;
  mount(root, [
    pageHead(app, c ? t("editor.title", { subject: c.subject || t("campaign.untitled") }) : t("editor.new"), {
      back: { href: href("campaigns"), label: t("nav.campaigns") },
      lead: c ? t("editor.lead") : t("editor.lead.new"),
      actions: c ? actions : null,
    }),
    h("div", { class: "editor" }, h("section", { class: "card", "aria-label": t("editor.form") }, form), preview.node),
  ]);
  status.set(c ? t("editor.allSaved") : t("editor.notYet"));
  if (c) void preview.refresh();
  else preview.empty();
}
