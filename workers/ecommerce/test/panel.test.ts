// The panel's logic that does not need a page: cursor paging and sorting
// (pager.js's Pager) and the two formatters every list uses (dom.js's money
// and when). Drawing — renderPager, bindSortableHeaders, el, notify — is DOM
// glue and is checked in a browser, not here.
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { Pager } from "../public/ecommerce-admin/pager.js";
import { money, when } from "../public/ecommerce-admin/dom.js";

/** A list of `rows` names served `size` at a time, cursor = last name seen. */
function served(rows: string[]) {
  const asked: Record<string, string>[] = [];
  const fetchPage = async (params: URLSearchParams) => {
    asked.push(Object.fromEntries(params));
    const size = Number(params.get("limit"));
    const after = params.get("cursor");
    const start = after ? rows.indexOf(after) + 1 : 0;
    const page = rows.slice(start, start + size);
    const more = start + size < rows.length;
    return { total: rows.length, count: page.length, nextCursor: more ? page[page.length - 1] : null };
  };
  return { asked, fetchPage };
}

describe("paging through a list", () => {
  it("walks forward and back to exactly the page it left", async () => {
    const api = served(["a", "b", "c", "d", "e"]);
    const pager = new Pager({ fetchPage: api.fetchPage, pageSize: 2 });
    await pager.reset();
    assert.equal(pager.page, 1);
    assert.equal(pager.total, 5);

    await pager.next();
    await pager.next();
    assert.equal(pager.page, 3);
    assert.equal(pager.count, 1);
    assert.equal(await pager.next(), null, "no page after the last one");

    await pager.previous();
    assert.equal(pager.page, 2);
    assert.equal(api.asked.at(-1)?.cursor, "b");
    await pager.previous();
    assert.equal(pager.page, 1);
    assert.equal(api.asked.at(-1)?.cursor, undefined);
    assert.equal(await pager.previous(), null, "no page before the first one");
  });

  it("sends only the filters that say something, and goes back to page one when they change", async () => {
    const api = served(["a", "b", "c"]);
    const pager = new Pager({ fetchPage: api.fetchPage, pageSize: 1 });
    await pager.reset();
    await pager.next();
    await pager.reset({ status: "paid", q: "", from: null, to: undefined, n: 0 });
    assert.equal(pager.page, 1);
    assert.deepEqual(api.asked.at(-1), { limit: "1", status: "paid", n: "0" });
  });

  it("turns a column around, and starts another at its natural end", async () => {
    const api = served([]);
    const pager = new Pager({ fetchPage: api.fetchPage, sort: "created", dir: "desc" });
    await pager.sortBy("created");
    assert.equal(pager.dir, "asc");
    await pager.sortBy("created");
    assert.equal(pager.dir, "desc");
    await pager.sortBy("name", "asc");
    assert.deepEqual([pager.sort, pager.dir], ["name", "asc"]);
    assert.equal(api.asked.at(-1)?.sort, "name");
    await pager.sortBy("total");
    assert.equal(pager.dir, "desc");
  });

  it("reads an answer with no total or no count as unknown and empty", async () => {
    const pager = new Pager({ fetchPage: async () => ({ nextCursor: null }) });
    await pager.load();
    assert.equal(pager.total, null);
    assert.equal(pager.count, 0);
    const silent = new Pager({ fetchPage: async () => undefined as never });
    await silent.load();
    assert.equal(silent.nextCursor, null);
    assert.equal(silent.pageSize, 25);
    assert.equal(new URLSearchParams(silent.params()).has("sort"), false);
  });
});

describe("formatting for the lists", () => {
  it("shows money in the currency's own number of decimals", () => {
    assert.match(money(2400, "EUR"), /24[.,]00/);
    assert.doesNotMatch(money(1234, "JPY"), /[.,]\d{2}\b(?!\d)/);
    assert.match(money(1234, "KWD"), /1[.,]234/);
  });

  it("says nothing rather than something wrong when it cannot", () => {
    assert.equal(money(null, "EUR"), "—");
    assert.equal(money(undefined, "EUR"), "—");
    assert.equal(money(100, ""), "—");
    // A code Intl refuses still gets a readable amount.
    assert.equal(money(1234, "E1"), "12.34 E1");
  });

  it("shows a date in the reader's own format, and a dash for none", () => {
    const iso = "2026-11-14T10:00:00.000Z";
    assert.equal(when(iso), new Date(iso).toLocaleString());
    assert.equal(when(""), "—");
    assert.equal(when(null), "—");
    assert.equal(when("not a date"), "—");
  });
});
