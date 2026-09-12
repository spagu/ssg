// GET /api/shop/admin/orders — the order list: filtered, sorted and paged.
//
// Paging is by cursor, not OFFSET. An offset page shifts under you as orders
// arrive: page two skips the row that page one pushed down, and shows another
// twice. The cursor is the last row's sort value **and** its id, because two
// orders can share a timestamp — or, when sorting by total, an amount — and a
// cursor that cannot break that tie loses rows.
//
// Sorting therefore changes the cursor as well as the ORDER BY, which is why
// the two are described together in one table below rather than assembled from
// two query parameters at the point of use.

import type { Env } from "../../_env";
import { fail, json, str } from "../../_lib";
import type { OrderRow } from "../../_types";
import type { AdminData } from "../_middleware";

const STATUSES = new Set([
  "pending",
  "paid",
  "fulfilled",
  "refunded",
  "cancelled",
  "needs_review",
  "failed",
]);

/** What may be sorted on, and the column each name means.
 *
 *  A whitelist, not a passthrough: the value ends up inside the SQL text, and
 *  the only safe way to put a caller's string there is not to. */
const SORTS: Record<string, { column: string; label: string }> = {
  date: { column: "o.created_at", label: "when it was placed" },
  paid: { column: "o.paid_at", label: "when it was paid" },
  total: { column: "o.total_minor", label: "what it came to" },
  number: { column: "o.number", label: "its number" },
  status: { column: "o.status", label: "its status" },
};

const DEFAULT_SORT = "date";
const DEFAULT_DIR = "desc";

/** The value a row contributes to the cursor, for the column being sorted on. */
function cursorValue(row: OrderRow, sort: string): string | number | null {
  switch (sort) {
    case "paid":
      return row.paid_at;
    case "total":
      return row.total_minor;
    case "number":
      return row.number;
    case "status":
      return row.status;
    default:
      return row.created_at;
  }
}

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env }) => {
  const url = new URL(request.url);
  const q = url.searchParams;

  const limit = Math.min(100, Math.max(1, Number.parseInt(q.get("limit") ?? "25", 10) || 25));
  const cursor = str(q.get("cursor"), 120);
  const status = str(q.get("status"), 20);
  const search = str(q.get("q"), 254).toLowerCase();
  const gateway = str(q.get("gateway"), 20).toLowerCase();
  const from = str(q.get("from"), 40);
  const to = str(q.get("to"), 40);
  const sort = str(q.get("sort"), 20) || DEFAULT_SORT;
  const dir = str(q.get("dir"), 4).toLowerCase() === "asc" ? "asc" : DEFAULT_DIR;

  if (status && !STATUSES.has(status)) {
    return fail(request, "invalid_status", "That is not an order status.", 422);
  }
  if (!(sort in SORTS)) {
    return fail(request, "invalid_sort", `Sort by one of: ${Object.keys(SORTS).join(", ")}.`, 422);
  }
  if ((from && Number.isNaN(Date.parse(from))) || (to && Number.isNaN(Date.parse(to)))) {
    return fail(request, "invalid_range", "from and to must be dates (YYYY-MM-DD).", 422);
  }

  // ── What is being looked at ────────────────────────────────────────────────
  const where: string[] = [];
  const binds: unknown[] = [];

  if (status) {
    where.push("o.status = ?");
    binds.push(status);
  }
  if (gateway) {
    where.push("LOWER(o.gateway) = ?");
    binds.push(gateway);
  }
  if (search) {
    // An order number or an email, whichever the owner pasted into the box.
    where.push("(LOWER(o.number) = ? OR c.email_lc LIKE ?)");
    binds.push(search, `%${search}%`);
  }
  if (from) {
    where.push("o.created_at >= ?");
    binds.push(from);
  }
  if (to) {
    // A date alone means the end of that day: "up to the 31st" includes it.
    where.push("o.created_at <= ?");
    binds.push(to.length === 10 ? `${to}T23:59:59.999Z` : to);
  }

  // The filter, without the cursor: the count has to describe the whole
  // selection, not the page.
  const filterSql = where.length ? `WHERE ${where.join(" AND ")}` : "";
  const filterBinds = [...binds];

  // ── Where in it ───────────────────────────────────────────────────────────
  const column = SORTS[sort]!.column;
  const comparison = dir === "asc" ? ">" : "<";
  if (cursor) {
    const at = cursor.indexOf("|");
    const [value, id] = at === -1 ? [cursor, ""] : [cursor.slice(0, at), cursor.slice(at + 1)];
    // Numbers must be compared as numbers: as text, 9 sorts after 10.
    const typed: string | number = sort === "total" ? Number(value) || 0 : value;
    where.push(`(${column} ${comparison} ? OR (${column} = ? AND o.id ${comparison} ?))`);
    binds.push(typed, typed, id);
  }

  // A NULL never satisfies a comparison, so sorting by a column that has them —
  // paid_at, on an unpaid order — would drop those rows the moment a cursor is
  // involved. They sort to the end and stay in the list.
  const nulls = column === "o.paid_at" ? `${column} IS NULL, ` : "";
  const sql = `SELECT o.*, c.email AS customer_email, c.name AS customer_name, c.country AS customer_country
                 FROM orders o
                 LEFT JOIN customers c ON c.id = o.customer_id
                ${where.length ? `WHERE ${where.join(" AND ")}` : ""}
                ORDER BY ${nulls}${column} ${dir.toUpperCase()}, o.id ${dir.toUpperCase()}
                LIMIT ?`;

  type Row = OrderRow & {
    customer_email: string | null;
    customer_name: string | null;
    customer_country: string | null;
  };

  // One row over the limit says whether there is another page; the count says
  // how big the whole thing is, which is what a reader needs to know whether
  // paging through it is worth starting.
  const [listed, counted] = await Promise.all([
    env.SHOP_DB.prepare(sql).bind(...binds, limit + 1).all<Row>(),
    env.SHOP_DB.prepare(
      `SELECT COUNT(*) AS n FROM orders o LEFT JOIN customers c ON c.id = o.customer_id ${filterSql}`,
    )
      .bind(...filterBinds)
      .first<{ n: number }>(),
  ]);

  const rows = listed.results ?? [];
  const page = rows.slice(0, limit);
  const last = page.at(-1);
  const more = rows.length > limit && last;

  return json({
    orders: page.map((o) => ({
      id: o.id,
      number: o.number,
      status: o.status,
      currency: o.currency,
      totalMinor: o.total_minor,
      taxMinor: o.tax_minor,
      taxCountry: o.tax_country,
      reverseCharge: o.reverse_charge === 1,
      gateway: o.gateway,
      email: o.customer_email,
      name: o.customer_name,
      country: o.customer_country,
      createdAt: o.created_at,
      paidAt: o.paid_at,
    })),
    total: counted?.n ?? 0,
    sort,
    dir,
    nextCursor: more ? `${cursorValue(last, sort) ?? ""}|${last.id}` : null,
  });
};
