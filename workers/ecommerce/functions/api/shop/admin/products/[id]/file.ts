// POST /api/shop/admin/products/:id/file — upload the file buyers receive.
//
// The bytes go straight to R2 under a key this worker chooses. A filename from
// a form is never a storage key: that is how a "product" ends up written over
// somebody else's object (S-14).

import { maxFileBytes, type Env } from "../../../_env";
import { audit } from "../../../_audit";
import { requireOwner } from "../../../_auth";
import { fail, json, nowISO, safeFilename, sha256hex } from "../../../_lib";
import type { ProductRow } from "../../../_types";
import type { AdminData } from "../../_middleware";
import { actorOf } from "../index";

/** What a file claims to be, checked against what it starts with. An extension
 *  is a suggestion; the first bytes are evidence. */
function sniff(bytes: Uint8Array): string | null {
  const startsWith = (sig: number[]): boolean => sig.every((b, i) => bytes[i] === b);
  if (startsWith([0x25, 0x50, 0x44, 0x46])) return "application/pdf"; // %PDF
  if (startsWith([0x50, 0x4b, 0x03, 0x04])) {
    // A zip container. EPUB is the one we expect; the mimetype entry sits at a
    // fixed offset in a conforming EPUB.
    const text = new TextDecoder().decode(bytes.slice(30, 90));
    if (text.includes("application/epub+zip")) return "application/epub+zip";
    return "application/zip";
  }
  if (startsWith([0x42, 0x4f, 0x4f, 0x4b, 0x4d, 0x4f, 0x42, 0x49])) return "application/x-mobipocket-ebook";
  if (startsWith([0x54, 0x50, 0x5a, 0x33])) return "application/vnd.amazon.ebook";
  return null;
}

export const onRequestPost: PagesFunction<Env, string, AdminData> = async ({ request, env, params, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;
  if (!env.SHOP_FILES) return fail(request, "not_configured", "File storage is not bound. Bind SHOP_FILES.", 503);

  const id = String(params.id ?? "");
  const product = await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`).bind(id).first<ProductRow>();
  if (!product) return fail(request, "not_found", "No such product.", 404);

  let file: File | null = null;
  try {
    const form = await request.formData();
    const entry = form.get("file");
    if (entry instanceof File) file = entry;
  } catch {
    return fail(request, "invalid_body", "Send the file as multipart form data.", 400);
  }
  if (!file) return fail(request, "no_file", "No file was attached.", 422);

  const limit = maxFileBytes(env);
  if (file.size > limit) {
    return fail(
      request,
      "file_too_large",
      `That file is larger than the ${Math.round(limit / 1024 / 1024)} MB limit.`,
      413,
    );
  }

  // Buffering is what lets us hash and sniff before anything is stored. It also
  // sets the practical size limit: streaming uploads with an incremental hash
  // are a later module (ECOM-029).
  const bytes = new Uint8Array(await file.arrayBuffer());
  const detected = sniff(bytes);
  if (!detected) {
    return fail(
      request,
      "unsupported_file",
      "That does not look like a PDF, EPUB or MOBI file.",
      422,
    );
  }

  const sha = await sha256hex(bytes.buffer as ArrayBuffer);
  const name = safeFilename(file.name || `${product.sku}.bin`);
  // The key includes a content hash, so replacing a file never overwrites the
  // old object: an order placed yesterday keeps downloading what it bought.
  const key = `products/${product.id}/${sha.slice(0, 12)}-${name}`;

  await env.SHOP_FILES.put(key, bytes, {
    httpMetadata: { contentType: detected },
    customMetadata: { sku: product.sku, sha256: sha },
  });

  const previous = product.file_key;
  await env.SHOP_DB.prepare(
    `UPDATE products SET file_key = ?, file_name = ?, file_size = ?, file_sha256 = ?, updated_at = ? WHERE id = ?`,
  )
    .bind(key, name, bytes.byteLength, sha, nowISO(), id)
    .run();

  // Only once the new file is recorded is the old one removed, and only if no
  // order still points at it through a token.
  if (previous && previous !== key) {
    const inUse = await env.SHOP_DB.prepare(
      `SELECT COUNT(*) AS n FROM download_tokens t
         JOIN order_items i ON i.id = t.order_item_id
        WHERE i.product_id = ? AND t.revoked = 0`,
    )
      .bind(id)
      .first<{ n: number }>();
    if ((inUse?.n ?? 0) === 0) await env.SHOP_FILES.delete(previous);
  }

  await audit(env, actorOf(data), "product.file", id, { sku: product.sku, size: bytes.byteLength, sha256: sha });
  return json({ fileName: name, fileSize: bytes.byteLength, sha256: sha, contentType: detected });
};
