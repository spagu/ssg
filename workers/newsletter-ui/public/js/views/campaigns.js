// Campaigns: the list by status, a new draft, or a draft from a post on the
// site's feed.

import { href } from "../core/router.js";
import { h, mount } from "../core/vnode.js";
import { STATUSES } from "../models/campaign.js";
import { btn, busy, chip, confirmDialog, emptyState, errorBlock, errorText, loading, pageHead, slot } from "./ui.js";

function table(app, items) {
  const { t, fmt } = app;
  return h(
    "div",
    { class: "table-scroll", role: "region", tabindex: "0", "aria-labelledby": "camp-caption" },
    h(
      "table",
      { class: "data list" },
      h("caption", { id: "camp-caption" }, t("camps.caption", { count: items.length })),
      h("thead", null, h("tr", null, ["col.subject", "col.status", "col.recipients", "col.when", "col.updated"].map((k) => h("th", { scope: "col", class: k === "col.recipients" ? "num" : null }, t(k))))),
      h(
        "tbody",
        null,
        items.map((c) =>
          h(
            "tr",
            null,
            h("th", { scope: "row" }, h("a", { href: href("campaign", { id: c.id }) }, c.subject || t("campaign.untitled"))),
            h("td", { "data-label": t("col.status") }, chip(app, c.status)),
            h("td", { "data-label": t("col.recipients"), class: "num" }, c.totalCount ? t("camps.sentOf", { sent: fmt.number(c.sentCount), total: fmt.number(c.totalCount) }) : "—"),
            h("td", { "data-label": t("col.when") }, c.finishedAt ? t("camps.finished", { at: fmt.dateTime(c.finishedAt) }) : c.startedAt ? t("camps.started", { at: fmt.dateTime(c.startedAt) }) : c.scheduledAt ? t("camps.scheduledFor", { at: fmt.dateTime(c.scheduledAt) }) : "—"),
            h("td", { "data-label": t("col.updated") }, fmt.relative(c.updatedAt)),
          ),
        ),
      ),
    ),
  );
}

// postPicker lists the feed's posts; one click makes a draft from a post.
function postPicker(app, out) {
  const { t, fmt, api } = app;
  const make = async (post, button, confirm = false) => {
    try {
      button.disabled = true;
      const c = await api.post("campaigns/from-post", confirm ? { guid: post.guid, confirm: true } : { guid: post.guid });
      app.notify(t("posts.created", { title: post.title }));
      app.go(href("campaign", { id: c.id }));
    } catch (e) {
      button.disabled = false;
      if (e.code === "already_exists") {
        const again = await confirmDialog(app, { title: t("posts.again.title", { title: post.title }), body: t("posts.again.body"), confirm: t("posts.again.go"), danger: false });
        if (again) return make(post, button, true);
        return;
      }
      app.notify(errorText(app, e), "critical");
    }
  };
  return async (e) => {
    out.set(loading(app));
    const r = await busy(app, e.currentTarget, () => api.get("feed/posts"));
    if (!r) return out.set([]);
    out.set(
      h(
        "section",
        { class: "card", "aria-labelledby": "posts-title" },
        h("h2", { id: "posts-title", tabindex: "-1", ref: (el) => setTimeout(() => el.focus(), 0) }, t("posts.title")),
        h("p", { class: "muted" }, t("posts.lead", { feed: r.feedUrl })),
        r.items.length
          ? h(
              "ul",
              { class: "posts" },
              r.items.map((p) =>
                h(
                  "li",
                  null,
                  h("div", null, h("p", { class: "post-title" }, p.title), h("p", { class: "muted" }, [fmt.date(p.published), p.language].filter((x) => x && x !== "—").join(" · ")), p.campaign ? h("p", null, h("a", { href: href("campaign", { id: p.campaign.id }) }, t("posts.existing", { status: t(`status.${p.campaign.status}`) }))) : null),
                  h("button", { type: "button", class: "btn", "aria-label": t("posts.make.for", { title: p.title }), on: { click: (ev) => make(p, ev.currentTarget) } }, t("posts.make")),
                ),
              ),
            )
          : h("p", null, t("posts.none")),
      ),
    );
  };
}

export default async function campaigns(app, root, route) {
  const { t, api } = app;
  const status = STATUSES.includes(route.query.status) ? route.query.status : null;
  const list = slot();
  const picker = slot();
  const more = slot();
  mount(root, [
    pageHead(app, t("camps.title"), {
      lead: t("camps.lead"),
      actions: [h("a", { class: "btn btn-primary", href: href("campaignNew") }, t("camps.new")), btn(t("posts.button"), postPicker(app, picker))],
    }),
    picker.node,
    h(
      "nav",
      { class: "status-filter", "aria-label": t("camps.byStatus") },
      h("ul", null, [null, ...STATUSES].map((s) => h("li", null, h("a", { href: href("campaigns", {}, s ? { status: s } : {}), "aria-current": s === status ? "page" : null }, s ? t(`status.${s}`) : t("camps.all"))))),
    ),
    list.node,
    more.node,
  ]);
  list.set(loading(app));
  const items = [];
  let cursor = null;
  const page = async () => {
    const r = await api.get("campaigns", { status, cursor, limit: 50 });
    items.push(...r.items);
    cursor = r.nextCursor;
    list.set(
      items.length
        ? table(app, items)
        : emptyState({
            img: "empty-campaigns",
            title: t(status ? "camps.none.status" : "camps.none", { status: status ? t(`status.${status}`) : "" }),
            body: t("camps.none.body"),
            action: h("a", { class: "btn btn-primary", href: href("campaignNew") }, t("camps.new")),
          }),
    );
    more.set(cursor ? btn(t("action.loadMore"), (e) => busy(app, e.currentTarget, page)) : []);
  };
  try {
    await page();
  } catch (e) {
    list.set(errorBlock(app, e, () => campaigns(app, root, route)));
  }
}
