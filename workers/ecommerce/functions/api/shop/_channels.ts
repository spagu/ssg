// Where an order came from (GO-165): the storefront's own checkout (`web`), an
// AI agent platform through ACP (`acp`, GO-163) or Google's agentic checkout
// through UCP (`ucp`, GO-167). The column is `orders.channel` (migration 5,
// default 'web'). One list, so the order filter and the stats agree on what a
// channel is; a leading underscore keeps the file out of the route table.

/** Every channel, in the order the panel lists them: the shop's own first. */
export const CHANNELS = ["web", "acp", "ucp"] as const;

export type Channel = (typeof CHANNELS)[number];

/** isChannel: `value` names a channel exactly — no case folding, so a filter
 *  value is either one of these or refused. */
export function isChannel(value: string): value is Channel {
  return (CHANNELS as readonly string[]).includes(value);
}

/** channelRank: where a channel sorts in a report — the known ones in their
 *  order, anything else (a channel a later version adds) after them. */
export function channelRank(value: string): number {
  const at = (CHANNELS as readonly string[]).indexOf(value);
  return at === -1 ? CHANNELS.length : at;
}
