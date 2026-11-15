// The revisions side panel: the history of one file, newest first, from
// GET api/revisions — who, when, the commit message — with "View" on each
// and "Show older" while there are more. Generic: the caller passes how to
// load a page and what viewing one does; the words come from `app.t`
// (rev.title, rev.empty, rev.view, rev.more, rev.by); `app.fmt.dateTime`
// formats the date. No panel-specific import.

import { h } from "../core/vnode.js";
import { busy, errorBlock, loading, slot } from "./ui.js";

// revisionsPanel(app, {title, load(cursor) → {items, nextCursor}, onView(item)}) → vnode.
export function revisionsPanel(app, { title, load, onView }) {
  const list = slot("rev-list");
  const items = [];
  let next = null;
  const draw = () =>
    list.set([
      items.length
        ? h("ol", { class: "rev-items" }, items.map((r) =>
            h(
              "li",
              null,
              h("p", { class: "rev-msg" }, r.message.split("\n")[0]),
              h("p", { class: "hint" }, app.t("rev.by", { author: r.authorName || r.authorEmail, date: app.fmt.dateTime(r.date) })),
              h("button", { type: "button", class: "btn btn-quiet", "aria-label": app.t("rev.viewOf", { message: r.message.split("\n")[0] }), on: { click: () => onView(r) } }, app.t("rev.view")),
            ),
          ))
        : h("p", { class: "muted" }, app.t("rev.empty")),
      next ? h("button", { type: "button", class: "btn", on: { click: (e) => void busy(app, e.currentTarget, more) } }, app.t("rev.more")) : null,
    ]);
  async function more() {
    const page = await load(next);
    items.push(...page.items);
    next = page.nextCursor;
    draw();
  }
  queueMicrotask(() => {
    list.set(loading(app));
    load(null)
      .then((page) => {
        items.push(...page.items);
        next = page.nextCursor;
        draw();
      })
      .catch((e) => list.set(errorBlock(app, e, null)));
  });
  return h("aside", { class: "card revisions", "aria-label": title }, h("h2", null, title), list.node);
}
