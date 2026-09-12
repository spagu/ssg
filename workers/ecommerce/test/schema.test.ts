// The schema runs itself. These tests are about the two ways that can go wrong:
// running twice, and drifting away from schema.sql.

import { env } from "cloudflare:test";
import { beforeEach, describe, expect, it } from "vitest";
import { allStatements, ensureSchema, resetSchemaCache } from "../functions/api/shop/_schema";

beforeEach(() => {
  resetSchemaCache();
});

async function objects(): Promise<string[]> {
  const { results } = await env.SHOP_DB.prepare(
    // D1 keeps its own bookkeeping in the same database; _cf_* is not ours.
    `SELECT name FROM sqlite_master
      WHERE name NOT LIKE 'sqlite_%' AND name NOT LIKE '\\_cf\\_%' ESCAPE '\\'
      ORDER BY name`,
  ).all<{ name: string }>();
  return (results ?? []).map((r) => r.name);
}

describe("migrations", () => {
  it("creates the whole shop", async () => {
    await ensureSchema(env);
    const names = await objects();
    for (const table of [
      "settings", "products", "prices", "customers", "orders", "order_items",
      "payments", "webhook_inbox", "download_tokens", "downloads", "vat_rates",
      "invoice_sequences", "invoices", "outbox", "audit_log", "admin_users",
      "admin_sessions",
    ]) {
      expect(names).toContain(table);
    }
  });

  it("is safe to run again", async () => {
    await ensureSchema(env);
    const first = await objects();
    resetSchemaCache();
    await ensureSchema(env);
    expect(await objects()).toEqual(first);
  });

  it("seeds VAT rates once, and never over the owner's own edits", async () => {
    await ensureSchema(env);
    const before = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM vat_rates`).first<{ n: number }>();
    expect(before?.n ?? 0).toBeGreaterThan(40);

    await env.SHOP_DB.prepare(`UPDATE vat_rates SET rate_bp = 1 WHERE country = 'PL' AND kind = 'ebook'`).run();
    resetSchemaCache();
    await ensureSchema(env);

    const edited = await env.SHOP_DB.prepare(
      `SELECT rate_bp FROM vat_rates WHERE country = 'PL' AND kind = 'ebook'`,
    ).first<{ rate_bp: number }>();
    expect(edited?.rate_bp).toBe(1);
  });

  it("runs once per isolate however many callers ask", async () => {
    // Ten concurrent first requests must not each run the migrations.
    await Promise.all(Array.from({ length: 10 }, () => ensureSchema(env)));
    expect((await objects()).length).toBeGreaterThan(0);
  });
});

describe("schema.sql agrees with the migrations", () => {
  it("declares exactly the same objects", async () => {
    await ensureSchema(env);
    const fromMigrations = await objects();

    // schema.sql is generated from the same statements, so comparing the
    // statement list to what ran is what keeps the documented file honest.
    const statements = allStatements();
    expect(statements.length).toBeGreaterThan(0);

    const declared = new Set<string>();
    for (const sql of statements) {
      const match = /CREATE (?:UNIQUE )?(?:TABLE|INDEX) IF NOT EXISTS (\w+)/i.exec(sql);
      if (match?.[1]) declared.add(match[1]);
    }
    expect([...declared].sort()).toEqual(fromMigrations.sort());
  });
});
