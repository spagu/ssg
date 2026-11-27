// NEWSLETTER_HOOK_FROM_DOMAINS (#463): which sender domains a webhook event
// must come from to be applied.
//
// A provider account can hold several sites, and a webhook registered on it
// receives the events of every sending domain of the account. Without a list
// each worker applies every event by address: another site's hard bounce or
// complaint suppresses the address here, its `delivered` resets the soft
// count here, its traffic counts as this worker's webhook activity.
//
// Unset (or only separators): every verified event is applied, as before.
// Set: an event is applied only when the domain of its sender address is on
// the list — exact, case-insensitive, no subdomain matching (news.example.com
// and example.com are different senders, and two sites may be siblings under
// one parent). An event without a readable sender is not provably ours and is
// skipped too. A skipped event is acknowledged and writes nothing.
//
// A value that is not a list of plain domains is a `problem`: the hooks
// answer 503 while it stands, so the provider retries and nothing is lost.

import type { Env } from "./lib";

export interface HookOrigin {
  domains: string[]; // lower-cased, distinct, in the order given; empty = no filter
  problem: string | null; // why the list cannot be applied; the hooks answer 503 while set
}

const LABEL = /^[a-z0-9-]+$/;

// isDomain accepts a plain domain: two or more dot-separated labels of
// [a-z0-9-]. An address, a URL, a port or a scheme fails on its @, / or :.
const isDomain = (d: string): boolean => {
  const labels = d.split(".");
  return labels.length > 1 && labels.every((l) => LABEL.test(l));
};

// hookOrigin reads the list: entries separated by commas and/or whitespace,
// any case, repeats ignored.
export function hookOrigin(env: Env): HookOrigin {
  const entries = [...new Set((env.NEWSLETTER_HOOK_FROM_DOMAINS ?? "").toLowerCase().split(/[\s,]+/).filter(Boolean))];
  const bad = entries.filter((d) => !isDomain(d));
  if (!bad.length) return { domains: entries, problem: null };
  const quoted = bad.map((d) => JSON.stringify(d.slice(0, 40))).join(", ");
  return { domains: entries, problem: `NEWSLETTER_HOOK_FROM_DOMAINS: ${quoted} is not a plain domain; list sender domains such as example.com, separated by commas` };
}

// originSettings: what GET /api/settings adds to `webhooks` — the list as
// read (also when it has a problem) and the problem.
export function originSettings(env: Env): { fromDomains: string[]; fromDomainsProblem: string | null } {
  const { domains, problem } = hookOrigin(env);
  return { fromDomains: domains, fromDomainsProblem: problem };
}

// senderDomain reads the domain of a sender: "a@example.com" or
// "Name <a@example.com>", lower-cased; null when there is no address.
export function senderDomain(from: unknown): string | null {
  if (typeof from !== "string") return null;
  const at = from.lastIndexOf("@");
  if (at < 0) return null;
  const domain = from.slice(at + 1).replace(/>\s*$/, "").trim().toLowerCase();
  return domain || null;
}

// fromUs says whether an event whose sender is `from` is to be applied.
export function fromUs(origin: HookOrigin, from: unknown): boolean {
  if (!origin.domains.length) return true;
  const domain = senderDomain(from);
  return domain !== null && origin.domains.includes(domain);
}
