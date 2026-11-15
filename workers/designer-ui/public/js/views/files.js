// The Files tab: the theme's files as a folder tree, and the one chosen in
// the editor (text), as a picture with "Replace" (images), or described
// (fonts). Reading needs designer-ui:read; changing, designer-ui:theme. A
// note on first visit says what a template is: the site's code.

import { h, mount } from "../core/vnode.js";
import { tree } from "../models/theme-files.js";
import { btn, emptyState, pageHead, slot } from "../panel/ui.js";
import { fileEditor } from "./editor.js";
import { themeUnavailable } from "./theme-unavailable.js";

const NOTE_KEY = "codeNoteSeen";

export default async function files(app, view, route) {
  const { t } = app;
  const theme = await app.loadTheme();
  if (!theme.available) return mount(view, [pageHead(t("files.title")), themeUnavailable(app, theme)]);
  const chosen = theme.files.find((f) => f.path === route.query.path) ?? null;
  const pane = slot("file-pane");
  const note = slot();
  const showNote = () =>
    note.set(
      app.prefs.read(NOTE_KEY)
        ? null
        : h(
            "div",
            { class: "callout warning", role: "note" },
            h("p", { class: "callout-title" }, t("files.codeTitle")),
            h("p", null, t("files.codeBody")),
            btn(t("files.codeOk"), () => (app.prefs.write(NOTE_KEY, "1"), showNote(), document.getElementById("view-title")?.focus())),
          ),
    );
  mount(view, [
    pageHead(t("files.title"), { lead: t("files.lead", { dir: theme.dir, count: theme.files.length }) }),
    note.node,
    theme.truncated ? h("p", { class: "callout warning" }, t("files.truncated")) : null,
    h("div", { class: "files-layout" }, h("nav", { class: "file-tree card", "aria-label": t("files.tree") }, treeView(app, tree(theme.files, theme.dir), chosen?.path ?? null, true)), pane.node),
  ]);
  showNote();
  if (!chosen) {
    pane.set(emptyState({ img: "pick-file", title: t("files.pickTitle"), body: t(route.query.path ? "files.gone" : "files.pick") }));
    return;
  }
  pane.set(await fileEditor(app, chosen));
}

// treeView: folders as disclosures (open where the chosen file is), files as
// links; an edited file is marked in words, not by colour alone.
function treeView(app, node, chosen, root = false) {
  const openHere = (n) => chosen !== null && chosen.startsWith(n.path + "/");
  const list = h(
    "ul",
    { class: root ? "tree" : "tree-sub" },
    node.dirs.map((d) => h("li", null, h("details", { open: root || openHere(d) }, h("summary", null, d.name, "/"), treeView(app, d, chosen)))),
    node.files.map((f) =>
      h(
        "li",
        null,
        h(
          "a",
          { href: app.href("files", {}, { path: f.path }), "aria-current": f.path === chosen ? "page" : null },
          f.name,
          app.themeDrafts.has(f.path) ? h("span", { class: "edited" }, " \u00b7 ", app.t("files.edited")) : null,
        ),
      ),
    ),
  );
  return list;
}
