// Exact attribution of bounces and complaints to a campaign (#460). Never by
// address and time: a report counts for a campaign only when it names it.
//
//   smtp          a refusal while sending is attributed by the engine itself:
//                 it knows which campaign's message was refused
//   resend        the webhook's data.tags[CAMPAIGN_TAG] (Resend returns the
//                 tags a message was sent with in every event about it)
//   mailchannels  nothing the webhook returns names the campaign: unattributed
//
// A tag passed the webhook signature, so only the provider (or whoever holds
// the webhook secret) could have written it; it is still checked for the shape
// of a campaign id and against the campaigns table before it counts.

import type { BounceKind } from "./bounce-classify";
import type { Env } from "./lib";

// The tag name a campaign message carries its campaign id under.
export const CAMPAIGN_TAG = "newsletter_campaign";

const CAMPAIGN_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

type Obj = Record<string, unknown>;
const obj = (v: unknown): Obj => (v && typeof v === "object" && !Array.isArray(v) ? (v as Obj) : {});

// resendCampaign reads the campaign id from a Resend payload {data: {tags}},
// or null when there is none in the shape of a campaign id.
export function resendCampaign(payload: unknown): string | null {
  const tag = obj(obj(obj(payload).data).tags)[CAMPAIGN_TAG];
  return typeof tag === "string" && CAMPAIGN_ID.test(tag) ? tag : null;
}

// knownCampaigns keeps the ids that name an existing campaign: one statement
// for a whole webhook request.
export async function knownCampaigns(env: Env, ids: (string | null)[]): Promise<Set<string>> {
  const wanted = [...new Set(ids.filter((id): id is string => id !== null))];
  if (!wanted.length) return new Set();
  const { results } = await env.NEWSLETTER_DB.prepare("SELECT id FROM campaigns WHERE id IN (SELECT value FROM json_each(?))")
    .bind(JSON.stringify(wanted))
    .all<{ id: string }>();
  return new Set(results.map((r) => r.id));
}

// ATTRIBUTED_KINDS: what campaign_bounces counts; `delivered` is not a bounce.
export const ATTRIBUTED_KINDS: ReadonlySet<BounceKind> = new Set(["hard", "soft", "complaint"]);

// countStatement adds one report to the campaign's count for (source, kind).
export const countStatement = (env: Env, campaignId: string, source: string, kind: BounceKind, at: string): D1PreparedStatement =>
  env.NEWSLETTER_DB.prepare(
    `INSERT INTO campaign_bounces (campaign_id, source, kind, count, updated_at) VALUES (?, ?, ?, 1, ?)
     ON CONFLICT(campaign_id, source, kind) DO UPDATE SET count = campaign_bounces.count + 1, updated_at = excluded.updated_at`,
  ).bind(campaignId, source, kind, at);
