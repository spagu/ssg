// One theme file: a text editor with line numbers and search, an image with
// "Replace", or a description of a font. Edits are drafts (models/
// theme-files.js) until the publish dialog sends them; History shows the
// file's revisions, each with its difference to the text here and "Restore".

import { h } from "../core/vnode.js";
import { base64Of, dataUrl, extOf, findAll, isImage, lineNumbers, nextMatch } from "../models/theme-files.js";
import { openDialog } from "../panel/dialog.js";
import { diffView } from "../panel/diff-view.js";
import { revisionsPanel } from "../panel/revisions.js";
import { btn, busy, slot } from "../panel/ui.js";

export async function fileEditor(app, file) {
  const { t } = app;
  const got = await app.readFile(file.path);
  const side = slot();
  const head = h("div", { class: "pane-head" }, h("h2", { id: "file-title" }, h("code", null, file.path)), btn(t("rev.open"), (e) => toggleHistory(app, file, side, e.currentTarget, got), { "aria-expanded": "false" }));
  let body;
  // An SVG is text and a picture: shown, replaceable, and editable as text.
  if (got.encoding === "text") body = [isImage(file.path) ? imageEditor(app, file, null, got.text) : null, textEditor(app, file, got.text)];
  else if (isImage(file.path)) body = imageEditor(app, file, got.base64);
  else body = h("p", { class: "muted" }, t("files.binary", { ext: extOf(file.path) }));
  return h("section", { class: "card file-pane", "aria-labelledby": "file-title" }, head, body, side.node);
}

function textEditor(app, file, original) {
  const { t } = app;
  const canEdit = app.caps.theme;
  const ref = { area: null, gutter: null, count: null };
  const state = { query: "", matches: [] };
  const value = () => app.themeDrafts.get(file.path)?.text ?? original;
  const sync = () => (ref.gutter.textContent = lineNumbers(ref.area.value));
  const find = (dir) => {
    state.matches = findAll(ref.area.value, state.query);
    const i = nextMatch(state.matches, dir > 0 ? ref.area.selectionEnd : ref.area.selectionStart, dir);
    ref.count.textContent = state.query ? (i < 0 ? t("editor.noMatch") : t("editor.match", { n: i + 1, count: state.matches.length, line: state.matches[i].line })) : "";
    if (i < 0) return;
    ref.area.focus();
    ref.area.setSelectionRange(state.matches[i].start, state.matches[i].end);
    // Bring the line into view: scrollTop from the line's position.
    const lh = parseFloat(getComputedStyle(ref.area).lineHeight) || 20;
    ref.area.scrollTop = Math.max(0, (state.matches[i].line - 3) * lh);
  };
  return h(
    "div",
    { class: "editor" },
    h(
      "div",
      { class: "editor-bar", role: "search" },
      h("label", { for: "ed-find" }, t("editor.find")),
      h("input", {
        type: "search", id: "ed-find", "aria-describedby": "ed-count",
        on: {
          input: (e) => ((state.query = e.target.value), (ref.count.textContent = "")),
          keydown: (e) => {
            if (e.key !== "Enter") return;
            e.preventDefault();
            find(e.shiftKey ? -1 : 1);
          },
        },
      }),
      btn(t("editor.prev"), () => find(-1), { variant: "quiet" }),
      btn(t("editor.next"), () => find(1), { variant: "quiet" }),
      h("span", { id: "ed-count", class: "hint", "aria-live": "polite", ref: (el) => (ref.count = el) }),
    ),
    h(
      "div",
      { class: "code-box" },
      h("pre", { class: "gutter", "aria-hidden": "true", ref: (el) => ((ref.gutter = el), (el.textContent = lineNumbers(value()))) }),
      h("textarea", {
        class: "code", id: "ed-text", "aria-label": t("editor.label", { path: file.path }), wrap: "off", spellcheck: "false", readonly: !canEdit, value: value(), rows: "24",
        ref: (el) => (ref.area = el),
        on: {
          input: (e) => (sync(), app.themeDrafts.setText(file, original, e.target.value), app.changed()),
          scroll: (e) => (ref.gutter.scrollTop = e.target.scrollTop),
        },
      }),
    ),
    canEdit ? null : h("p", { class: "note" }, t("editor.readOnly")),
  );
}

// imageEditor: `original` is base64 (a binary image) or null with `text`
// (an SVG), whose replacement is a text draft like any edit in the editor.
function imageEditor(app, file, original, text = null) {
  const { t } = app;
  const img = { el: null };
  const asBase64 = (s) => base64Of(new TextEncoder().encode(s).buffer);
  const shown = () => (text === null ? app.themeDrafts.get(file.path)?.base64 ?? original : asBase64(app.themeDrafts.get(file.path)?.text ?? text));
  const replace = async (input) => {
    const f = input.files?.[0];
    if (!f) return;
    if (extOf(f.name) !== extOf(file.path)) return app.notify(t("files.wrongType", { ext: extOf(file.path) }), "critical");
    if (text === null) app.themeDrafts.setImage(file, base64Of(await f.arrayBuffer()));
    else app.themeDrafts.setText(file, text, await f.text());
    img.el.src = dataUrl(file.path, shown());
    const area = document.getElementById("ed-text");
    if (area && text !== null) (area.value = app.themeDrafts.get(file.path)?.text ?? text), area.dispatchEvent(new Event("input"));
    app.changed();
    app.notify(t("files.replaced", { path: file.path }));
  };
  return h(
    "div",
    { class: "image-edit" },
    h("img", { src: dataUrl(file.path, shown()), alt: t("files.imageAlt", { path: file.path }), class: "theme-image", ref: (el) => (img.el = el) }),
    app.caps.theme
      ? h("div", { class: "field" }, h("label", { for: "img-file" }, t("files.replace")), h("input", { type: "file", id: "img-file", accept: `.${extOf(file.path)}`, "aria-describedby": "img-hint", on: { change: (e) => void busy(app, null, () => replace(e.target)) } }), h("p", { id: "img-hint", class: "hint" }, t("files.replaceHint", { ext: extOf(file.path) })))
      : null,
  );
}

// toggleHistory shows the file's revisions beside the editor.
function toggleHistory(app, file, side, button, got) {
  const open = button.getAttribute("aria-expanded") === "true";
  button.setAttribute("aria-expanded", String(!open));
  if (open) return side.set(null);
  side.set(revisionsPanel(app, {
    title: app.t("rev.fileTitle", { path: file.path }),
    load: (cursor) => app.api.get("revisions", { path: file.path, cursor }),
    onView: (r) => void busy(app, null, () => viewFileRevision(app, file, r, got)),
  }));
}

async function viewFileRevision(app, file, rev, got) {
  const { t } = app;
  const old = await app.api.get(`revisions/${encodeURIComponent(rev.commitId)}`, { path: file.path });
  const text = got.encoding === "text" && old.encoding === "text";
  const now = app.themeDrafts.get(file.path)?.text ?? got.text;
  const content = [
    h("p", { class: "hint" }, t("rev.by", { author: rev.authorName || rev.authorEmail, date: app.fmt.dateTime(rev.date) }), " \u2014 ", rev.message.split("\n")[0]),
    text ? diffView(t, now, old.text, { label: t("rev.diffLabel") }) : h("img", { src: dataUrl(file.path, old.base64), alt: t("files.imageAlt", { path: file.path }), class: "theme-image" }),
  ];
  const actions = [{ label: t("action.close"), value: null }];
  if (app.caps.theme) actions.push({ label: t("rev.restoreFile"), value: "restore", variant: "primary", submit: true });
  if ((await openDialog({ title: t("rev.fileAt", { commit: rev.commitId.slice(0, 7) }), content, actions, wide: true })) !== "restore") return;
  if (text) app.themeDrafts.setText(file, got.text, old.text);
  else app.themeDrafts.setImage(file, old.base64);
  app.changed();
  app.notify(t("rev.restoredFile", { path: file.path }));
  app.refresh();
}
