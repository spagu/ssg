// What a webhook request does once its signature checked out (#460): record
// that it arrived (hook-activity.ts), keep the campaign ids that name a real
// campaign (bounce-attribution.ts), apply the bounces (bounce-store.ts), then
// let the webhook bounce breaker (hook-breaker.ts) see the new hard bounces.

import { knownCampaigns } from "./bounce-attribution";
import type { Classified } from "./bounce-classify";
import { applyReport } from "./bounce-store";
import type { Applied, Bounce } from "./bounce-store";
import { recordActivity } from "./hook-activity";
import { feedBreaker } from "./hook-breaker";
import type { Env } from "./lib";

// One provider event: its classification (null = nothing to do to an
// address), its dedup id, and the campaign id it names, unchecked.
export interface HookEvent {
  classified: Classified | null;
  id: string;
  campaign: string | null;
}

// toBounce: `delivered` events carry no id — resetting a counter twice is
// harmless, and recording one row per delivered message would cost a write
// per mail sent.
const toBounce = (c: Classified, id: string, source: string, campaignId: string | null): Bounce => ({
  id: c.kind === "delivered" ? null : id,
  source,
  ...c,
  campaignId,
});

export async function settle(env: Env, provider: string, events: HookEvent[]): Promise<Applied> {
  await recordActivity(env, provider, events.map((e) => e.classified?.kind ?? "other"));
  const known = await knownCampaigns(env, events.map((e) => e.campaign));
  const bounces: Bounce[] = [];
  for (const e of events) {
    if (!e.classified) continue;
    const campaignId = e.campaign !== null && known.has(e.campaign) ? e.campaign : null;
    bounces.push(toBounce(e.classified, e.id, provider, campaignId));
  }
  const { applied, duplicate, fresh } = await applyReport(env, provider, bounces);
  await feedBreaker(env, fresh);
  return { applied, duplicate };
}
