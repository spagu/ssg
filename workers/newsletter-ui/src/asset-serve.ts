// GET|HEAD {base}/hooks/a/:key — a campaign image, served by this worker when
// NEWSLETTER_ASSETS_URL is unset (#446).
//
// Public, like the click redirect: a mail client or its image proxy fetches it
// without credentials. Only a name the store itself writes (ASSET_KEY: 64 hex
// characters and an image extension) reaches the bucket; any other path —
// another prefix, "..", a %-escaped slash — is 404 before R2 is asked, so the
// route can never read an arbitrary object of the bucket.
//
// Nothing is counted and nothing about the request (IP, User-Agent, cookies,
// referrer) is read or stored: an image load is not an "open". This route is
// not open tracking through the back door, and must not become one.
//
// The answer is immutable (the key is the content's hash), carries the stored
// content type and nosniff, and a matching If-None-Match gets 304.

import { ASSET_KEY, assetsEnabled } from "./asset-config";
import { IMMUTABLE } from "./asset-store";
import { json } from "./lib";
import type { RouteContext } from "./router";

export async function serveAsset({ request, env, params }: RouteContext): Promise<Response> {
  if (!assetsEnabled(env)) return json({ error: "assets_disabled" }, 404);
  const key = params.key;
  if (!ASSET_KEY.test(key)) return json({ error: "not_found" }, 404);
  const bucket = env.NEWSLETTER_ASSETS as R2Bucket;
  // GET passes the request's conditional headers to R2, which answers a
  // matching If-None-Match with the object's metadata and no body.
  const obj = request.method === "HEAD" ? await bucket.head(key) : await bucket.get(key, { onlyIf: request.headers });
  if (!obj) return json({ error: "not_found" }, 404);
  const headers = new Headers({
    "content-type": obj.httpMetadata?.contentType ?? "application/octet-stream",
    "cache-control": IMMUTABLE,
    "x-content-type-options": "nosniff",
    etag: obj.httpEtag,
  });
  const body = "body" in obj ? (obj as R2ObjectBody).body : undefined;
  if (notModified(request.headers.get("if-none-match"), obj.httpEtag) || (request.method === "GET" && !body)) {
    return new Response(null, { status: 304, headers });
  }
  headers.set("content-length", String(obj.size));
  return new Response(request.method === "HEAD" ? null : body, { status: 200, headers });
}

// notModified: If-None-Match (a list, weak or strong, or *) names this etag.
export function notModified(header: string | null, etag: string): boolean {
  if (!header) return false;
  const bare = etag.replace(/^W\//, "");
  return header.split(",").some((t) => {
    const tag = t.trim();
    return tag === "*" || tag.replace(/^W\//, "") === bare;
  });
}
