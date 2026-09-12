-- Schema for the ecommerce worker, written out for a human.
--
-- You do not have to run this: the worker applies its own migrations on first
-- use (see _schema.ts). It is here so the tables can be read without reading
-- TypeScript, and so a local database can be created in one command:
--
--   wrangler d1 execute shop --local --file=functions/api/shop/schema.sql
--
-- A test compares this file's objects against the ones the migrations actually
-- create, so the two cannot drift apart unnoticed.

CREATE TABLE IF NOT EXISTS schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
  key         TEXT PRIMARY KEY,
  value       TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS products (
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
);

CREATE INDEX IF NOT EXISTS idx_products_status ON products (status, sku);

CREATE TABLE IF NOT EXISTS prices (
  product_id   TEXT NOT NULL,
  currency     TEXT NOT NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor >= 0),
  PRIMARY KEY (product_id, currency)
);

CREATE TABLE IF NOT EXISTS customers (
  id           TEXT PRIMARY KEY,
  email        TEXT NOT NULL,
  email_lc     TEXT NOT NULL,
  name         TEXT,
  vat_id       TEXT,
  vat_id_valid INTEGER,
  country      TEXT,
  created_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_customers_email ON customers (email_lc);

CREATE TABLE IF NOT EXISTS orders (
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
);

CREATE INDEX IF NOT EXISTS idx_orders_status_created ON orders (status, created_at);

CREATE INDEX IF NOT EXISTS idx_orders_customer ON orders (customer_id, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_gateway_ref ON orders (gateway, gateway_ref);

CREATE TABLE IF NOT EXISTS order_items (
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
);

CREATE INDEX IF NOT EXISTS idx_items_order ON order_items (order_id);

CREATE TABLE IF NOT EXISTS payments (
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
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_ref ON payments (gateway, gateway_ref, kind);

CREATE TABLE IF NOT EXISTS webhook_inbox (
  gateway      TEXT NOT NULL,
  event_id     TEXT NOT NULL,
  event_type   TEXT NOT NULL,
  received_at  TEXT NOT NULL,
  processed_at TEXT,
  result       TEXT,
  PRIMARY KEY (gateway, event_id)
);

CREATE TABLE IF NOT EXISTS download_tokens (
  token_hash    TEXT PRIMARY KEY,
  order_item_id TEXT NOT NULL,
  expires_at    TEXT NOT NULL,
  uses          INTEGER NOT NULL DEFAULT 0,
  max_uses      INTEGER NOT NULL,
  revoked       INTEGER NOT NULL DEFAULT 0,
  created_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tokens_item ON download_tokens (order_item_id);

CREATE TABLE IF NOT EXISTS downloads (
  id         TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL,
  ip_hash    TEXT,
  user_agent TEXT,
  bytes_sent INTEGER,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS vat_rates (
  country    TEXT NOT NULL,
  kind       TEXT NOT NULL,
  rate_bp    INTEGER NOT NULL,
  valid_from TEXT NOT NULL,
  valid_to   TEXT,
  PRIMARY KEY (country, kind, valid_from)
);

CREATE TABLE IF NOT EXISTS invoice_sequences (
  series TEXT NOT NULL,
  year   INTEGER NOT NULL,
  next   INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY (series, year)
);

CREATE TABLE IF NOT EXISTS invoices (
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
);

CREATE INDEX IF NOT EXISTS idx_invoices_order ON invoices (order_id);

CREATE TABLE IF NOT EXISTS outbox (
  id           TEXT PRIMARY KEY,
  kind         TEXT NOT NULL,
  target       TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  attempts     INTEGER NOT NULL DEFAULT 0,
  next_attempt TEXT NOT NULL,
  last_error   TEXT,
  done_at      TEXT,
  created_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_outbox_due ON outbox (done_at, next_attempt);

CREATE TABLE IF NOT EXISTS audit_log (
  id          TEXT PRIMARY KEY,
  actor       TEXT NOT NULL,
  action      TEXT NOT NULL,
  subject     TEXT,
  detail_json TEXT,
  created_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log (created_at);

CREATE TABLE IF NOT EXISTS admin_users (
  id            TEXT PRIMARY KEY,
  email_lc      TEXT NOT NULL UNIQUE,
  pass_hash     TEXT NOT NULL,
  role          TEXT NOT NULL DEFAULT 'owner',
  created_at    TEXT NOT NULL,
  last_login_at TEXT
);

CREATE TABLE IF NOT EXISTS admin_sessions (
  jti_hash     TEXT PRIMARY KEY,
  admin_id     TEXT NOT NULL,
  expires_at   TEXT NOT NULL,
  rotated_from TEXT,
  created_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_admin ON admin_sessions (admin_id);

ALTER TABLE admin_sessions ADD COLUMN used_at TEXT;

CREATE INDEX IF NOT EXISTS idx_sessions_used ON admin_sessions (used_at);
