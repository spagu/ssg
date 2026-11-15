// The schema runs itself. These tests are about the two ways that can go wrong:
// running twice, and drifting away from schema.sql.

import { env } from "./_env";
import { beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
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
      "invoice_sequences", "invoices", "outbox", "audit_log", "auth_attempts",
    ]) {
      assert.ok((names).includes(table));
    }
    // GO-155: the shop's own accounts and sessions are no longer created (an
    // existing install keeps its old tables; nothing is dropped).
    assert.ok(!names.includes("admin_users") && !names.includes("admin_sessions"));
  });

  it("is safe to run again", async () => {
    await ensureSchema(env);
    const first = await objects();
    resetSchemaCache();
    await ensureSchema(env);
    assert.deepEqual(await objects(), first);
  });

  it("seeds VAT rates once, and never over the owner's own edits", async () => {
    await ensureSchema(env);
    const before = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM vat_rates`).first<{ n: number }>();
    assert.ok((before?.n ?? 0) > 40);

    await env.SHOP_DB.prepare(`UPDATE vat_rates SET rate_bp = 1 WHERE country = 'PL' AND kind = 'ebook'`).run();
    resetSchemaCache();
    await ensureSchema(env);

    const edited = await env.SHOP_DB.prepare(
      `SELECT rate_bp FROM vat_rates WHERE country = 'PL' AND kind = 'ebook'`,
    ).first<{ rate_bp: number }>();
    assert.equal(edited?.rate_bp, 1);
  });

  it("runs once per isolate however many callers ask", async () => {
    // Ten concurrent first requests must not each run the migrations.
    await Promise.all(Array.from({ length: 10 }, () => ensureSchema(env)));
    assert.ok(((await objects()).length) > 0);
  });
});

describe("schema.sql agrees with the migrations", () => {
  it("declares exactly the same objects", async () => {
    await ensureSchema(env);
    const fromMigrations = await objects();

    // schema.sql is generated from the same statements, so comparing the
    // statement list to what ran is what keeps the documented file honest.
    const statements = allStatements();
    assert.ok((statements.length) > 0);

    const declared = new Set<string>();
    for (const sql of statements) {
      const match = /CREATE (?:UNIQUE )?(?:TABLE|INDEX) IF NOT EXISTS (\w+)/i.exec(sql);
      if (match?.[1]) declared.add(match[1]);
    }
    assert.deepEqual([...declared].sort(), fromMigrations.sort());
  });
});
