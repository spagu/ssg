// GET {base}/api/campaigns/:id/images?language= — the pre-send image check
// (#446). It lists the images of the campaign as the rendered mail has them
// (the real renderer and sanitizer: what readers get, not a guess from the
// Markdown) and says for each one what may go wrong:
//
//   not_stored   not under the store's base URL (only with a usable store)
//   no_alt       missing or empty alt text
//   unreachable  did not answer 200 with an image type to a mail image
//                proxy's kind of request (src/image-probe.ts)
//
// It warns, it never blocks: the campaign is not changed and sending does not
// consult it. Nothing about the probe is stored. It works with the store off.

import { ASSET_KEY, SERVE_PREFIX, assetsBaseUrl, assetsEnabled } from "./asset-config";
import { previewMessage } from "./campaign-actions";
import { loadCampaign } from "./campaign-model";
import { pool, probeFetch, probeStore } from "./image-probe";
import type { Fetcher, Probe } from "./image-probe";
import { basePath, json } from "./lib";
import type { Env } from "./lib";
import { siteOf, unknownVariables } from "./render";
import type { Site } from "./render";
import type { RouteContext } from "./router";
import { sanitizeHtml } from "./sanitize";
import type { HtmlNode } from "./sanitize";

// At most this many distinct images are probed per call (the Workers Free
// subrequest budget, see image-probe.ts); the rest are judged on alt and
// address only and counted in `skipped`.
export const MAX_PROBED_IMAGES = 8;
export const PROBE_POOL = 4;
const LANGUAGE = /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})?$/;

export type ImageProblem = "not_stored" | "no_alt" | "unreachable";

export interface ImageItem {
  url: string;
  alt: string | null;
  stored: boolean;
  probe: Probe | null; // null: not probed (over MAX_PROBED_IMAGES)
  problems: ImageProblem[];
}

// imagesOf reads every <img> of a rendered mail, in document order. The mail
// is already sanitizer output; parsing it again gives the tree it came from.
export function imagesOf(html: string): { url: string; alt: string | null }[] {
  const out: { url: string; alt: string | null }[] = [];
  const walk = (nodes: HtmlNode[]) => {
    for (const n of nodes) {
      if (n.type !== "el") continue;
      if (n.tag === "img") {
        const attr = (name: string) => n.attrs.find(([k]) => k === name)?.[1] ?? null;
        out.push({ url: attr("src") as string, alt: attr("alt") });
      }
      walk(n.children);
    }
  };
  walk(sanitizeHtml(html));
  return out;
}

// ownKey: the store key when this worker serves the URL itself
// ({site}{base}/hooks/a/:key, store on), else null.
function ownKey(env: Env, site: Site, url: string): string | null {
  const base = basePath(env);
  if (!assetsEnabled(env) || base === null) return null;
  const prefix = `${site.url}${base}${SERVE_PREFIX}/`;
  const key = url.startsWith(prefix) ? url.slice(prefix.length) : "";
  return ASSET_KEY.test(key) ? key : null;
}

// checkImages builds the report for a rendered mail's HTML.
export async function checkImages(env: Env, site: Site, html: string, fetcher: Fetcher = fetch) {
  const images = imagesOf(html);
  const distinct = [...new Set(images.map((i) => i.url))];
  const probed = distinct.slice(0, MAX_PROBED_IMAGES);
  const probes = await pool(probed, PROBE_POOL, (url) => {
    const key = ownKey(env, site, url);
    return key ? probeStore(env.NEWSLETTER_ASSETS as R2Bucket, key) : probeFetch(url, fetcher);
  });
  const byUrl = new Map(probed.map((url, i) => [url, probes[i]]));
  const on = assetsEnabled(env);
  const base = assetsBaseUrl(env);
  const items: ImageItem[] = images.map(({ url, alt }) => {
    // A store that is on but has no usable address (assets.problem in the
    // settings) is a configuration fault: no image is blamed for it.
    const usable = on && base !== "";
    const stored = usable && url.startsWith(`${base}/`);
    const probe = byUrl.get(url) ?? null;
    const problems: ImageProblem[] = [];
    if (usable && !stored) problems.push("not_stored");
    if (!alt?.trim()) problems.push("no_alt");
    if (probe && !probe.ok) problems.push("unreachable");
    return { url, alt, stored, probe, problems };
  });
  return { items, checked: probed.length, skipped: distinct.length - probed.length };
}

export async function campaignImages({ env, params, url }: RouteContext): Promise<Response> {
  const language = url.searchParams.get("language") ?? "en";
  if (!LANGUAGE.test(language)) return json({ error: "invalid_input", detail: "language must be a language code" }, 400);
  const c = await loadCampaign(env, params.id);
  if (!c) return json({ error: "not_found" }, 404);
  const site = siteOf(env);
  if (!site) return json({ error: "no_site_url", detail: "set NEWSLETTER_SITE_URL to the site's https:// origin (where the newsletter template runs)" }, 503);
  // The renderer refuses unknown variables; say which, as saving would.
  const unknown = unknownVariables(c.subject, c.body, c.textOverride);
  if (unknown.length) return json({ error: "unknown_variable", detail: unknown.map((n) => `{{${n}}}`).join(", ") }, 400);
  return json(await checkImages(env, site, previewMessage(c, language, site).html));
}
