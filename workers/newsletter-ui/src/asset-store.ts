// The campaign image store's admin API (#446):
//
//   POST   {base}/api/assets       the raw image as the body → 201 (new) / 200 (already stored)
//   GET    {base}/api/assets       {items, nextCursor}, newest first, each with inUse
//   DELETE {base}/api/assets/:key  204; 409 asset_in_use while any campaign mentions it
//
// Two places hold an image: the `assets` table (the listing) and the R2 bucket
// (the bytes). They are written in an order that keeps every operation safely
// repeatable when the second write fails:
//   upload  R2 first, then the row — a row always has bytes behind it once
//           written; a failed insert leaves an unlisted object that the repeat
//           upload (same bytes → same key) puts again and lists.
//   delete  R2 first, then the row — a failed row delete leaves a listed image
//           without bytes; the repeat delete removes it (deleting a missing
//           object is harmless), and a repeat upload of the file heals it, since
//           an upload that finds the row checks the object and puts it back.
// The key is never taken from the client: it is the SHA-256 of the bytes
// stored — the upload itself, or the width-capped copy made with the optional
// Images binding, whose row then remembers the upload's hash (source_sha256).

import { ASSET_KEY, assetMaxBytes, assetUrl, assetsConfigProblem, assetsEnabled } from "./asset-config";
import { sniffImage } from "./asset-image";
import { assetMaxWidth, capWidth, resizeEnabled } from "./asset-resize";
import type { ResizeReport } from "./asset-resize";
import { json, sha256hex } from "./lib";
import type { Env } from "./lib";
import type { RouteContext } from "./router";
import { decodeCursor, encodeCursor } from "./subscribers";

export const IMMUTABLE = "public, max-age=31536000, immutable";
const DEFAULT_LIMIT = 50;
const MAX_LIMIT = 200;
const COLUMNS = "key, type, bytes, width, height, metadata, created_at";

export interface AssetRecord {
  key: string;
  url: string;
  type: string;
  bytes: number;
  width: number | null;
  height: number | null;
  metadata: boolean;
  createdAt: string;
}

const record = (env: Env, r: Record<string, unknown>): AssetRecord => ({
  key: r.key as string,
  url: assetUrl(env, r.key as string),
  type: r.type as string,
  bytes: Number(r.bytes),
  width: (r.width as number | null) ?? null,
  height: (r.height as number | null) ?? null,
  metadata: Number(r.metadata) === 1,
  createdAt: r.created_at as string,
});

// IN_USE: some campaign's stored content mentions the key, whatever its status
// — a sent mail sits in inboxes indefinitely, a draft would break too. `?` or
// a.key is the 64-hex key, which cannot appear by accident.
const mentions = (key: string) =>
  `EXISTS (SELECT 1 FROM campaigns c WHERE instr(c.body, ${key}) > 0 OR instr(COALESCE(c.text_override, ''), ${key}) > 0)`;

// ready: null when the store can be used, else the answer (404 when off,
// 503 when its address cannot be built).
function ready(env: Env): Response | null {
  if (!assetsEnabled(env)) return json({ error: "assets_disabled" }, 404);
  const problem = assetsConfigProblem(env);
  return problem ? json({ error: "assets_misconfigured", detail: problem }, 503) : null;
}

const tooLarge = (max: number) => json({ error: "too_large", detail: `an image may be at most ${max} bytes` }, 413);

// readCapped reads the body, giving up as soon as it passes max bytes (null).
// The declared content-length is checked first by the caller, but it is only
// a claim: the cap is enforced on what actually arrives.
async function readCapped(body: ReadableStream<Uint8Array> | null, max: number): Promise<Uint8Array | null> {
  if (!body) return new Uint8Array(0);
  const reader = body.getReader();
  const parts: Uint8Array[] = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    total += value.byteLength;
    if (total > max) {
      await reader.cancel();
      return null;
    }
    parts.push(value);
  }
  const out = new Uint8Array(total);
  let at = 0;
  for (const p of parts) {
    out.set(p, at);
    at += p.byteLength;
  }
  return out;
}

export async function uploadAsset({ request, env }: RouteContext): Promise<Response> {
  const notReady = ready(env);
  if (notReady) return notReady;
  const max = assetMaxBytes(env);
  const declared = request.headers.get("content-length");
  if (declared !== null && Number(declared) > max) return tooLarge(max);
  const bytes = await readCapped(request.body, max);
  if (!bytes) return tooLarge(max);
  if (!bytes.length) return json({ error: "empty_body", detail: "send the image file as the request body" }, 400);
  // Errors never echo the bytes: the detail names the accepted types only.
  const info = sniffImage(bytes);
  if (!info) return json({ error: "unsupported_type", detail: "JPEG, PNG or WebP only" }, 415);

  const hash = await sha256hex(bytes);
  const sourceKey = `${hash}.${info.ext}`;
  const db = env.NEWSLETTER_DB;
  const bucket = env.NEWSLETTER_ASSETS as R2Bucket;
  // `resize` (only with NEWSLETTER_IMAGES, src/asset-resize.ts) says what the
  // width cap did; without the binding the answer is exactly the record.
  const answer = (r: Record<string, unknown>, status: number, resize: ResizeReport | null) =>
    json(resize ? { ...record(env, r), resize } : record(env, r), status);
  // The same file again: stored as uploaded (key = its hash), or as a resized
  // copy remembered by source_sha256 — a transform need not give the same
  // bytes twice, so the copy's own hash cannot find it.
  const existing = await db
    .prepare(`SELECT ${COLUMNS}, source_sha256 FROM assets WHERE key = ? OR source_sha256 = ? ORDER BY key = ? DESC, created_at DESC LIMIT 1`)
    .bind(sourceKey, hash, sourceKey)
    .first<Record<string, unknown>>();
  const original = { width: info.width, height: info.height, bytes: bytes.length };
  const repeat = (r: Record<string, unknown>) =>
    answer(r, 200, resizeEnabled(env) ? { resized: r.source_sha256 != null, maxWidth: assetMaxWidth(env), original, error: null } : null);
  if (existing?.key === sourceKey) {
    if (!(await bucket.head(sourceKey))) await putObject(bucket, sourceKey, bytes, info.type); // a half-finished delete
    return repeat(existing);
  }
  // A resized copy whose object is gone is made again below, like a new upload.
  if (existing && (await bucket.head(existing.key as string))) return repeat(existing);

  const capped = await capWidth(env, bytes, info);
  const resized = capped.report?.resized === true;
  const key = resized ? `${await sha256hex(capped.bytes)}.${capped.info.ext}` : sourceKey;
  await putObject(bucket, key, capped.bytes, capped.info.type);
  const row = {
    key,
    type: capped.info.type,
    bytes: capped.bytes.length,
    width: capped.info.width,
    height: capped.info.height,
    metadata: capped.info.metadata ? 1 : 0,
    created_at: new Date().toISOString(),
  };
  const res = await db
    .prepare(`INSERT OR IGNORE INTO assets (${COLUMNS}, source_sha256) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
    .bind(row.key, row.type, row.bytes, row.width, row.height, row.metadata, row.created_at, resized ? hash : null)
    .run();
  if (res.meta.changes === 0) {
    // Another upload of the same file won the race: answer with its row.
    const won = await db.prepare(`SELECT ${COLUMNS} FROM assets WHERE key = ?`).bind(key).first<Record<string, unknown>>();
    return answer(won as Record<string, unknown>, 200, capped.report);
  }
  return answer(row, 201, capped.report);
}

const putObject = (bucket: R2Bucket, key: string, bytes: Uint8Array, type: string) =>
  bucket.put(key, bytes, { httpMetadata: { contentType: type, cacheControl: IMMUTABLE } });

// GET ?cursor=&limit= — newest first; each item says whether a campaign uses it.
export async function listAssets({ env, url }: RouteContext): Promise<Response> {
  const notReady = ready(env);
  if (notReady) return notReady;
  const p = url.searchParams;
  const rawLimit = p.get("limit");
  if (rawLimit !== null && (!/^\d+$/.test(rawLimit) || Number(rawLimit) < 1)) {
    return json({ error: "invalid_limit", detail: `limit must be 1-${MAX_LIMIT}` }, 400);
  }
  const limit = rawLimit === null ? DEFAULT_LIMIT : Math.min(Number(rawLimit), MAX_LIMIT);
  const args: unknown[] = [];
  let where = "";
  const rawCursor = p.get("cursor");
  if (rawCursor) {
    const c = decodeCursor(rawCursor);
    if (!c) return json({ error: "invalid_cursor", detail: "use nextCursor from the previous page" }, 400);
    where = "WHERE (a.created_at < ? OR (a.created_at = ? AND a.key < ?))";
    args.push(c[0], c[0], c[1]);
  }
  const { results } = await env.NEWSLETTER_DB.prepare(
    `SELECT a.key, a.type, a.bytes, a.width, a.height, a.metadata, a.created_at, ${mentions("a.key")} AS in_use
       FROM assets a ${where} ORDER BY a.created_at DESC, a.key DESC LIMIT ?`,
  )
    .bind(...args, limit + 1)
    .all<Record<string, unknown>>();
  const items = results.slice(0, limit).map((r) => ({ ...record(env, r), inUse: Number(r.in_use) === 1 }));
  const last = items[items.length - 1];
  return json({ items, nextCursor: results.length > limit ? encodeCursor(last.createdAt, last.key) : null });
}

export async function deleteAsset({ env, params }: RouteContext): Promise<Response> {
  const notReady = ready(env);
  if (notReady) return notReady;
  const key = params.key;
  if (!ASSET_KEY.test(key)) return json({ error: "not_found" }, 404);
  const db = env.NEWSLETTER_DB;
  const row = await db.prepare(`SELECT key, ${mentions("?")} AS in_use FROM assets WHERE key = ?`).bind(key, key, key).first<{ in_use: number }>();
  if (!row) return json({ error: "not_found" }, 404);
  if (Number(row.in_use) === 1) return json({ error: "asset_in_use", detail: "a campaign uses this image; remove it from every campaign first" }, 409);
  await (env.NEWSLETTER_ASSETS as R2Bucket).delete(key);
  await db.prepare("DELETE FROM assets WHERE key = ?").bind(key).run();
  return new Response(null, { status: 204 });
}
