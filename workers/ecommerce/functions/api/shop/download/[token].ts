// GET /api/shop/download/:token — the only way a paid file leaves the bucket.
//
// Streams straight from R2 to the response: a 100 MB ebook must not be buffered
// in a Worker with a 128 MB memory budget.

import type { Env } from "../_env";
import { checkToken, inRangeWindow, openRangeWindow, spendUse } from "../_downloads";
import { fail, ipHash, newId, nowISO, safeFilename, userAgent } from "../_lib";
import { ensureSchema } from "../_schema";
import type { ProductRow } from "../_types";

export const onRequestGet: PagesFunction<Env> = async ({ request, env, params, waitUntil }) => {
  if (!env.SHOP_DB) return fail(request, "not_configured", "The shop database is not bound.", 503);
  if (!env.SHOP_FILES) return fail(request, "not_configured", "File storage is not bound. Bind SHOP_FILES.", 503);
  await ensureSchema(env);

  const token = String(params.token ?? "");
  if (token.length < 20 || token.length > 200) {
    return fail(request, "invalid_token", "That download link is not valid.", 400);
  }

  const check = await checkToken(env, token);
  if (!check.ok) {
    // Each refusal says which one it is: a buyer whose link aged out needs to
    // know to ask for a new one, not to wonder whether they were robbed.
    const messages: Record<string, [string, number]> = {
      not_found: ["That download link is not valid.", 404],
      revoked: ["That download link has been withdrawn.", 410],
      expired: ["That download link has expired. Request a new one.", 410],
      exhausted: ["That link has been used the maximum number of times. Request a new one.", 410],
      not_paid: ["This order has not been paid for.", 403],
    };
    const [message, status] = messages[check.reason] ?? ["That download link is not valid.", 404];
    return fail(request, check.reason, message, status);
  }

  const row = check.row;
  const product = await env.SHOP_DB.prepare(
    `SELECT p.* FROM products p JOIN order_items i ON i.product_id = p.id WHERE i.id = ?`,
  )
    .bind(row.order_item_id)
    .first<ProductRow>();
  if (!product?.file_key) {
    return fail(request, "file_missing", "This product has no file attached yet.", 404);
  }

  // A reader app continuing a download it already paid for must not be charged
  // again. The window is per token and per client, and opens on the first
  // request rather than on the first range request, because the first request
  // from an ereader is often already a ranged one.
  const clientKey = (await ipHash(request, env)) ?? "anon";
  const range = request.headers.get("range");
  const continuing = Boolean(range) && (await inRangeWindow(env, row.token_hash, clientKey));

  if (!continuing) {
    // The limit lives in the UPDATE, so two parallel requests at max_uses = 1
    // cannot both win (S-12).
    if (!(await spendUse(env, row.token_hash))) {
      return fail(request, "exhausted", "That link has been used the maximum number of times.", 410);
    }
    await openRangeWindow(env, row.token_hash, clientKey);
  }

  const object = range
    ? await env.SHOP_FILES.get(product.file_key, { range: request.headers })
    : await env.SHOP_FILES.get(product.file_key);
  if (!object) return fail(request, "file_missing", "The file could not be found in storage.", 404);

  const filename = safeFilename(product.file_name ?? `${product.sku}.bin`);
  const headers = new Headers();
  object.writeHttpMetadata(headers);
  headers.set("content-type", headers.get("content-type") ?? "application/octet-stream");
  // filename* with UTF-8 so a Polish title survives; the plain filename is the
  // sanitised fallback for old clients (S-13).
  headers.set(
    "content-disposition",
    `attachment; filename="${filename}"; filename*=UTF-8''${encodeURIComponent(product.file_name ?? filename)}`,
  );
  headers.set("cache-control", "private, no-store");
  headers.set("x-content-type-options", "nosniff");
  headers.set("accept-ranges", "bytes");
  if (product.file_sha256) headers.set("etag", `"${product.file_sha256}"`);

  let status = 200;
  // The request asking is what makes the answer partial. R2 can report a range
  // on an unranged get (its own full-object range), and a 206 to a client that
  // never asked for one is a protocol error some downloaders take badly.
  if (range && object.range && "offset" in object.range) {
    const offset = object.range.offset ?? 0;
    const length = object.range.length ?? object.size - offset;
    headers.set("content-range", `bytes ${offset}-${offset + length - 1}/${object.size}`);
    headers.set("content-length", String(length));
    status = 206;
  } else {
    headers.set("content-length", String(object.size));
  }

  // The log is written after the response is on its way; it is bookkeeping, not
  // part of the download.
  waitUntil(
    env.SHOP_DB.prepare(
      `INSERT INTO downloads (id, token_hash, ip_hash, user_agent, bytes_sent, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
    )
      .bind(newId(), row.token_hash, clientKey === "anon" ? null : clientKey, userAgent(request), object.size, nowISO())
      .run()
      .catch(() => undefined),
  );

  return new Response(object.body, { status, headers });
};
