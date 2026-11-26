// Webhook attribution and health (#460), schema version 9. Counts only: no
// table here holds an address.
//   campaign_bounces  hard / soft / complaint reports attributed to a campaign
//                     exactly — the send engine's own refusals (source smtp)
//                     and webhook events that name the campaign (bounce-
//                     attribution.ts) — one row per (campaign, source, kind)
//   hook_breaker      the webhook side of the bounce breaker (hook-breaker.ts):
//                     hard bounces reported while the campaign was sending since
//                     its last trip, and its sent count at that trip
//   hook_activity     verified webhook events per UTC hour, provider and kind,
//                     with the time of the latest; rows older than
//                     ACTIVITY_RETENTION_MS are pruned on write (hook-activity.ts)
export const HOOK_TABLES = [
  `CREATE TABLE IF NOT EXISTS campaign_bounces (
     campaign_id TEXT NOT NULL,
     source      TEXT NOT NULL,
     kind        TEXT NOT NULL,
     count       INTEGER NOT NULL DEFAULT 0,
     updated_at  TEXT NOT NULL,
     PRIMARY KEY (campaign_id, source, kind)
   )`,
  `CREATE TABLE IF NOT EXISTS hook_breaker (
     campaign_id TEXT PRIMARY KEY,
     hard        INTEGER NOT NULL DEFAULT 0,
     sent_base   INTEGER NOT NULL DEFAULT 0,
     updated_at  TEXT NOT NULL
   )`,
  `CREATE TABLE IF NOT EXISTS hook_activity (
     hour     TEXT NOT NULL,
     provider TEXT NOT NULL,
     kind     TEXT NOT NULL,
     count    INTEGER NOT NULL DEFAULT 0,
     last_at  TEXT NOT NULL,
     PRIMARY KEY (hour, provider, kind)
   )`,
];
