// Click counting (GO-129), off unless NEWSLETTER_TRACK_CLICKS is set.
//
// With it on, every http(s) link in a campaign's body is rewritten to
//   {site}{base}/hooks/c/{campaignId}/{linkId}
// — the campaign and the link, nothing about the reader. linkId is the first
// 16 hex digits of sha256(url): a pure function of the target, so the same
// link carries the same id in every recipient's mail, in the preview and in a
// test send, without the renderer touching the database. With UTM tagging on
// (#443) the stored target is the tagged URL; the id stays that of the link
// as written. The redirect
// (click-redirect.ts) only goes to targets stored in click_links for that
// campaign, so the worker is never an open redirector.
//
// Registration is lazy: the send engine registers a batch's links right
// before sending it (one SELECT, plus an INSERT per link not yet stored), and
// a test send registers its own so the tester's clicks work. The set of links
// can differ by language ({{language}} may sit in a URL), so links are
// collected per language present in the batch.

import type { Env } from "./lib";
import { basePath, isOn, sha256hex } from "./lib";
import { renderMessage } from "./render";
import type { CampaignContent, Recipient, Rendered, Site } from "./render";
import { tagUrl, utmFor } from "./utm";

// MAX_LINKS caps the distinct links counted per campaign; links beyond it are
// sent as written. A newsletter has a handful; a body with thousands is not
// worth thousands of rows.
export const MAX_LINKS = 100;

export const trackClicks = (env: Env): boolean => isOn(env.NEWSLETTER_TRACK_CLICKS);

export const linkId = async (url: string): Promise<string> => (await sha256hex(url)).slice(0, 16);

export const clickUrl = (site: Site, base: string, campaignId: string, id: string): string =>
  `${site.url}${base}/hooks/c/${encodeURIComponent(campaignId)}/${id}`;

export type RenderFor = (r: Recipient) => Rendered;

export interface TrackedContent extends CampaignContent {
  id: string;
}

// rendererFor returns the function that renders `c` for one recipient. With
// click counting and UTM tagging (#443, utm.ts) both off it is renderMessage
// itself and the output is byte for byte what it was before GO-129. Otherwise
// the links of every language among `recipients` are collected and given ids
// before anything is rendered for real. A link is first tagged, then — with
// click counting on and a valid base path — replaced by its counting URL, so
// the stored redirect target already carries the UTM parameters; `register`
// stores those targets. The id is always that of the link as written, which
// makes utm_content and the click counter's link id the same value. A content
// error (an unknown variable) is not thrown here: the returned function
// throws it again for the recipient concerned, where the caller already
// handles it.
export async function rendererFor(env: Env, c: TrackedContent, site: Site, recipients: Recipient[], register: boolean): Promise<RenderFor> {
  const base = basePath(env);
  const track = trackClicks(env) && base !== null;
  const utm = utmFor(env, site, c);
  if (!track && !utm) return (r) => renderMessage(c, r, site);
  const ids = new Map<string, string>(); // url as written → link id
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
  const target = (u: string, id: string): string => (utm ? (tagUrl(u, utm, id) ?? u) : u);
  if (register && track && ids.size) await registerLinks(env, c.id, new Map([...ids].map(([u, id]) => [target(u, id), id])));
  const links = (u: string): string | null => {
    const id = ids.get(u);
    if (!id) return null;
    return track ? clickUrl(site, base, c.id, id) : target(u, id);
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
