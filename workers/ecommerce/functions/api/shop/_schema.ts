// Schema, as numbered migrations that run themselves.
//
// Binding the D1 database is the only setup step: the worker brings its tables
// up to date on first use, once per isolate, exactly as the comments worker
// does — but versioned, because a shop changes its schema more often than a
// comment table does.
//
// Rules for every migration, so an old deployment keeps working against a new
// database (the project's "no breaking changes before 2.0"):
//   - only CREATE ... IF NOT EXISTS, ALTER TABLE ... ADD COLUMN, INSERT OR IGNORE
//   - never DROP, never RENAME, never a type change
//   - an ADD COLUMN that already exists is swallowed (SQLite has no IF NOT EXISTS there)
//
// schema.sql in this directory is the same thing written out for a human, and a
// test proves the two agree by comparing sqlite_master after migrating.

import type { Env } from "./_env";

/** Bump by appending a new array. Never edit a released migration: someone's
 *  database has already run it. */
const MIGRATIONS: string[][] = [
  // ── 1: the shop ──────────────────────────────────────────────────────────
  [
    `CREATE TABLE IF NOT EXISTS settings (
       key         TEXT PRIMARY KEY,
       value       TEXT NOT NULL,
       updated_at  TEXT NOT NULL
     )`,
    `CREATE TABLE IF NOT EXISTS products (
       id             TEXT PRIMARY KEY,
       sku            TEXT NOT NULL UNIQUE,
       name           TEXT NOT NULL,
       description    TEXT,
       kind           TEXT NOT NULL DEFAULT 'digital',
       status         TEXT NOT NULL DEFAULT 'draft',
       tax_category   TEXT NOT NULL DEFAULT 'ebook',
       file_key       TEXT,
       file_name      TEXT,
       file_size      INTEGER,
       file_sha256    TEXT,
       download_limit INTEGER NOT NULL DEFAULT 5,
       download_days  INTEGER NOT NULL DEFAULT 30,
       image_url      TEXT,
       created_at     TEXT NOT NULL,
       updated_at     TEXT NOT NULL
     )`,
    `CREATE INDEX IF NOT EXISTS idx_products_status ON products (status, sku)`,
    `CREATE TABLE IF NOT EXISTS prices (
       product_id   TEXT NOT NULL,
       currency     TEXT NOT NULL,
       amount_minor INTEGER NOT NULL CHECK (amount_minor >= 0),
       PRIMARY KEY (product_id, currency)
     )`,
    `CREATE TABLE IF NOT EXISTS customers (
       id           TEXT PRIMARY KEY,
       email        TEXT NOT NULL,
       email_lc     TEXT NOT NULL,
       name         TEXT,
       vat_id       TEXT,
       vat_id_valid INTEGER,
       country      TEXT,
       created_at   TEXT NOT NULL
     )`,
    `CREATE INDEX IF NOT EXISTS idx_customers_email ON customers (email_lc)`,
    `CREATE TABLE IF NOT EXISTS orders (
       id                TEXT PRIMARY KEY,
       number            TEXT NOT NULL UNIQUE,
       key_hash          TEXT NOT NULL,
       status            TEXT NOT NULL DEFAULT 'pending',
       customer_id       TEXT,
       currency          TEXT NOT NULL,
       subtotal_minor    INTEGER NOT NULL,
       tax_minor         INTEGER NOT NULL,
       total_minor       INTEGER NOT NULL,
       tax_country       TEXT,
       tax_evidence      TEXT,
       reverse_charge    INTEGER NOT NULL DEFAULT 0,
       gateway           TEXT,
       gateway_ref       TEXT,
       consent_marketing INTEGER NOT NULL DEFAULT 0,
       consent_waiver    INTEGER NOT NULL DEFAULT 0,
       ip_hash           TEXT,
       user_agent        TEXT,
       locale            TEXT,
       notes             TEXT,
       created_at        TEXT NOT NULL,
       paid_at           TEXT,
       updated_at        TEXT NOT NULL
     )`,
    `CREATE INDEX IF NOT EXISTS idx_orders_status_created ON orders (status, created_at)`,
    `CREATE INDEX IF NOT EXISTS idx_orders_customer ON orders (customer_id, created_at)`,
    `CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_gateway_ref ON orders (gateway, gateway_ref)`,
    `CREATE TABLE IF NOT EXISTS order_items (
       id           TEXT PRIMARY KEY,
       order_id     TEXT NOT NULL,
       product_id   TEXT NOT NULL,
       sku          TEXT NOT NULL,
       name         TEXT NOT NULL,
       quantity     INTEGER NOT NULL CHECK (quantity > 0),
       unit_minor   INTEGER NOT NULL,
       tax_rate_bp  INTEGER NOT NULL,
       tax_minor    INTEGER NOT NULL,
       total_minor  INTEGER NOT NULL
     )`,
    `CREATE INDEX IF NOT EXISTS idx_items_order ON order_items (order_id)`,
    `CREATE TABLE IF NOT EXISTS payments (
       id           TEXT PRIMARY KEY,
       order_id     TEXT NOT NULL,
       gateway      TEXT NOT NULL,
       gateway_ref  TEXT NOT NULL,
       kind         TEXT NOT NULL,
       status       TEXT NOT NULL,
       amount_minor INTEGER NOT NULL,
       currency     TEXT NOT NULL,
       raw_json     TEXT,
       created_at   TEXT NOT NULL
     )`,
    `CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_ref ON payments (gateway, gateway_ref, kind)`,
    `CREATE TABLE IF NOT EXISTS webhook_inbox (
       gateway      TEXT NOT NULL,
       event_id     TEXT NOT NULL,
       event_type   TEXT NOT NULL,
       received_at  TEXT NOT NULL,
       processed_at TEXT,
       result       TEXT,
       PRIMARY KEY (gateway, event_id)
     )`,
    `CREATE TABLE IF NOT EXISTS download_tokens (
       token_hash    TEXT PRIMARY KEY,
       order_item_id TEXT NOT NULL,
       expires_at    TEXT NOT NULL,
       uses          INTEGER NOT NULL DEFAULT 0,
       max_uses      INTEGER NOT NULL,
       revoked       INTEGER NOT NULL DEFAULT 0,
       created_at    TEXT NOT NULL
     )`,
    `CREATE INDEX IF NOT EXISTS idx_tokens_item ON download_tokens (order_item_id)`,
    `CREATE TABLE IF NOT EXISTS downloads (
       id         TEXT PRIMARY KEY,
       token_hash TEXT NOT NULL,
       ip_hash    TEXT,
       user_agent TEXT,
       bytes_sent INTEGER,
       created_at TEXT NOT NULL
     )`,
    `CREATE TABLE IF NOT EXISTS vat_rates (
       country    TEXT NOT NULL,
       kind       TEXT NOT NULL,
       rate_bp    INTEGER NOT NULL,
       valid_from TEXT NOT NULL,
       valid_to   TEXT,
       PRIMARY KEY (country, kind, valid_from)
     )`,
    `CREATE TABLE IF NOT EXISTS invoice_sequences (
       series TEXT NOT NULL,
       year   INTEGER NOT NULL,
       next   INTEGER NOT NULL DEFAULT 1,
       PRIMARY KEY (series, year)
     )`,
    `CREATE TABLE IF NOT EXISTS invoices (
       id            TEXT PRIMARY KEY,
       number        TEXT NOT NULL UNIQUE,
       kind          TEXT NOT NULL,
       order_id      TEXT NOT NULL,
       corrects_id   TEXT,
       issued_at     TEXT NOT NULL,
       currency      TEXT NOT NULL,
       total_minor   INTEGER NOT NULL,
       snapshot_json TEXT NOT NULL,
       pdf_key       TEXT,
       created_at    TEXT NOT NULL
     )`,
    `CREATE INDEX IF NOT EXISTS idx_invoices_order ON invoices (order_id)`,
    `CREATE TABLE IF NOT EXISTS outbox (
       id           TEXT PRIMARY KEY,
       kind         TEXT NOT NULL,
       target       TEXT NOT NULL,
       payload_json TEXT NOT NULL,
       attempts     INTEGER NOT NULL DEFAULT 0,
       next_attempt TEXT NOT NULL,
       last_error   TEXT,
       done_at      TEXT,
       created_at   TEXT NOT NULL
     )`,
    `CREATE INDEX IF NOT EXISTS idx_outbox_due ON outbox (done_at, next_attempt)`,
    `CREATE TABLE IF NOT EXISTS audit_log (
       id          TEXT PRIMARY KEY,
       actor       TEXT NOT NULL,
       action      TEXT NOT NULL,
       subject     TEXT,
       detail_json TEXT,
       created_at  TEXT NOT NULL
     )`,
    `CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log (created_at)`,
    `CREATE TABLE IF NOT EXISTS admin_users (
       id            TEXT PRIMARY KEY,
       email_lc      TEXT NOT NULL UNIQUE,
       pass_hash     TEXT NOT NULL,
       role          TEXT NOT NULL DEFAULT 'owner',
       created_at    TEXT NOT NULL,
       last_login_at TEXT
     )`,
    `CREATE TABLE IF NOT EXISTS admin_sessions (
       jti_hash     TEXT PRIMARY KEY,
       admin_id     TEXT NOT NULL,
       expires_at   TEXT NOT NULL,
       rotated_from TEXT,
       created_at   TEXT NOT NULL
     )`,
    `CREATE INDEX IF NOT EXISTS idx_sessions_admin ON admin_sessions (admin_id)`,
  ],
  // ── 2: refresh-token reuse detection ─────────────────────────────────────
  //
  // A rotated token used to be deleted, which made a replay indistinguishable
  // from an expired session: both simply said no, and the thief's own session
  // carried on. Keeping the spent row is what lets the second use be recognised
  // as a second use.
  [
    `ALTER TABLE admin_sessions ADD COLUMN used_at TEXT`,
    `CREATE INDEX IF NOT EXISTS idx_sessions_used ON admin_sessions (used_at)`,
  ],
  // ── 3: more than one person ──────────────────────────────────────────────
  //
  // A name, so the audit log reads as people rather than as ids, and a way to
  // suspend an account. Deleting one would orphan the entries that point at it,
  // and "who did this" is the question the log exists to answer.
  [
    `ALTER TABLE admin_users ADD COLUMN name TEXT`,
    `ALTER TABLE admin_users ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0`,
  ],
];

/** The EU + UK standard and ebook rates, seeded once so a new shop is not
 *  unusable on day one. Dated, because rates change and the shop must be able
 *  to say which rate applied when (see _tax).
 *
 *  These are a starting point, not tax advice: the panel shows a warning when
 *  the newest row is over a year old, and the seller's accountant has the last
 *  word. Ebook rates are the reduced ones several member states adopted after
 *  the 2018 directive allowed it. */
const VAT_SEED: Array<[string, string, number]> = [
  // country, kind, rate in basis points
  ["AT", "standard", 2000], ["AT", "ebook", 1000],
  ["BE", "standard", 2100], ["BE", "ebook", 600],
  ["BG", "standard", 2000], ["BG", "ebook", 900],
  ["HR", "standard", 2500], ["HR", "ebook", 500],
  ["CY", "standard", 1900], ["CY", "ebook", 400],
  ["CZ", "standard", 2100], ["CZ", "ebook", 1200],
  ["DK", "standard", 2500],
  ["EE", "standard", 2200], ["EE", "ebook", 900],
  ["FI", "standard", 2550], ["FI", "ebook", 1400],
  ["FR", "standard", 2000], ["FR", "ebook", 550],
  ["DE", "standard", 1900], ["DE", "ebook", 700],
  ["GR", "standard", 2400], ["GR", "ebook", 600],
  ["HU", "standard", 2700], ["HU", "ebook", 500],
  ["IE", "standard", 2300], ["IE", "ebook", 0],
  ["IT", "standard", 2200], ["IT", "ebook", 400],
  ["LV", "standard", 2100], ["LV", "ebook", 500],
  ["LT", "standard", 2100], ["LT", "ebook", 900],
  ["LU", "standard", 1700], ["LU", "ebook", 300],
  ["MT", "standard", 1800], ["MT", "ebook", 500],
  ["NL", "standard", 2100], ["NL", "ebook", 900],
  ["PL", "standard", 2300], ["PL", "ebook", 500],
  ["PT", "standard", 2300], ["PT", "ebook", 600],
  ["RO", "standard", 1900], ["RO", "ebook", 500],
  ["SK", "standard", 2300], ["SK", "ebook", 1000],
  ["SI", "standard", 2200], ["SI", "ebook", 500],
  ["ES", "standard", 2100], ["ES", "ebook", 400],
  ["SE", "standard", 2500], ["SE", "ebook", 600],
  ["GB", "standard", 2000], ["GB", "ebook", 0],
];

const SEED_FROM = "2026-01-01T00:00:00.000Z";

let ready: Promise<void> | null = null;

/** Brings the database up to date, once per isolate. On failure the cached
 *  promise is cleared so the next request retries rather than inheriting a
 *  permanent error. */
export function ensureSchema(env: Env): Promise<void> {
  if (!ready) {
    ready = migrate(env).catch((e) => {
      ready = null;
      throw e;
    });
  }
  return ready;
}

/** Exposed for tests, which need a fresh run against a fresh database. */
export function resetSchemaCache(): void {
  ready = null;
}

/** The runner's own bookkeeping: which migrations this database has seen.
 *
 *  Not part of MIGRATIONS — it has to exist before the first one can be
 *  recorded — but it is part of the schema, so allStatements names it too and
 *  schema.sql creates it like everything else. */
const BOOKKEEPING = `CREATE TABLE IF NOT EXISTS schema_migrations (
       version    INTEGER PRIMARY KEY,
       applied_at TEXT NOT NULL
     )`;

async function migrate(env: Env): Promise<void> {
  const db = env.SHOP_DB;
  await db.prepare(BOOKKEEPING).run();

  const row = await db.prepare(`SELECT MAX(version) AS v FROM schema_migrations`).first<{ v: number | null }>();
  const current = row?.v ?? 0;

  for (let version = current + 1; version <= MIGRATIONS.length; version++) {
    const statements = MIGRATIONS[version - 1] ?? [];
    for (const sql of statements) {
      try {
        await db.prepare(sql).run();
      } catch (e) {
        // ALTER TABLE ADD COLUMN on a column that exists is the one error a
        // re-run legitimately produces; anything else is a real failure.
        if (!/duplicate column/i.test(String(e))) throw e;
      }
    }
    await db
      .prepare(`INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (?, ?)`)
      .bind(version, new Date().toISOString())
      .run();
  }

  await seedVatRates(env);
}

/** Seeds the rate table only when it is empty: a shop that has edited its rates
 *  must never have them overwritten by a deploy. */
async function seedVatRates(env: Env): Promise<void> {
  const existing = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM vat_rates`).first<{ n: number }>();
  if ((existing?.n ?? 0) > 0) return;
  const insert = env.SHOP_DB.prepare(
    `INSERT OR IGNORE INTO vat_rates (country, kind, rate_bp, valid_from, valid_to) VALUES (?, ?, ?, ?, NULL)`,
  );
  await env.SHOP_DB.batch(VAT_SEED.map(([country, kind, bp]) => insert.bind(country, kind, bp, SEED_FROM)));
}

/** The statements a fresh database runs, for the test that keeps schema.sql
 *  honest. */
export function allStatements(): string[] {
  return [BOOKKEEPING, ...MIGRATIONS.flat()];
}
