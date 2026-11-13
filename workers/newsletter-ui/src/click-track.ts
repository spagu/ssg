// Click counting (GO-129), off unless NEWSLETTER_TRACK_CLICKS is set.
//
// With it on, every http(s) link in a campaign's body is rewritten to
//   {site}{base}/hooks/c/{campaignId}/{linkId}
// — the campaign and the link, nothing about the reader. linkId is the first
// 16 hex digits of sha256(url): a pure function of the target, so the same
// link carries the same id in every recipient's mail, in the preview and in a
// test send, without the renderer touching the database. The redirect
// (click-redirect.ts) only goes to targets stored in click_links for that
// campaign, so the worker is never an open redirector.
//
// Registration is lazy: the send engine registers a batch's links right
// before sending it (one SELECT, plus an INSERT per link not yet stored), and
// a test send registers its own so the tester's clicks work. The set of links
// can differ by language ({{language}} may sit in a URL), so links are
// collected per language present in the batch.

import type { Env } from "./lib";
import { basePath, sha256hex } from "./lib";
import { renderMessage } from "./render";
import type { CampaignContent, Recipient, Rendered, Site } from "./render";

// MAX_LINKS caps the distinct links counted per campaign; links beyond it are
// sent as written. A newsletter has a handful; a body with thousands is not
// worth thousands of rows.
export const MAX_LINKS = 100;

export const trackClicks = (env: Env): boolean => /^(1|true|yes|on)$/i.test((env.NEWSLETTER_TRACK_CLICKS ?? "").trim());

export const linkId = async (url: string): Promise<string> => (await sha256hex(url)).slice(0, 16);

export const clickUrl = (site: Site, base: string, campaignId: string, id: string): string =>
  `${site.url}${base}/hooks/c/${encodeURIComponent(campaignId)}/${id}`;

export type RenderFor = (r: Recipient) => Rendered;

export interface TrackedContent extends CampaignContent {
  id: string;
}

// rendererFor returns the function that renders `c` for one recipient. With
// tracking off (or no valid base path) it is renderMessage itself and the
// output is byte for byte what it was before GO-129. With it on, the links of
// every language among `recipients` are collected, given ids and — when
// `register` — stored, before anything is rendered for real. A content error
// (an unknown variable) is not thrown here: the returned function throws it
// again for the recipient concerned, where the caller already handles it.
export async function rendererFor(env: Env, c: TrackedContent, site: Site, recipients: Recipient[], register: boolean): Promise<RenderFor> {
  const base = basePath(env);
  if (!trackClicks(env) || base === null) return (r) => renderMessage(c, r, site);
  const ids = new Map<string, string>(); // url → link id
  const seen = new Set<string | null>();
  for (const r of recipients) {
    if (seen.has(r.language)) continue;
    seen.add(r.language);
    const urls: string[] = [];
    try {
      renderMessage(c, r, site, { links: (u) => (urls.push(u), null) });
    } catch {
      continue;
    }
    for (const u of urls) if (!ids.has(u) && ids.size < MAX_LINKS) ids.set(u, await linkId(u));
  }
  if (register && ids.size) await registerLinks(env, c.id, ids);
  const links = (u: string): string | null => {
    const id = ids.get(u);
    return id ? clickUrl(site, base, c.id, id) : null;
  };
  return (r) => renderMessage(c, r, site, { links });
}

// registerLinks stores the links not stored yet. INSERT OR IGNORE: two
// consumers registering the same link at once both succeed, one row remains.
export async function registerLinks(env: Env, campaignId: string, ids: Map<string, string>): Promise<void> {
  const db = env.NEWSLETTER_DB;
  const { results } = await db.prepare("SELECT link_id FROM click_links WHERE campaign_id = ?").bind(campaignId).all<{ link_id: string }>();
  const known = new Set(results.map((r) => r.link_id));
  const now = new Date().toISOString();
  const fresh = [...ids].filter(([, id]) => !known.has(id));
  if (!fresh.length) return;
  await db.batch(
    fresh.map(([url, id]) =>
      db.prepare("INSERT OR IGNORE INTO click_links (campaign_id, link_id, url, clicks, created_at) VALUES (?, ?, ?, 0, ?)").bind(campaignId, id, url, now),
    ),
  );
}
