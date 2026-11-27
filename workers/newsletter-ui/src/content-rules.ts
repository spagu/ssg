// Content rules (#449): house rules a campaign must pass before it is
// scheduled. Optional: with none of the variables set (content-rules-config.ts)
// nothing here changes any answer. They judge only what the author wrote —
// the subject, the body and the text version, never the footer the worker
// adds — rendered by the real renderer with the preview token:
//
//   banned_phrase  a NEWSLETTER_BANNED_PHRASES phrase in the subject, the body
//                  (its words, link texts and alt texts) or the text version
//                  when it is written by hand (content-rules-text.ts)
//   blocked_image  an image of the rendered mail whose URL contains a
//                  NEWSLETTER_BLOCKED_IMAGE_PATTERNS substring, in any case
//   utm_conflict   with NEWSLETTER_UTM_STRICT, a link the worker would tag
//                  (its host is in utmHosts) that already has a utm_*
//                  parameter: the worker leaves such a link as written, so
//                  the mail would carry the author's tags, not the worker's;
//                  a link to a NEWSLETTER_TRACK_SKIP_HOSTS host is never one
//
// GET /api/campaigns/:id/content-check is the read-only check for the panel;
// contentGate is what scheduling calls. Both render once per call.

import { PREVIEW_TOKEN, previewMessage } from "./campaign-actions";
import { loadCampaign } from "./campaign-model";
import type { Campaign } from "./campaign-model";
import { rewriteText, rewriteTree, guarded } from "./click-rewrite";
import { skipHostsConfig, skipsLink } from "./click-skip";
import { rulesConfig, summaryOf } from "./content-rules-config";
import type { ContentRules } from "./content-rules-config";
import { phraseHits, phraseRegExp, readerText, scannable } from "./content-rules-text";
import { imagesOf } from "./image-check";
import { json } from "./lib";
import type { Env } from "./lib";
import { renderParts, siteOf, unknownVariables, unsubscribeEndpoint } from "./render";
import type { CampaignContent, Site } from "./render";
import type { RouteContext } from "./router";
import { utmHosts } from "./utm";

const LANGUAGE = /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})?$/;

export type Where = "subject" | "body" | "text";

// One finding: `rule` says which rule, `where` which part of the mail,
// `value` what was found (the phrase as configured, the image URL, the link);
// the other fields are the rule's own details.
export type Finding =
  | { rule: "banned_phrase"; where: Where; value: string; match: string; count: number }
  | { rule: "blocked_image"; where: "body"; value: string; pattern: string }
  | { rule: "utm_conflict"; where: "body" | "text"; value: string; host: string; parameters: string[] };

function phraseFindings(rules: ContentRules, texts: [Where, string | null][]): Finding[] {
  const compiled = rules.phrases.map((phrase) => ({ phrase, re: phraseRegExp(phrase) }));
  return texts.flatMap(([where, text]) =>
    text === null ? [] : phraseHits(scannable(text), compiled).map((h): Finding => ({ rule: "banned_phrase", where, value: h.phrase, match: h.match, count: h.count })),
  );
}

function imageFindings(rules: ContentRules, html: string): Finding[] {
  const patterns = rules.imagePatterns.map((p) => [p, p.toLowerCase()]);
  const out: Finding[] = [];
  for (const url of new Set(imagesOf(html).map((i) => i.url))) {
    const hit = patterns.find(([, lower]) => url.toLowerCase().includes(lower));
    if (hit) out.push({ rule: "blocked_image", where: "body", value: url, pattern: hit[0] });
  }
  return out;
}

// utmParameters: the utm_* keys of a link to a tagged host, as written.
function utmParameters(url: string, hosts: Set<string>): { host: string; parameters: string[] } | null {
  let u: URL;
  try {
    u = new URL(url);
  } catch {
    return null;
  }
  const host = u.hostname.toLowerCase();
  const parameters = [...new Set([...u.searchParams.keys()].filter((k) => /^utm_/i.test(k)))];
  return hosts.has(host) && parameters.length ? { host, parameters } : null;
}

// utmFindings looks at exactly the links UTM tagging is offered: the http(s)
// links of the body and of a hand-written text version, never the
// unsubscribe link (click-rewrite.ts decides, here as in the renderer).
function utmFindings(env: Env, site: Site, tree: Parameters<typeof rewriteTree>[0], override: string | null): Finding[] {
  const hosts = utmHosts(env, site);
  const skip = skipHostsConfig(env).hosts; // #464: sent as written, so never tagged by the worker
  const out: Finding[] = [];
  const collect = (where: "body" | "text") => {
    const seen = new Set<string>();
    return guarded((url) => {
      const found = seen.has(url) || skipsLink(skip, url) ? null : utmParameters(url, hosts);
      seen.add(url);
      if (found) out.push({ rule: "utm_conflict", where, value: url, ...found });
      return null;
    }, unsubscribeEndpoint(site));
  };
  rewriteTree(tree, collect("body"));
  if (override !== null) rewriteText(override, collect("text"));
  return out;
}

// contentFindings checks a campaign against configured rules, rendered in
// `language` (only {{language}} depends on it). The text version is scanned
// for phrases only when written by hand: otherwise it is made from the body,
// which is scanned already.
export function contentFindings(env: Env, c: CampaignContent, site: Site, language: string, rules: ContentRules): Finding[] {
  const parts = renderParts(c, { email: "", language, token: PREVIEW_TOKEN }, site);
  return [
    ...(rules.phrases.length ? phraseFindings(rules, [["subject", parts.subject], ["body", readerText(parts.tree)], ["text", parts.override]]) : []),
    ...(rules.imagePatterns.length ? imageFindings(rules, previewMessage(c, language, site).html) : []),
    ...(rules.utmStrict ? utmFindings(env, site, parts.tree, parts.override) : []),
  ];
}

const invalid = (problem: string) => json({ error: "content_rules_invalid", detail: problem }, 503);

// gateLanguage: the language scheduling checks in — the segment's first, as
// the panel's checks default to, else English.
export const gateLanguage = (c: Campaign): string => c.segment.languages[0] ?? "en";

// contentGate is what scheduling asks: null to go on, or the refusal —
// 503 content_rules_invalid for a configuration that cannot be honoured,
// 409 content_check_failed with the findings. Call it after contentProblem
// (the renderer refuses unknown variables).
export function contentGate(env: Env, c: Campaign, site: Site): Response | null {
  const { rules, problem } = rulesConfig(env);
  if (problem) return invalid(problem);
  if (!summaryOf(rules, null).active) return null;
  const findings = contentFindings(env, c, site, gateLanguage(c), rules);
  if (!findings.length) return null;
  const detail = `${findings.length} finding${findings.length === 1 ? "" : "s"} against the content rules`;
  return json({ error: "content_check_failed", detail, findings }, 409);
}

// GET {base}/api/campaigns/:id/content-check?language= → {findings, rules}.
// Read-only: the campaign is not changed and nothing is stored. `rules` is
// counts and switches; the phrase list is never returned, only the phrases
// found in this campaign.
export async function campaignContentCheck({ env, params, url }: RouteContext): Promise<Response> {
  const language = url.searchParams.get("language") ?? "en";
  if (!LANGUAGE.test(language)) return json({ error: "invalid_input", detail: "language must be a language code" }, 400);
  const c = await loadCampaign(env, params.id);
  if (!c) return json({ error: "not_found" }, 404);
  const { rules, problem } = rulesConfig(env);
  if (problem) return invalid(problem);
  const summary = summaryOf(rules, null);
  if (!summary.active) return json({ findings: [], rules: summary });
  const site = siteOf(env);
  if (!site) return json({ error: "no_site_url", detail: "set NEWSLETTER_SITE_URL to the site's https:// origin (where the newsletter template runs)" }, 503);
  const unknown = unknownVariables(c.subject, c.body, c.textOverride);
  if (unknown.length) return json({ error: "unknown_variable", detail: unknown.map((n) => `{{${n}}}`).join(", ") }, 400);
  return json({ findings: contentFindings(env, c, site, language, rules), rules: summary });
}
