// Bounces and complaints per campaign (GO-129) — only what the tables can
// honestly say.
//
// A permanent refusal while sending is a `failed` row in campaign_sends and is
// already counted in recipients.failed. A bounce or complaint reported AFTER
// delivery ends up in `suppressions`, which is keyed by address hash and holds
// no campaign id, so it cannot be credited to a campaign without guessing.
// Rather than guess (e.g. by time, as unsubscribes are), these figures are
// null and `detail` says why. When bounce events carry the campaign they came
// from, this is the one place to change.

export interface CampaignBounces {
  hard: number | null;
  soft: number | null;
  complaints: number | null;
  detail: string;
}

export const BOUNCES: CampaignBounces = {
  hard: null,
  soft: null,
  complaints: null,
  detail: "bounces and complaints reported after delivery are not linked to a campaign; permanent refusals at send time are counted in recipients.failed",
};
