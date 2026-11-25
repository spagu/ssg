// The image library (#446): the stored images, newest first, in a dialog
// over the editor — insert one at the body's cursor (alt text asked again:
// it belongs to this use, not to the file), or delete one no campaign uses.
// Paged with nextCursor. While the dialog is open the rest of the page is
// inert, so its own live line announces what happened, not the page notice.

import { h } from "../core/vnode.js";
import { LIBRARY_PAGE, byteSize, dimensions, thumbSize } from "../models/assets.js";
import { askAlt, assetErrorText, insertSnippet } from "./asset-insert.js";
import { btn, confirmDialog, errorBlock, loading, panelDialog, slot } from "./ui.js";

export async function openLibrary(app, ctx) {
  const { t, fmt, api } = app;
  const list = slot("asset-list");
  const more = slot();
  const said = slot();
  const state = { items: [], cursor: null, done: false };
  const say = (message) => said.set(h("p", null, message));
  const dlg = panelDialog(app, {
    id: "lib",
    title: t("library.title"),
    content: [h("p", { class: "hint" }, t("library.lead")), h("div", { role: "status", "aria-live": "polite", class: "library-said" }, said.node), list.node, more.node],
  });

  const insert = async (a) => {
    const answer = await askAlt(app, { title: t("library.insert.title"), body: t("library.insert.body"), confirm: t("library.insert.go") });
    if (!answer) return;
    dlg.close();
    insertSnippet(ctx.body(), ctx.format(), answer.alt, a.url);
    app.notify(t("library.inserted"));
  };

  const remove = async (a) => {
    if (!(await confirmDialog(app, { title: t("library.delete.title"), body: t("library.delete.body"), confirm: t("library.delete.go") }))) return;
    try {
      await api.del(`assets/${encodeURIComponent(a.key)}`);
      state.items = state.items.filter((x) => x.key !== a.key);
      say(t("library.deleted"));
    } catch (e) {
      // 409: a campaign started using it since the list was loaded; 404: it
      // is gone already. Either way the list now says what is true.
      if (e.code === "asset_in_use") a.inUse = true;
      if (e.code === "not_found") state.items = state.items.filter((x) => x.key !== a.key);
      say(assetErrorText(app, e, ctx.assets));
    }
    draw();
    document.getElementById("lib-title")?.focus();
  };

  const failed = (e) => say(assetErrorText(app, e, ctx.assets));
  const item = (a) => {
    const dims = dimensions(a);
    const date = fmt.date(a.createdAt);
    const size = thumbSize(a);
    const metaId = `asset-${a.key.slice(0, 16)}`;
    return h(
      "li",
      { class: "asset" },
      h("img", { src: a.url, alt: dims ? t("library.thumbAlt", { dims, date }) : t("library.thumbAltNoDims", { date }), width: String(size.width), height: String(size.height), loading: "lazy", decoding: "async" }),
      h(
        "div",
        { class: "asset-meta", id: metaId },
        h("p", null, dims ?? t("library.noDims"), " · ", byteSize(a.bytes, app.lang)),
        h("p", null, t("library.uploaded", { date })),
        h("p", null, h("span", { class: `chip ${a.inUse ? "chip-inuse" : "chip-unused"}` }, t(a.inUse ? "library.inUse" : "library.notInUse"))),
      ),
      h(
        "div",
        { class: "asset-actions" },
        btn(t("library.insert"), () => {
          insert(a).catch(failed);
        }, { "aria-describedby": metaId }),
        a.inUse ? null : btn(t("library.delete"), () => {
          remove(a).catch(failed);
        }, { variant: "quiet", "aria-describedby": metaId }),
      ),
    );
  };

  function draw() {
    list.set(state.items.length ? h("ul", { class: "asset-grid", "aria-label": t("library.title") }, state.items.map(item)) : h("p", { class: "muted" }, t("library.empty")));
    more.set(state.done ? null : btn(t("library.more"), (e) => {
      load(e.currentTarget).catch(failed);
    }));
  }

  // load fetches the next page; after "Show more" focus moves to the first
  // new image's button, since the button itself may disappear.
  async function load(button = null) {
    const first = state.items.length;
    if (button) button.disabled = true;
    try {
      const page = await api.get("assets", { limit: LIBRARY_PAGE, cursor: state.cursor });
      state.items.push(...page.items);
      state.cursor = page.nextCursor;
      state.done = !page.nextCursor;
    } catch (e) {
      // A failed first page replaces the list; a failed "Show more" keeps
      // the images already shown and offers the retry in its place.
      (first ? more : list).set(errorBlock(app, e, () => {
        load().catch(failed);
      }));
      return;
    }
    draw();
    say(t("library.shown", { count: state.items.length }));
    if (button && state.items.length > first) list.el?.querySelectorAll(".asset")[first]?.querySelector("button")?.focus();
  }

  list.set(loading(app));
  await load();
}
