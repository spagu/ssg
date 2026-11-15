// The Insert list (GO-174): every widget the site offers — its typed
// components, its shortcodes, the built-in blocks it renders — in one
// dialog with a search field. Picking one inserts it at the caret: at once
// when there is nothing to ask, after a small form otherwise (a component's
// props, a table's size, a code block's language, a footnote's text, a video
// address). The list comes from GET api/widgets (src/widgets.ts) once per
// editor; a site whose components.json cannot be read still lists the rest,
// with a line saying why the components are missing.

import { h } from "../core/vnode.js";
import { component as insertCall, fenced, math, shortcode } from "../models/widget-blocks.js";
import { filterWidgets } from "../models/widget-call.js";
import { openDialog } from "../panel/dialog.js";
import { slot } from "../panel/ui.js";
import { blockForm, componentForm } from "./widget-forms.js";
import { openLibrary } from "./media-library.js";

const KINDS = ["component", "shortcode", "block"];

// titleOf: the name a person sees — blocks in the panel's language.
export const titleOf = (t, item) => (item.kind === "block" ? t(`insert.block.${item.name}`) : item.title);
const hintOf = (t, item) => (item.kind === "block" ? t(`insert.blockHint.${item.name}`) : item.description);

// pickWidget: the chosen widget, "image" for the media library, or null.
async function pickWidget(app, list, canLibrary) {
  const { t } = app;
  const state = { q: "", pick: null };
  const results = slot("insert-results");
  const shown = () => filterWidgets(list.items, state.q, (it) => `${titleOf(t, it)} ${hintOf(t, it)}`);
  const itemButton = (item) =>
    h("li", null, h("button", { type: "submit", class: "insert-item", on: { click: () => (state.pick = item) } },
      h("span", { class: "insert-name" }, titleOf(t, item)),
      item.kind === "component" ? h("code", { class: "insert-syntax" }, item.syntax) : null,
      hintOf(t, item) ? h("span", { class: "insert-hint" }, hintOf(t, item)) : null));
  const draw = () => {
    const items = shown();
    const groups = KINDS.map((kind) => {
      const of = items.filter((i) => i.kind === kind);
      const missing = kind === "component" && list.components.status !== "ok" && !state.q ? h("p", { class: "hint" }, t(`insert.components.${list.components.status}`)) : null;
      if (!of.length && !missing) return null;
      return h("section", { class: "insert-group", "aria-label": t(`insert.kind.${kind}`) }, h("h3", null, t(`insert.kind.${kind}`)), missing, of.length ? h("ul", { class: "insert-list" }, of.map(itemButton)) : null);
    });
    const libraryRow = canLibrary && (!state.q || filterWidgets([{ name: "image" }], state.q, () => `${t("insert.image")} ${t("insert.imageHint")}`).length)
      ? h("section", { class: "insert-group", "aria-label": t("insert.kind.media") }, h("h3", null, t("insert.kind.media")),
        h("ul", { class: "insert-list" }, itemButton({ kind: "image", name: "image", title: t("insert.image"), description: t("insert.imageHint") })))
      : null;
    const none = items.length || libraryRow ? null : h("p", { role: "status" }, t("insert.none", { q: state.q }));
    results.set([groups, libraryRow, none]);
  };
  const pending = openDialog({
    title: t("insert.title"),
    wide: true,
    content: [
      h("div", { class: "field" }, h("label", { for: "insert-q" }, t("insert.search")),
        h("input", { id: "insert-q", type: "search", autocomplete: "off", autofocus: true, "aria-describedby": "insert-q-hint", on: { input: (e) => ((state.q = e.target.value), draw()) } }),
        h("p", { id: "insert-q-hint", class: "hint" }, t("insert.searchHint"))),
      results.node,
    ],
    // Enter in the search field inserts the first match.
    actions: [{ label: t("action.cancel"), value: null }, { label: t("insert.first"), value: "go", variant: "primary", submit: true, check: () => !!(state.pick ??= shown()[0] ?? null) }],
  });
  draw(); // the dialog is in the page now: fill its list
  return (await pending) ? state.pick : null;
}

// insertWidget: open the list, then put the chosen widget at the caret.
export async function insertWidget(app, ed) {
  const { t } = app;
  ed.widgets ??= await app.api.get("widgets");
  const canLibrary = !!ed.site.media;
  const item = await pickWidget(app, ed.widgets, canLibrary);
  if (!item) return;
  if (item.kind === "image") return openLibrary(app, ed);
  if (item.kind === "shortcode") return ed.text.insert((text, sel) => shortcode(text, sel, item.name));
  if (item.kind === "component") {
    const call = item.props.length ? await componentForm(app, item) : item.syntax;
    if (call) ed.text.insert((text, sel) => insertCall(text, sel, call));
    return;
  }
  if (item.name === "mermaid") return ed.text.insert((text, sel) => fenced(text, sel, "mermaid", t("insert.sample.mermaid")));
  if (item.name === "math") return ed.text.insert((text, sel) => math(text, sel, t("insert.sample.math")));
  const fn = await blockForm(app, item.name);
  if (fn) ed.text.insert(fn);
}
