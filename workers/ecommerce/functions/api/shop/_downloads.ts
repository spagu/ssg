// Download tokens: the only way a paid file leaves R2.
//
// The token is 32 random bytes and the database keeps only its SHA-256, so a
// dump of the table yields no working links (S-11). Every check that can refuse
// runs before the file is opened, and the use counter is incremented
// atomically, because two parallel requests at a limit of one must not both
// succeed (S-12).

import type { Env } from "./_env";
import { nowISO, randomToken, sha256hex } from "./_lib";
import type { DownloadTokenRow, OrderItemRow, ProductRow } from "./_types";

export interface IssuedToken {
  token: string;
  tokenHash: string;
  itemId: string;
  expiresAt: string;
  maxUses: number;
  productName: string;
}

/** Issues one token per line of an order. Idempotent by construction: the
 *  caller only reaches this once per order (see _fulfil), and a re-run finds
 *  the tokens already there. */
export async function issueTokens(
  env: Env,
  items: OrderItemRow[],
  products: Map<string, ProductRow>,
): Promise<IssuedToken[]> {
  if (items.length === 0) return [];

  // Only a LIVE token counts as "already issued": resend revokes the old ones
  // first and expects new ones, and a revoked token must not block that.
  const existing = await env.SHOP_DB.prepare(
    `SELECT order_item_id FROM download_tokens
      WHERE revoked = 0 AND expires_at > ? AND order_item_id IN (${items.map(() => "?").join(",")})`,
  )
    .bind(nowISO(), ...items.map((i) => i.id))
    .all<{ order_item_id: string }>();
  const already = new Set((existing.results ?? []).map((r) => r.order_item_id));

  const issued: IssuedToken[] = [];
  const inserts = [];
  for (const item of items) {
    if (already.has(item.id)) continue;
    const product = products.get(item.product_id);
    if (!product || !product.file_key) continue; // nothing to deliver for this line

    const token = randomToken(32);
    const tokenHash = await sha256hex(token);
    const expiresAt = new Date(Date.now() + product.download_days * 86400000).toISOString();
    inserts.push(
      env.SHOP_DB.prepare(
        `INSERT INTO download_tokens (token_hash, order_item_id, expires_at, uses, max_uses, revoked, created_at)
         VALUES (?, ?, ?, 0, ?, 0, ?)`,
      ).bind(tokenHash, item.id, expiresAt, product.download_limit, nowISO()),
    );
    issued.push({
      token,
      tokenHash,
      itemId: item.id,
      expiresAt,
      maxUses: product.download_limit,
      productName: product.name,
    });
  }
  if (inserts.length > 0) await env.SHOP_DB.batch(inserts);
  return issued;
}

export type TokenCheck =
  | { ok: true; row: DownloadTokenRow }
  | { ok: false; reason: "not_found" | "revoked" | "expired" | "exhausted" | "not_paid" };

/** Every reason a token can be refused, checked in the order that costs least.
 *  The result is deliberately specific so the download endpoint can say
 *  "expired" rather than a blanket 404 — a buyer whose link has aged out needs
 *  to know to ask for a new one. */
export async function checkToken(env: Env, token: string): Promise<TokenCheck> {
  const tokenHash = await sha256hex(token);
  const row = await env.SHOP_DB.prepare(`SELECT * FROM download_tokens WHERE token_hash = ?`)
    .bind(tokenHash)
    .first<DownloadTokenRow>();
  if (!row) return { ok: false, reason: "not_found" };
  if (row.revoked === 1) return { ok: false, reason: "revoked" };
  if (Date.parse(row.expires_at) <= Date.now()) return { ok: false, reason: "expired" };
  if (row.uses >= row.max_uses) return { ok: false, reason: "exhausted" };

  const order = await env.SHOP_DB.prepare(
    `SELECT o.status AS status FROM orders o
       JOIN order_items i ON i.order_id = o.id
      WHERE i.id = ?`,
  )
    .bind(row.order_item_id)
    .first<{ status: string }>();
  if (!order || !["paid", "fulfilled"].includes(order.status)) return { ok: false, reason: "not_paid" };

  return { ok: true, row };
}

/** Spends one use, atomically.
 *
 *  The WHERE clause carries the limit, so the database decides who wins when
 *  two requests arrive together; `changes` tells us whether this one did. */
export async function spendUse(env: Env, tokenHash: string): Promise<boolean> {
  const res = await env.SHOP_DB.prepare(
    `UPDATE download_tokens SET uses = uses + 1 WHERE token_hash = ? AND uses < max_uses AND revoked = 0`,
  )
    .bind(tokenHash)
    .run();
  return (res.meta?.changes ?? 0) > 0;
}

/** A range request from a reader app that is continuing a download it already
 *  paid a use for. Without this window, an ereader fetching an EPUB in six
 *  chunks would exhaust a limit of five on the first book.
 *
 *  Keyed on token and client, kept in KV for ten minutes. Without KV bound the
 *  shop degrades to counting every request, which is safe but blunt. */
export async function inRangeWindow(env: Env, tokenHash: string, clientKey: string): Promise<boolean> {
  if (!env.SHOP_KV) return false;
  const key = `dl:${tokenHash}:${clientKey}`;
  return (await env.SHOP_KV.get(key)) !== null;
}

export async function openRangeWindow(env: Env, tokenHash: string, clientKey: string): Promise<void> {
  if (!env.SHOP_KV) return;
  await env.SHOP_KV.put(`dl:${tokenHash}:${clientKey}`, "1", { expirationTtl: 600 });
}

export async function revokeTokensForOrder(env: Env, orderId: string): Promise<number> {
  const res = await env.SHOP_DB.prepare(
    `UPDATE download_tokens SET revoked = 1
      WHERE order_item_id IN (SELECT id FROM order_items WHERE order_id = ?)`,
  )
    .bind(orderId)
    .run();
  return res.meta?.changes ?? 0;
}
