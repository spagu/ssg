// POST /api/shop/resend — send the download links again.
//
// Always answers the same way, whether or not the email matches an order. An
// endpoint that says "no such order" is an endpoint that tells a stranger which
// addresses have bought something.

import type { Env } from "./_env";
import { issueTokens } from "./_downloads";
import { shopOrigin } from "./_fulfil";
import { fail, isValidEmail, json, readBody, str, verifyTurnstile } from "./_lib";
import { resendMail } from "./_mail";
import { drainOutbox, enqueue } from "./_outbox";
import { guard } from "./_ratelimit";
import { ensureSchema } from "./_schema";
import { allSettings, moduleOn } from "./_settings";
import type { CustomerRow, OrderItemRow, OrderRow, ProductRow } from "./_types";

interface ResendBody {
  email?: string;
  orderNumber?: string;
  turnstileToken?: string;
  "cf-turnstile-response"?: string;
}

export const onRequestPost: PagesFunction<Env> = async ({ request, env, waitUntil }) => {
  if (!env.SHOP_DB) return fail(request, "not_configured", "The shop database is not bound.", 503);
  await ensureSchema(env);

  // Switched off, this endpoint does not exist rather than refusing politely:
  // a shop that does not offer the form should not be advertising that it has
  // one to anyone probing.
  if (!(await moduleOn(env, "resend"))) {
    return fail(request, "not_found", "This shop does not offer that.", 404);
  }

  // Three in ten minutes. This endpoint sends email to an address the caller
  // chose, which is the shape of a mail-bombing tool if it is left uncapped.
  const limited = await guard(env, request, "resend");
  if (limited) return limited;

  const body = await readBody<ResendBody>(request);
  if (!body) return fail(request, "invalid_body", "The request body could not be read.", 400);

  const email = str(body.email, 254);
  if (!isValidEmail(email)) return fail(request, "invalid_email", "A valid email address is required.", 422);

  if (env.TURNSTILE_SECRET && (await moduleOn(env, "turnstile"))) {
    const token = str(body.turnstileToken ?? body["cf-turnstile-response"], 4096);
    const ip = request.headers.get("cf-connecting-ip");
    if (!token || !(await verifyTurnstile(env.TURNSTILE_SECRET, token, ip))) {
      return fail(request, "captcha_failed", "The anti-spam check did not pass. Please try again.", 403);
    }
  }

  // The work happens in the background and the answer is the same either way.
  waitUntil(resend(env, request, email, str(body.orderNumber, 40)).catch(() => undefined));

  return json({
    ok: true,
    message: "If that address has an order with us, the download links are on their way.",
  });
};

async function resend(env: Env, request: Request, email: string, orderNumber: string): Promise<void> {
  const customer = await env.SHOP_DB.prepare(`SELECT * FROM customers WHERE email_lc = ?`)
    .bind(email.toLowerCase())
    .first<CustomerRow>();
  if (!customer) return;

  const query = orderNumber
    ? env.SHOP_DB.prepare(
        `SELECT * FROM orders WHERE customer_id = ? AND number = ? AND status IN ('paid','fulfilled')`,
      ).bind(customer.id, orderNumber)
    : env.SHOP_DB.prepare(
        `SELECT * FROM orders WHERE customer_id = ? AND status IN ('paid','fulfilled')
          ORDER BY created_at DESC LIMIT 3`,
      ).bind(customer.id);

  const { results } = await query.all<OrderRow>();
  const orders = results ?? [];
  if (orders.length === 0) return;

  const settings = await allSettings(env);
  const renew = settings["downloads.renew_on_resend"] !== false;
  const origin = await shopOrigin(env, request);

  for (const order of orders) {
    const { results: items } = await env.SHOP_DB.prepare(
      `SELECT * FROM order_items WHERE order_id = ?`,
    )
      .bind(order.id)
      .all<OrderItemRow>();

    const products = new Map<string, ProductRow>();
    for (const item of items ?? []) {
      const p = await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`)
        .bind(item.product_id)
        .first<ProductRow>();
      if (p) products.set(item.product_id, p);
    }

    // Someone who lost the email a month later should not be left without the
    // book they paid for: expired tokens are replaced unless the shop says not
    // to. A live token cannot be re-sent, because only its hash was kept.
    if (renew) {
      await env.SHOP_DB.prepare(
        `UPDATE download_tokens SET revoked = 1
          WHERE order_item_id IN (SELECT id FROM order_items WHERE order_id = ?)`,
      )
        .bind(order.id)
        .run();
    }

    const tokens = await issueTokens(env, items ?? [], products);
    if (tokens.length === 0) continue;

    const mail = resendMail({
      locale: order.locale ?? "en",
      shopName: String(settings["shop.name"] ?? "Shop"),
      orderNumber: order.number,
      totalMinor: order.total_minor,
      currency: order.currency,
      downloads: tokens.map((t) => ({
        name: t.productName,
        url: `${origin}/api/shop/download/${t.token}`,
        expiresAt: t.expiresAt,
        maxUses: t.maxUses,
      })),
      resendUrl: `${origin}/shop/resend/`,
    });
    await enqueue(env, "email", customer.email, mail as unknown as Record<string, unknown>);
  }

  // The buyer is waiting on this email specifically, so push the queue now
  // rather than leaving it to the next visitor.
  await drainOutbox(env, 5).catch(() => 0);
}
