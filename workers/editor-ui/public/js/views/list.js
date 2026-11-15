// The article list: the site's Markdown files (GET api/content), newest
// first, with search and filters, the drafts of new articles kept in this
// browser, and "New article". Words in the path and the section are asked of
// the worker (they page); language and state filter what is loaded, on the
// page. An article with an unpublished draft here says so in words.

import { h, mount } from "../core/vnode.js";
import { STATES, filterItems, languagesOf, newDrafts, sectionLabel, sectionsOf, titleOfItem } from "../models/article-list.js";
import { titleOf } from "../models/article-doc.js";
import { btn, busy, emptyState, errorBlock, loading, pageHead, slot } from "../panel/ui.js";
import { newArticle } from "./new-article.js";

const select = (id, label, value, options, onChange) =>
  h("div", { class: "field" }, h("label", { for: id }, label), h("select", { id, on: { change: (e) => onChange(e.target.value) } }, options.map(([v, text]) => h("option", { value: v, selected: v === value }, text))));

export default async function list(app, view, route) {
  const { t } = app;
  const q = route.query.q ?? "";
  const section = route.query.section ?? "";
  const filters = { language: route.query.lang ?? "", state: STATES.includes(route.query.show) ? route.query.show : "all" };
  const [site, drafts] = await Promise.all([app.loadSite(), app.store.listDrafts()]);
  const local = new Set(drafts.filter((d) => !d.isNew).map((d) => d.path));
  const items = [];
  let next = null;
  let pending = 0;
  const results = slot("results");
  const go = (query) => app.go(app.href("list", {}, { q, section, lang: filters.language, show: filters.state === "all" ? "" : filters.state, ...query }));

  async function load(cursor) {
    const page = await app.api.get("content", { q, section, cursor });
    items.push(...page.items);
    next = page.nextCursor;
    pending = page.pending;
  }
  const draw = () => results.set(table(app, filterItems(items, filters, local), { local, next, pending, filtered: !!(q || section || filters.language || filters.state !== "all"), more: (e) => void busy(app, e.currentTarget, async () => (await load(next), draw())) }));

  const sections = sectionsOf([], site.sections);
  if (section && !sections.includes(section)) sections.push(section);
  mount(view, [
    pageHead(t("list.title"), { lead: t("list.lead"), actions: app.caps.publish && sections.length ? btn(t("list.new"), () => void newArticle(app, site).catch((e) => app.notify(String(e?.message ?? e), "critical")), { variant: "primary" }) : null }),
    draftsBlock(app, newDrafts(drafts)),
    h("form", { class: "filters", role: "search", "aria-label": t("list.filters"), on: { submit: (e) => (e.preventDefault(), go({ q: e.target.elements.q.value.trim() })) } },
      h("div", { class: "field" }, h("label", { for: "f-q" }, t("list.search")), h("input", { id: "f-q", name: "q", type: "search", value: q, "aria-describedby": "f-q-hint" }), h("p", { id: "f-q-hint", class: "hint" }, t("list.searchHint"))),
      select("f-section", t("list.section"), section, [["", t("list.anySection")], ...sections.map((s) => [s, sectionLabel(s)])], (v) => go({ section: v })),
      select("f-lang", t("list.language"), filters.language, [["", t("list.anyLanguage")], ...languagesOf([], site.languages).map((l) => [l, l])], (v) => ((filters.language = v), replaceQuery(), draw())),
      select("f-show", t("list.show"), filters.state, STATES.map((s) => [s, t(`list.show.${s}`)]), (v) => ((filters.state = v), replaceQuery(), draw())),
      h("div", { class: "filter-actions" }, h("button", { type: "submit", class: "btn" }, t("list.searchGo")))),
    results.node,
  ]);
  // replaceQuery: the page-side filters in the address, without reloading.
  const replaceQuery = () => history.replaceState(null, "", app.href("list", {}, { q, section, lang: filters.language, show: filters.state === "all" ? "" : filters.state }));
  results.set(loading(app));
  try {
    await load(null);
    draw();
  } catch (e) {
    results.set(errorBlock(app, e, () => app.refresh()));
  }
  return null;
}

function draftsBlock(app, drafts) {
  const { t } = app;
  if (!drafts.length) return null;
  return h("section", { class: "card", "aria-labelledby": "drafts-title" },
    h("h2", { id: "drafts-title" }, t("list.newDrafts")),
    h("ul", { class: "plain draft-list" }, drafts.map((d) => h("li", null,
      h("a", { href: app.href("new", { key: d.key }) }, titleOf(d)), " ",
      h("span", { class: "hint" }, t("list.savedAt", { when: app.fmt.relative(d.savedAt), path: d.path }))))));
}

// dateText: a day as a day (UTC, as written), a time in the reader's zone,
// anything else as the article writes it.
const dateText = (app, d) => (!d ? "\u2014" : /^\d{4}-\d{2}-\d{2}$/.test(d) ? app.fmt.dayLong(d) : Number.isNaN(Date.parse(d)) ? String(d) : app.fmt.date(d));

function table(app, shown, { local, next, pending, filtered, more }) {
  const { t } = app;
  if (!shown.length && !next) {
    return filtered
      ? emptyState({ img: "no-results", title: t("list.noneFoundTitle"), body: t("list.noneFound") })
      : emptyState({ img: "no-articles", title: t("list.emptyTitle"), body: t(app.caps.publish ? "list.empty" : "list.emptyReader") });
  }
  return [
    h("div", { class: "table-scroll" }, h("table", { class: "data list" },
      h("caption", null, t("list.caption", { count: shown.length })),
      h("thead", null, h("tr", null, ["list.colTitle", "list.colSection", "list.colLanguage", "list.colDate", "list.colState"].map((k) => h("th", { scope: "col" }, t(k))))),
      h("tbody", null, shown.map((i) => h("tr", null,
        h("th", { scope: "row" }, h("a", { href: app.href("edit", { path: i.path }) }, titleOfItem(i)), i.loaded === false ? h("span", { class: "hint" }, " ", t("list.titleUnread")) : null, h("p", { class: "hint mono" }, i.path)),
        h("td", { "data-label": t("list.colSection") }, sectionLabel(i.section)),
        h("td", { "data-label": t("list.colLanguage") }, i.language ?? "\u2014"),
        h("td", { "data-label": t("list.colDate") }, i.date ? app.fmt.date(i.date) : "\u2014"),
        h("td", { "data-label": t("list.colState") },
          i.draft === true ? h("span", { class: "chip" }, t("list.siteDraft")) : h("span", { class: "chip chip-ok" }, t("list.published")),
          local.has(i.path) ? [" ", h("span", { class: "chip chip-local" }, t("list.localDraft"))] : null)))))),
    pending ? h("p", { class: "hint" }, t("list.pending", { count: pending })) : null,
    next ? btn(t("list.more"), more) : null,
  ];
}
