// Links sent exactly as written (#464): NEWSLETTER_TRACK_SKIP_HOSTS.
//
// Hosts separated by commas or spaces. A link whose host is one of them, or a
// subdomain of one at a label boundary ("shop.example" covers
// "www.shop.example", never "notshop.example"), is never offered to click
// counting or UTM tagging: no /hooks/c/ rewrite, no click_links row, no utm_*.
// Skipping wins over NEWSLETTER_UTM_HOSTS and over the site's own host.
//
// The host is compared with what `new URL(link).hostname` gives: lower case,
// no port, an internationalised name in its punycode (xn--) form — so an
// entry must be written that way too.
//
// An entry that is not a plain host name is a configuration problem, never
// silently dropped: a typo would otherwise send the very link the admin
// wanted left alone through the redirect. The problem joins the content rules'
// (content-rules-config.ts), so scheduling is refused (503
// content_rules_invalid) until it is fixed.

import type { Env } from "./lib";

export interface SkipHosts {
  hosts: string[]; // the valid entries, lower case, distinct
  problem: string | null;
}

// A plain host name: letters, digits, dots and dashes, at least two labels,
// no empty label. Anything else (a scheme, a path, a port, user info) is not.
const HOST_CHARS = /^[a-z0-9.-]+$/;
const isHost = (s: string): boolean => HOST_CHARS.test(s) && s.includes(".") && s.split(".").every(Boolean);
const SHOWN = 5; // entries quoted in the problem; the rest are counted

// skipHostsConfig reads the variable. Unset, empty or only separators → no
// hosts and no problem: the mail is exactly what it was before #464.
export function skipHostsConfig(env: Env): SkipHosts {
  const entries = (env.NEWSLETTER_TRACK_SKIP_HOSTS ?? "").split(/[\s,]+/).filter(Boolean);
  const hosts = new Set<string>();
  const bad: string[] = [];
  for (const entry of entries) {
    const host = entry.toLowerCase();
    if (isHost(host)) hosts.add(host);
    else bad.push(entry);
  }
  return { hosts: [...hosts], problem: bad.length ? problemOf(bad) : null };
}

function problemOf(bad: string[]): string {
  const quoted = bad.slice(0, SHOWN).map((b) => `"${b.slice(0, 40)}"`);
  if (bad.length > SHOWN) quoted.push(`and ${bad.length - SHOWN} more`);
  return `NEWSLETTER_TRACK_SKIP_HOSTS must list plain host names (no scheme, path, port or @; internationalised names in their xn-- form), not ${quoted.join(", ")}`;
}

// skipsLink: whether `url` goes out as written. A URL that cannot be parsed
// has no host and is not skipped (the rewriters leave it alone anyway).
export function skipsLink(hosts: string[], url: string): boolean {
  if (!hosts.length) return false;
  let host: string;
  try {
    host = new URL(url).hostname; // already lower case for http(s)
  } catch {
    return false;
  }
  return hosts.some((h) => host === h || host.endsWith(`.${h}`));
}

// skipHostsOf: the fields GET /api/settings adds to `clicks`.
export function skipHostsOf(env: Env): { skipHosts: string[]; skipHostsProblem: string | null } {
  const { hosts, problem } = skipHostsConfig(env);
  return { skipHosts: hosts, skipHostsProblem: problem };
}
