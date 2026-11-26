// Bounces and complaints per campaign (GO-129, #460) — only what the tables
// can honestly say, never by guessing (e.g. by time, as unsubscribes are).
//
// campaign_bounces holds the reports attributed exactly (bounce-attribution.ts):
// refusals while sending and webhook events that name the campaign. Reports
// that name none — MailChannels webhooks, messages sent before attribution
// existed — reach `suppressions` only, keyed by address, and are not counted
// here. When the transport attributes nothing and nothing was attributed,
// the figures stay null and `detail` says why.

import type { Env } from "./lib";
import { transportOf } from "./mail/_transport";

export interface CampaignBounces {
  hard: number | null;
  soft: number | null;
  complaints: number | null;
  attribution: "exact" | "none";
  detail: string;
}

// The transports whose reports name the campaign. SMTP: the engine's own
// refusals; Resend: the CAMPAIGN_TAG tag every campaign message carries.
export const ATTRIBUTING_TRANSPORTS: ReadonlySet<string> = new Set(["smtp", "resend"]);

export const EXACT_DETAIL =
  "hard bounces, soft bounces and complaints attributed to this campaign exactly: refusals while sending and webhook events " +
  "that name the campaign; reports that name none (MailChannels webhooks, messages sent before attribution) are not counted here; " +
  "permanent refusals at send time are also in recipients.failed";

export const NONE_DETAIL =
  "the configured transport reports nothing that names a campaign, so bounces and complaints reported after delivery are not linked " +
  "to one (they are on the suppression list); permanent refusals at send time are counted in recipients.failed";

export const BOUNCES: CampaignBounces = { hard: null, soft: null, complaints: null, attribution: "none", detail: NONE_DETAIL };

// campaignBounces sums campaign_bounces over every source for one campaign.
export async function campaignBounces(env: Env, campaignId: string): Promise<CampaignBounces> {
  const { results } = await env.NEWSLETTER_DB.prepare("SELECT kind, SUM(count) AS n FROM campaign_bounces WHERE campaign_id = ? GROUP BY kind")
    .bind(campaignId)
    .all<{ kind: string; n: number }>();
  if (!results.length && !ATTRIBUTING_TRANSPORTS.has(transportOf(env) ?? "")) return BOUNCES;
  const n = (kind: string) => Number(results.find((r) => r.kind === kind)?.n ?? 0);
  return { hard: n("hard"), soft: n("soft"), complaints: n("complaint"), attribution: "exact", detail: EXACT_DETAIL };
}
