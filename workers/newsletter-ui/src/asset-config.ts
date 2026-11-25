// Campaign images (#446): whether the store is on, where its images live and
// how large one may be. Every asset route and later features (the panel, the
// pre-send image check) read the configuration through these functions only.
//
//   NEWSLETTER_ASSETS            R2 bucket binding — the switch. Unbound, the
//                                feature does not exist: the routes answer 404
//                                assets_disabled and mail renders as before.
//   NEWSLETTER_ASSETS_URL        the bucket's own https base (an R2 custom
//                                domain): images are served by R2 and never
//                                reach this worker. Unset → this worker serves
//                                them at {NEWSLETTER_SITE_URL}{base}/hooks/a.
//   NEWSLETTER_ASSETS_MAX_BYTES  largest upload, default 2 MiB, clamped to 10 MiB.
//
// A base that cannot work (NEWSLETTER_ASSETS_URL not https or unparsable; no
// https NEWSLETTER_SITE_URL to serve from) is a configuration problem, reported
// like an unusable mail transport — never a silent fallback to another address,
// which would put images somewhere the admin did not choose.

import { basePath, intVar } from "./lib";
import type { Env } from "./lib";
import { siteOf } from "./render";

export const DEFAULT_ASSET_MAX_BYTES = 2 * 1024 * 1024;
// A mail image beyond a few hundred KiB is already slow on a phone; 10 MiB
// only stops a mistake (a raw camera file) from filling the bucket.
export const ASSET_MAX_BYTES_CEILING = 10 * 1024 * 1024;

// ASSET_KEY: the only names the store ever writes — the SHA-256 of the bytes
// and the extension of the sniffed type. 64 hex characters cannot appear in a
// campaign by accident, which is what the in-use check relies on.
export const ASSET_KEY = /^[0-9a-f]{64}\.(jpg|png|webp)$/;

// The path the worker serves images under, relative to the base path.
export const SERVE_PREFIX = "/hooks/a";

export const assetsEnabled = (env: Env): boolean => !!env.NEWSLETTER_ASSETS;

export function assetMaxBytes(env: Env): number {
  return Math.min(intVar(env.NEWSLETTER_ASSETS_MAX_BYTES, DEFAULT_ASSET_MAX_BYTES), ASSET_MAX_BYTES_CEILING);
}

// ownBase reads NEWSLETTER_ASSETS_URL: the base without trailing slashes, ""
// when unset, null when set to something that is not a plain https URL.
function ownBase(env: Env): string | null {
  const raw = (env.NEWSLETTER_ASSETS_URL ?? "").trim();
  if (!raw) return "";
  try {
    const u = new URL(raw);
    if (u.protocol !== "https:" || u.username || u.password || u.search || u.hash) return null;
    return (u.origin + u.pathname).replace(/\/+$/, "");
  } catch {
    return null;
  }
}

// baseProblem says why images cannot be given an address, or null.
function baseProblem(env: Env): string | null {
  const own = ownBase(env);
  if (own === null) return "NEWSLETTER_ASSETS_URL must be an https:// URL without a query, a fragment or credentials";
  if (own) return null;
  // Served by this worker: the mail needs an absolute https address. An http:
  // NEWSLETTER_SITE_URL (a local preview) is not one — the sanitizer keeps
  // https: images only, so such a URL would be dropped from the mail. The rule
  // is not widened for previews; set NEWSLETTER_ASSETS_URL there instead.
  if (!siteOf(env)) return "set NEWSLETTER_SITE_URL to the site's https:// origin, or NEWSLETTER_ASSETS_URL, to give campaign images an address";
  if (basePath(env) === null) return "NEWSLETTER_UI_BASE is not a valid path";
  return null;
}

// assetsConfigProblem is baseProblem when the store is on; without the binding
// there is nothing to configure, so nothing to report.
export const assetsConfigProblem = (env: Env): string | null => (assetsEnabled(env) ? baseProblem(env) : null);

// assetsBaseUrl is the https base every image URL starts with, no trailing
// slash; "" when the configuration gives none (assetsConfigProblem says why).
export function assetsBaseUrl(env: Env): string {
  if (baseProblem(env)) return "";
  return ownBase(env) || `${siteOf(env)!.url}${basePath(env)}${SERVE_PREFIX}`;
}

// assetUrl is the public address of one stored image.
export const assetUrl = (env: Env, key: string): string => `${assetsBaseUrl(env)}/${key}`;

// assetsOrigin is the origin stored images are served from, for the panel's
// img-src (src/app.ts); null when the store is off or has no address.
export function assetsOrigin(env: Env): string | null {
  const base = assetsEnabled(env) ? assetsBaseUrl(env) : "";
  return base ? new URL(base).origin : null;
}
