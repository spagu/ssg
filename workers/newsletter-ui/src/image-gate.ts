// The image gate (#448): with NEWSLETTER_IMAGES_REQUIRE set, scheduling a
// campaign runs the pre-send image check (src/image-check.ts) on the mail as it
// would go out and refuses when an image breaks an enforced rule:
//
//   409 {error: "image_check_failed", detail, items, rules}
//       items: the failing images in the shape of GET /api/campaigns/:id/images,
//       each with only its enforced problems (`problems` and `blocking` alike);
//       rules: the enforced rules.
//   503 {error: "image_rules_unusable", detail}  the rules cannot be applied
//       (src/image-gate-rules.ts): fail closed, never a schedule unchecked.
//
// Only POST /api/campaigns/:id/schedule is gated. It is the one move that
// commits a draft's content to the list: only drafts are editable, so resume
// (paused → sending) and the engine's scheduled → sending carry content that
// was already through this gate, and a gate there could strand a half-sent
// campaign on a flaky image host. A test send and saving a draft are never
// gated: the author must be able to look at the mail while fixing it.
//
// The check takes seconds when `load` probes the network. It runs before the
// state moves, and the move is guarded by the draft's updated_at
// (unchangedSince below): an edit saved during the check makes the schedule
// fail with 409 campaign_changed rather than send content nobody checked.

import { previewMessage } from "./campaign-actions";
import { loadCampaign } from "./campaign-model";
import type { Campaign } from "./campaign-model";
import { MAX_PROBED_IMAGES, checkImages } from "./image-check";
import { blockingOf, imageRules } from "./image-gate-rules";
import type { Fetcher } from "./image-probe";
import { json } from "./lib";
import type { Env } from "./lib";
import { siteOf } from "./render";

// gateLanguage: the language the mail is checked in — the campaign's first
// segment language, else English; the panel's check uses the same rule
// (public/js/models/image-check.js checkLanguage).
export const gateLanguage = (c: Campaign): string => c.segment.languages[0] ?? "en";

// imageGate answers the refusal, or null when scheduling may go on. Without
// the switch it returns null at once: no render, no request, no query.
export async function imageGate(env: Env, c: Campaign, fetcher?: Fetcher): Promise<Response | null> {
  const { rules, problem } = imageRules(env);
  if (problem) return json({ error: "image_rules_unusable", detail: problem }, 503);
  if (!rules.length) return null;
  const site = siteOf(env);
  if (!site) return json({ error: "no_site_url", detail: "set NEWSLETTER_SITE_URL to the site's https:// origin (where the newsletter template runs)" }, 503);
  const html = previewMessage(c, gateLanguage(c), site).html;
  // Without `load` nothing is probed: alt and address need no network.
  const report = await checkImages(env, site, html, fetcher, rules.includes("load") ? MAX_PROBED_IMAGES : 0);
  const items = report.items
    .map((i) => {
      const blocking = blockingOf(i, rules);
      return { ...i, problems: blocking, blocking };
    })
    .filter((i) => i.blocking.length);
  if (!items.length) return null;
  const detail = `${items.length === 1 ? "1 image breaks" : `${items.length} images break`} the enforced rules (${rules.join(", ")})`;
  return json({ error: "image_check_failed", detail, items, rules }, 409);
}

// unchangedSince is the guard for the schedule's state move: the draft's
// updated_at while the gate is on, nothing (today's UPDATE) while it is off.
export const unchangedSince = (env: Env, c: Campaign): string | undefined => (imageRules(env).rules.length ? c.updatedAt : undefined);

// changedWhileChecked explains a lost move: 409 campaign_changed when the
// draft is still a draft but was saved after it was loaded; null otherwise
// (the caller answers invalid_transition, as without the gate).
export async function changedWhileChecked(env: Env, c: Campaign): Promise<Response | null> {
  if (unchangedSince(env, c) === undefined) return null;
  const now = await loadCampaign(env, c.id);
  return now && now.status === c.status && now.updatedAt !== c.updatedAt
    ? json({ error: "campaign_changed", detail: "the campaign was edited while its images were checked; check it and schedule it again" }, 409)
    : null;
}
