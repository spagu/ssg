// Row shapes, so a query result is typed the same way everywhere.

export interface ProductRow {
  id: string;
  sku: string;
  name: string;
  description: string | null;
  kind: string;
  status: string;
  tax_category: string;
  file_key: string | null;
  file_name: string | null;
  file_size: number | null;
  file_sha256: string | null;
  download_limit: number;
  download_days: number;
  image_url: string | null;
  created_at: string;
  updated_at: string;
}

export interface PriceRow {
  product_id: string;
  currency: string;
  amount_minor: number;
}

export interface CustomerRow {
  id: string;
  email: string;
  email_lc: string;
  name: string | null;
  vat_id: string | null;
  vat_id_valid: number | null;
  country: string | null;
  created_at: string;
}

export type OrderStatus =
  | "pending"
  | "paid"
  | "fulfilled"
  | "refunded"
  | "cancelled"
  | "needs_review"
  | "failed";

export interface OrderRow {
  id: string;
  number: string;
  key_hash: string;
  status: OrderStatus;
  customer_id: string | null;
  currency: string;
  subtotal_minor: number;
  tax_minor: number;
  total_minor: number;
  tax_country: string | null;
  tax_evidence: string | null;
  reverse_charge: number;
  gateway: string | null;
  gateway_ref: string | null;
  consent_marketing: number;
  consent_waiver: number;
  ip_hash: string | null;
  user_agent: string | null;
  locale: string | null;
  notes: string | null;
  created_at: string;
  paid_at: string | null;
  updated_at: string;
  /** Where the order came from: 'web' (the storefront), 'acp' (an AI agent
   *  platform, GO-163) or 'ucp' (Google's Universal Commerce Protocol, GO-167). Optional because rows read before migration 5 ran and
   *  orders built in memory may not carry it; the column defaults to 'web'. */
  channel?: string;
}

export interface OrderItemRow {
  id: string;
  order_id: string;
  product_id: string;
  sku: string;
  name: string;
  quantity: number;
  unit_minor: number;
  tax_rate_bp: number;
  tax_minor: number;
  total_minor: number;
}

export interface PaymentRow {
  id: string;
  order_id: string;
  gateway: string;
  gateway_ref: string;
  kind: "charge" | "refund";
  status: "pending" | "succeeded" | "failed";
  amount_minor: number;
  currency: string;
  raw_json: string | null;
  created_at: string;
}

export interface InvoiceRow {
  id: string;
  number: string;
  kind: "invoice" | "credit_note";
  order_id: string;
  corrects_id: string | null;
  issued_at: string;
  currency: string;
  total_minor: number;
  snapshot_json: string;
  pdf_key: string | null;
  created_at: string;
  /** SHA-256 of the key mailed with the invoice link (migration 7); null when
   *  none was mailed. Never leaves the database. */
  link_hash?: string | null;
}

export interface DownloadTokenRow {
  token_hash: string;
  order_item_id: string;
  expires_at: string;
  uses: number;
  max_uses: number;
  revoked: number;
  created_at: string;
}
