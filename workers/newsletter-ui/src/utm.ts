// UTM tagging of links to the site (#443), off unless NEWSLETTER_UTM is set.
//
// With it on, a campaign link whose host is the site's (NEWSLETTER_SITE_URL,
// plus the hosts in NEWSLETTER_UTM_HOSTS) gets four parameters:
//   utm_source   NEWSLETTER_UTM_SOURCE, default "newsletter"
//   utm_medium   email
//   utm_campaign a slug of the campaign's subject (the campaign id when the
//                subject has no ASCII letter or digit)
//   utm_content  the link id the click counter uses: the first 16 hex digits
//                of sha256 of the link AS WRITTEN, before tagging
// They name the campaign and the link, never the reader, so the same link is
// the same URL in every recipient's mail.
//
// Left exactly as written: a link that already carries any utm_* parameter, a
// link to another host, and what the renderer never offers to a rewriter at
// all — the unsubscribe link, mailto: and images (click-rewrite.ts).

import type { Env } from "./lib";
import { isOn } from "./lib";
import type { Site } from "./render";

export const DEFAULT_UTM_SOURCE = "newsletter";
const MAX_SLUG = 60;

export interface Utm {
  source: string;
  campaign: string;
  hosts: Set<string>;
}

export const utmOn = (env: Env): boolean => isOn(env.NEWSLETTER_UTM);

export const utmSource = (env: Env): string => (env.NEWSLETTER_UTM_SOURCE ?? "").trim() || DEFAULT_UTM_SOURCE;

// utmHosts is the site's host plus NEWSLETTER_UTM_HOSTS (hosts separated by
// commas or spaces), lower case. A host is compared whole: "www.example.com"
// is not "example.com" unless it is listed.
export function utmHosts(env: Env, site: Site): Set<string> {
  const extra = (env.NEWSLETTER_UTM_HOSTS ?? "").split(/[\s,]+/).filter(Boolean);
  return new Set([site.host, ...extra].map((h) => h.toLowerCase()));
}

// campaignSlug turns a subject into a utm_campaign value: ASCII lower case,
// accents dropped, everything else a dash. `fallback` (the campaign id) is
// used when nothing is left, e.g. a subject in a non-Latin script.
export function campaignSlug(subject: string, fallback: string): string {
  const slug = subject
    .normalize("NFKD")
    .replace(/\p{M}+/gu, "")
    .replace(/[łŁ]/g, "l")
    .replace(/ß/g, "ss")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .slice(0, MAX_SLUG)
    .replace(/^-+|-+$/g, "");
  return slug || fallback;
}

// utmFor is the tagging of one campaign, or null with NEWSLETTER_UTM off.
export function utmFor(env: Env, site: Site, c: { id: string; subject: string }): Utm | null {
  if (!utmOn(env)) return null;
  return { source: utmSource(env), campaign: campaignSlug(c.subject, c.id), hosts: utmHosts(env, site) };
}

// tagUrl answers `url` with the UTM parameters appended (before a #fragment),
// or null when the link is not to be tagged. The rest of the URL is kept
// character for character.
export function tagUrl(url: string, utm: Utm, linkId: string): string | null {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return null;
  }
  if (!utm.hosts.has(parsed.hostname.toLowerCase())) return null;
  for (const key of parsed.searchParams.keys()) if (/^utm_/i.test(key)) return null;
  const hash = url.indexOf("#");
  const head = hash < 0 ? url : url.slice(0, hash);
  const joint = !head.includes("?") ? "?" : /[?&]$/.test(head) ? "" : "&";
  const params = [
    ["utm_source", utm.source],
    ["utm_medium", "email"],
    ["utm_campaign", utm.campaign],
    ["utm_content", linkId],
  ].map(([k, v]) => `${k}=${encodeURIComponent(v)}`);
  return head + joint + params.join("&") + (hash < 0 ? "" : url.slice(hash));
}
