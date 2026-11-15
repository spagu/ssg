// Paging and sorting, once, for every list in the panel.
//
// The lists page by cursor rather than by offset, because an offset page shifts
// under you as rows arrive: page two skips the row page one pushed down and
// shows another twice. A cursor cannot do that — but it also cannot jump to
// page five, and it cannot go back on its own.
//
// Going back is what this remembers. Each page's starting cursor is pushed onto
// a stack as the reader moves forward, so "previous" is the cursor that started
// the page before, exactly. No arithmetic, no second guess at where the reader
// was.

import { el } from "./dom.js";

export class Pager {
  /**
   * @param {object} options
   * @param {(params: URLSearchParams) => Promise<{ total?: number, nextCursor: string|null }>} options.fetchPage
   *   Loads one page and renders it. Returns what the API said about the rest.
   * @param {number} [options.pageSize]
   * @param {string} [options.sort]  the column name the API understands
   * @param {string} [options.dir]   "asc" or "desc"
   */
  constructor({ fetchPage, pageSize = 25, sort = "", dir = "desc" }) {
    this.fetchPage = fetchPage;
    this.pageSize = pageSize;
    this.sort = sort;
    this.dir = dir;

    this.filters = {};
    this.cursor = null;
    this.nextCursor = null;
    this.total = null;
    this.stack = []; // the cursor that started each page behind this one
    this.count = 0; // rows on the page now showing
  }

  /** The page number a reader sees, counting from one. */
  get page() {
    return this.stack.length + 1;
  }

  /** Everything the API needs to answer for the current position. */
  params() {
    const params = new URLSearchParams({ limit: String(this.pageSize) });
    for (const [key, value] of Object.entries(this.filters)) {
      if (value !== "" && value !== null && value !== undefined) params.set(key, String(value));
    }
    if (this.sort) {
      params.set("sort", this.sort);
      params.set("dir", this.dir);
    }
    if (this.cursor) params.set("cursor", this.cursor);
    return params;
  }

  async load() {
    const result = await this.fetchPage(this.params());
    this.nextCursor = result?.nextCursor ?? null;
    this.total = typeof result?.total === "number" ? result.total : null;
    this.count = result?.count ?? 0;
    return result;
  }

  /** A new filter, a new sort, a new search: back to the first page, because
   *  a cursor from the old selection means nothing in the new one. */
  async reset(filters = this.filters) {
    this.filters = filters;
    this.cursor = null;
    this.stack = [];
    return this.load();
  }

  async next() {
    if (!this.nextCursor) return null;
    this.stack.push(this.cursor);
    this.cursor = this.nextCursor;
    return this.load();
  }

  async previous() {
    if (this.stack.length === 0) return null;
    this.cursor = this.stack.pop() ?? null;
    return this.load();
  }

  /** Sorting by the column already sorted on turns it around; by another,
   *  starts at that column's natural end — newest first for a date, largest
   *  first for an amount, A first for a name. */
  async sortBy(column, naturalDir = "desc") {
    if (this.sort === column) this.dir = this.dir === "asc" ? "desc" : "asc";
    else {
      this.sort = column;
      this.dir = naturalDir;
    }
    return this.reset();
  }
}

/** The footer under a list: where you are, and the two ways out of it. */
export function renderPager(host, pager, onChange) {
  const from = pager.count === 0 ? 0 : (pager.page - 1) * pager.pageSize + 1;
  const to = (pager.page - 1) * pager.pageSize + pager.count;

  const summary =
    pager.count === 0
      ? "Nothing to show"
      : pager.total === null
        ? `Showing ${from}–${to}`
        : `Showing ${from}–${to} of ${pager.total}`;

  const button = (label, enabled, go) =>
    el("button", {
      type: "button",
      class: "secondary small",
      disabled: !enabled,
      onClick: async () => {
        await go();
        onChange();
      },
    }, label);

  host.replaceChildren(
    el("p", { class: "muted", text: summary }),
    el("span", { class: "spacer" }),
    button("← Previous", pager.stack.length > 0, () => pager.previous()),
    el("span", { class: "muted pagenum", text: `Page ${pager.page}` }),
    button("Next →", Boolean(pager.nextCursor), () => pager.next()),
  );
  host.hidden = false;
}

/** Makes a table's header cells sort the list.
 *
 *  `aria-sort` on the header is what an assistive technology reads to say which
 *  column the table is ordered by and which way — the arrow beside the label is
 *  the same fact for everyone else. */
export function bindSortableHeaders(table, pager, onChange) {
  for (const th of table.querySelectorAll("th[data-sort]")) {
    const column = th.dataset.sort;
    const natural = th.dataset.sortDir ?? "desc";
    const label = th.dataset.label ?? th.textContent.trim();
    th.dataset.label = label;

    const active = pager.sort === column;
    th.setAttribute("aria-sort", active ? (pager.dir === "asc" ? "ascending" : "descending") : "none");

    th.replaceChildren(
      el("button", {
        type: "button",
        class: "sortbtn",
        onClick: async () => {
          await pager.sortBy(column, natural);
          onChange();
        },
      },
        label,
        el("span", { class: "arrow", "aria-hidden": "true", text: active ? (pager.dir === "asc" ? "↑" : "↓") : "↕" }),
      ),
    );
  }
}
