// The headers the preview needs (GO-188), set around the shared pipeline
// (portable/app.ts) rather than inside it: every request still goes through
// the base path, the rate limit, the gate and the route's permission first.
//
//   - {base}/api/preview: the page is the worker's own answer, framed by the
//     panel only (frame-ancestors 'self'; the site's X-Frame-Options: DENY
//     never reaches it). PREVIEW_CSP runs nothing — no script source at all —
//     and `sandbox` gives the page an opaque origin even when it is opened
//     in a tab of its own, so it can never act with the `auth` session that
//     shares this origin. Styles, images and fonts load as on the site.
//   - the panel's own page: its policy (portable/http-kit.ts CSP) gains
//     frame-src 'self', the one frame it shows.
// Everything else passes untouched.

import { underBase } from "../portable/http-kit";
import { basePath } from "./http";
import type { Env } from "./http";

export const PREVIEW_CSP = [
  "default-src 'none'",
  // The site's stylesheets, inline styles and images; https: for the web
  // fonts and images a theme takes from a font or image host.
  "style-src 'self' 'unsafe-inline' https:",
  "img-src 'self' data: https:",
  "font-src 'self' data: https:",
  "media-src 'self' https:",
  "base-uri 'self'",
  "form-action 'none'",
  "frame-ancestors 'self'",
  "sandbox",
].join("; ");

// withPreview runs the pipeline (`next`) and adjusts the two answers above.
export async function withPreview(request: Request, env: Env, next: (request: Request) => Promise<Response>): Promise<Response> {
  const res = await next(request);
  const base = basePath(env);
  const rel = base === null ? null : underBase(new URL(request.url).pathname, base);
  if (rel === "/api/preview") return headers(res, { "content-security-policy": PREVIEW_CSP, "x-frame-options": "SAMEORIGIN" });
  const panelPage = rel !== null && !rel.startsWith("/api/") && /^text\/html/i.test(res.headers.get("content-type") ?? "");
  if (!panelPage) return res;
  return headers(res, { "content-security-policy": `${res.headers.get("content-security-policy")}; frame-src 'self'` });
}

function headers(res: Response, set: Record<string, string>): Response {
  const out = new Response(res.body, res);
  for (const [k, v] of Object.entries(set)) out.headers.set(k, v);
  out.headers.delete("set-cookie");
  return out;
}
