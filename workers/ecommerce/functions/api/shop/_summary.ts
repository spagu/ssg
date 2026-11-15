// The shop's module summary (GO-156): a few numbers, what waits for the owner
// and what is wrong with the configuration, for the admin home page of the
// `core` worker (workers/module-summary.schema.json). A leading underscore
// keeps this file out of the Pages route table — module/summary.ts serves it.
//
// Aggregates only: two statements of COUNT and SUM (plus the cached settings
// read), never a buyer's name, address, email or anything else about a person
// — test/summary.test.ts seeds recognisable personal data and scans the answer
// for it. The words are in _summary-text.ts.

import { enabledGateways } from "./_env";
import type { Env } from "./_env";
import { currencyDecimals } from "./_money";
import { allSettings } from "./_settings";
import { counted, summaryOf } from "./_summary-kit";
import type { Attention, Metric, Problem, Summary } from "./_summary-kit";
import { LABELS, NOTES, OUTBOX_FAILED, PROBLEMS, REVIEW, UNDELIVERED, VAT_ENDING, WEBHOOK_ERRORS, revenueLabel } from "./_summary-text";
import type { ProblemCode } from "./_summary-text";

// Where the attention items link: views of the panel, which routes by hash.
export const PANEL = "/ecommerce-admin/";
const DAY = 86_400_000;
// FAILED_ATTEMPTS: an outgoing message tried this often is reported as
// failing — the same threshold as the panel's warning (admin/me.ts).
export const FAILED_ATTEMPTS = 5;

// isLocalHost: a development host, where test keys are expected. Anything
// else counts as a live site for the test-mode warnings.
export function isLocalHost(hostname: string): boolean {
  const h = hostname.toLowerCase();
  return h === "localhost" || h.endsWith(".localhost") || h === "[::1]" || h === "::1" || /^127\.\d+\.\d+\.\d+$/.test(h);
}

// configProblems: what is wrong with the bindings, secrets and settings —
// named, never with a value. `settings` is null without a database.
export function configProblems(env: Env, settings: Record<string, unknown> | null): Problem[] {
  const codes: ProblemCode[] = [];
  if (!env.SHOP_DB) codes.push("no_database");
  if (!env.SHOP_FILES) codes.push("no_files");
  if (!env.SHOP_KV) codes.push("no_kv");
  if (enabledGateways(env).length === 0) codes.push("no_gateway");
  if (!env.SHOP_MAIL_URL || !env.SHOP_MAIL_FROM) codes.push("no_mail");
  if (settings && settings["modules.invoices"] !== false && ["seller.name", "seller.address", "seller.country"].some((k) => !settings[k])) {
    codes.push("no_seller_details");
  }
  return codes.map((code) => ({ code, text: PROBLEMS[code] }));
}

// testModeOnLive: a warning per gateway that takes no real money while the
// site is reached under a public host name.
function testModeOnLive(env: Env, live: boolean): Attention[] {
  if (!live) return [];
  const offered = enabledGateways(env);
  const out: Attention[] = [];
  if (offered.includes("stripe") && env.STRIPE_SECRET_KEY!.startsWith("sk_test_")) out.push({ severity: "warning", text: NOTES.stripeTest, href: `${PANEL}#payments` });
  if (offered.includes("paypal") && env.SHOP_PAYPAL_ENV !== "live") out.push({ severity: "warning", text: NOTES.paypalSandbox, href: `${PANEL}#payments` });
  return out;
}

interface Figures {
  paid: number | null;
  gross: number | null;
  review: number | null;
  refunds: number;
  refunded: number;
  onSale: number;
  failedMessages: number;
  webhookErrors: number;
  vatEnding: number;
  vatNewest: string | null;
  undelivered: number;
}

// figures: every number, in two statements over indexed tables.
async function figures(db: D1Database, currency: string, now: number): Promise<Figures> {
  const since = new Date(now - 30 * DAY).toISOString();
  const orders = await db
    .prepare(
      `SELECT SUM(CASE WHEN status IN ('paid', 'fulfilled', 'refunded') AND paid_at >= ? THEN 1 ELSE 0 END) AS paid,
              SUM(CASE WHEN status IN ('paid', 'fulfilled', 'refunded') AND paid_at >= ? AND currency = ? THEN total_minor ELSE 0 END) AS gross,
              SUM(CASE WHEN status = 'needs_review' THEN 1 ELSE 0 END) AS review
         FROM orders`,
    )
    .bind(since, since, currency)
    .first<Pick<Figures, "paid" | "gross" | "review">>();
  const rest = await db
    .prepare(
      `SELECT
         (SELECT COUNT(*) FROM payments WHERE kind = 'refund' AND status = 'succeeded' AND created_at >= ?) AS refunds,
         (SELECT COALESCE(SUM(ABS(amount_minor)), 0) FROM payments WHERE kind = 'refund' AND status = 'succeeded' AND created_at >= ? AND currency = ?) AS refunded,
         (SELECT COUNT(*) FROM products WHERE status = 'active') AS onSale,
         (SELECT COUNT(*) FROM outbox WHERE done_at IS NULL AND attempts >= ?) AS failedMessages,
         (SELECT COUNT(*) FROM webhook_inbox WHERE result LIKE 'error:%' AND received_at >= ?) AS webhookErrors,
         (SELECT COUNT(*) FROM vat_rates r WHERE r.valid_to > ? AND r.valid_to <= ?
             AND NOT EXISTS (SELECT 1 FROM vat_rates n WHERE n.country = r.country AND n.kind = r.kind AND n.valid_from >= r.valid_to)) AS vatEnding,
         (SELECT MAX(valid_from) FROM vat_rates) AS vatNewest,
         (SELECT COUNT(*) FROM orders o WHERE o.status IN ('paid', 'fulfilled') AND EXISTS (
             SELECT 1 FROM order_items i JOIN products p ON p.id = i.product_id
              WHERE i.order_id = o.id AND p.file_key IS NOT NULL
                AND NOT EXISTS (SELECT 1 FROM download_tokens t WHERE t.order_item_id = i.id))) AS undelivered`,
    )
    .bind(since, since, currency, FAILED_ATTEMPTS, new Date(now - 7 * DAY).toISOString(), new Date(now).toISOString(), new Date(now + 30 * DAY).toISOString())
    .first<Omit<Figures, "paid" | "gross" | "review">>();
  return { ...orders!, ...rest! };
}

// shopSummary is the whole answer of GET /api/shop/module/summary. `live`:
// the request reached a public host (see isLocalHost).
export async function shopSummary(env: Env, options: { live: boolean }, at: Date = new Date()): Promise<Summary> {
  if (!env.SHOP_DB) return summaryOf({ metrics: [], problems: configProblems(env, null) }, at);
  const settings = await allSettings(env);
  const currency = String(settings["currency.base"] || "EUR").toUpperCase();
  const f = await figures(env.SHOP_DB, currency, at.getTime());

  const net = Math.max(0, Number(f.gross ?? 0) - Number(f.refunded));
  const metrics: Metric[] = [
    { key: "paidOrders", label: LABELS.paidOrders, value: Number(f.paid ?? 0) },
    { key: "revenue", label: revenueLabel(currency), value: net / 10 ** currencyDecimals(currency) },
    { key: "refunds", label: LABELS.refunds, value: Number(f.refunds) },
    { key: "productsOnSale", label: LABELS.productsOnSale, value: Number(f.onSale) },
  ];

  const attention: Attention[] = [];
  const add = (severity: Attention["severity"], n: number, phrase: typeof REVIEW, view: string) => {
    if (n > 0) attention.push({ severity, text: counted(phrase, n), href: `${PANEL}#${view}` });
  };
  add("action", Number(f.review ?? 0), REVIEW, "orders");
  add("action", Number(f.undelivered), UNDELIVERED, "orders");
  add("warning", Number(f.webhookErrors), WEBHOOK_ERRORS, "payments");
  add("warning", Number(f.failedMessages), OUTBOX_FAILED, "log");
  attention.push(...testModeOnLive(env, options.live));
  add("warning", Number(f.vatEnding), VAT_ENDING, "vat");
  if (f.vatNewest && at.getTime() - Date.parse(f.vatNewest) > 365 * DAY) {
    attention.push({ severity: "info", text: NOTES.vatStale, href: `${PANEL}#vat` });
  }
  return summaryOf({ metrics, attention, problems: configProblems(env, settings) }, at);
}
