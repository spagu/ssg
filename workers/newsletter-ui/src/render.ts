// renderMessage: the ONE function that turns a campaign into the mail a
// recipient gets. Preview, test send and the send engine all call it, so what
// the admin previews is byte for byte what a reader receives.
//
// Pipeline: variables → (Markdown → HTML) → sanitizer → layout + footer for the
// HTML part; the sanitized tree → text (or text_override) + footer for the
// text part. The footer with the unsubscribe link is appended to both parts
// always — also when the body already uses {{unsubscribe_url}} — so no mail can
// go out without a way to leave the list.

import { guarded, rewriteText, rewriteTree } from "./click-rewrite";
import type { LinkRewriter } from "./click-rewrite";
import { FOOTER, footerLanguage } from "./i18n";
import { markdownToHtml } from "./markdown";
import { escapeHtml } from "./render-entities";
import { layout } from "./render-layout";
import { htmlToText } from "./render-text";
import { sanitizeHtml, serialize } from "./sanitize";

export interface CampaignContent {
  subject: string;
  body: string;
  bodyFormat: "markdown" | "html";
  textOverride: string | null;
}

// The recipient as the engine reads it from `subscribers` (email is not used
// in the content: no {{email}} variable, a forwarded mail leaks nothing).
export interface Recipient {
  email: string;
  language: string | null;
  token: string;
}

export interface Site {
  url: string; // the origin, "https://example.com"
  host: string; // "example.com"
}

export interface Rendered {
  subject: string;
  html: string;
  text: string;
  headers: Record<string, string>;
}

// Optional render settings. `links` rewrites the body's http(s) links (click
// counting, GO-129); without it the output is exactly what it was before.
export interface RenderOptions {
  links?: LinkRewriter;
}

export const VARIABLES = ["unsubscribe_url", "language", "site_url"] as const;
const VARIABLE = /\{\{\s*([^{}]*?)\s*\}\}/g;

// unknownVariables lists every {{name}} in the texts that is not a variable.
// Saving or scheduling refuses them: an unknown name must never become an
// empty string in thousands of mails.
export function unknownVariables(...texts: (string | null)[]): string[] {
  const out = new Set<string>();
  for (const t of texts) {
    for (const m of (t ?? "").matchAll(VARIABLE)) {
      if (!(VARIABLES as readonly string[]).includes(m[1])) out.add(m[1]);
    }
  }
  return [...out];
}

// siteOf reads NEWSLETTER_SITE_URL. Only https: — the unsubscribe link sits in
// a List-Unsubscribe header mailbox providers POST to. null = not usable.
export function siteOf(env: { NEWSLETTER_SITE_URL?: string }): Site | null {
  try {
    const u = new URL((env.NEWSLETTER_SITE_URL ?? "").trim());
    return u.protocol === "https:" ? { url: u.origin, host: u.hostname } : null;
  } catch {
    return null;
  }
}

// unsubscribeUrl points at the `newsletter` template's endpoint on the site,
// which owns the subscribers and handles RFC 8058 one-click POSTs.
export const unsubscribeEndpoint = (site: Site): string => `${site.url}/api/newsletter/unsubscribe`;
export const unsubscribeUrl = (site: Site, token: string): string => `${unsubscribeEndpoint(site)}?token=${encodeURIComponent(token)}`;

// A language code is only substituted when it looks like one; a stored value
// that does not falls back to the footer's language.
const LANGUAGE = /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})?$/;

function substitute(text: string, values: Record<string, string>, escape: boolean): string {
  return text.replace(VARIABLE, (_whole, name: string) => {
    if (!Object.hasOwn(values, name)) throw new Error(`unknown variable {{${name}}}`);
    return escape ? escapeHtml(values[name]) : values[name];
  });
}

export function renderMessage(c: CampaignContent, r: Recipient, site: Site, opts: RenderOptions = {}): Rendered {
  const lang = footerLanguage(r.language);
  const unsub = unsubscribeUrl(site, r.token);
  const values = { unsubscribe_url: unsub, language: r.language && LANGUAGE.test(r.language) ? r.language : lang, site_url: site.url };
  const source = c.bodyFormat === "html" ? substitute(c.body, values, true) : markdownToHtml(substitute(c.body, values, false));
  // The rewriter never sees the unsubscribe link or a non-http(s) one, and
  // the footer is added after it ran: leaving the list is never redirected.
  const links = opts.links ? guarded(opts.links, unsubscribeEndpoint(site)) : null;
  const tree = links ? rewriteTree(sanitizeHtml(source), links) : sanitizeHtml(source);
  const subject = substitute(c.subject, values, false);
  const t = FOOTER[lang];
  const reason = t.reason.replace("{site}", site.host);

  const footerHtml = `<p style="margin:0;">${escapeHtml(reason)}<br>\n<a href="${escapeHtml(unsub)}">${escapeHtml(t.unsubscribe)}</a></p>`;
  const html = layout({ lang, title: subject, content: serialize(tree), footer: footerHtml });
  const override = c.textOverride !== null ? substitute(c.textOverride, values, false).trim() : null;
  const body = override === null ? htmlToText(tree) : links ? rewriteText(override, links) : override;
  const text = `${body}\n\n-- \n${reason}\n${t.unsubscribe}: ${unsub}\n`;

  return {
    subject,
    html,
    text,
    headers: {
      "List-Unsubscribe": `<${unsub}>`,
      "List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
      "List-Id": `<newsletter.${site.host}>`,
      Precedence: "bulk",
    },
  };
}
