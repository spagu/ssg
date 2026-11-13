// The route registry: one line per route, paths relative to the base path.
// Later features append here — e.g.
//   { method: "GET", path: "/api/campaigns/:id", handler: getCampaign },
//   { method: "POST", path: "/hooks/resend", handler: resendHook },
// Anything under /api/ is behind admin auth and the CSRF header rule; anything
// under /hooks/ is public and must verify its own signature (see router.ts).

import type { Route } from "./router";
import { health } from "./health";
import { getSubscriber, listSubscribers } from "./subscribers";
import { eraseSubscriber, patchSubscriber, unsubscribeSubscriber } from "./subscriber-edit";
import { exportSubscriber, exportSubscribersCSV } from "./subscriber-export";
import { subscriberStats } from "./subscriber-stats";
import { importPreview, importRun } from "./import";
import { createCampaign, deleteCampaign, duplicateCampaign, getCampaign, listCampaigns, updateCampaign } from "./campaign-crud";
import {
  cancelCampaign, pauseCampaign, previewCampaign, resumeCampaign, scheduleCampaign, testCampaign, unscheduleCampaign,
} from "./campaign-actions";
import { campaignProgress, engineStatus } from "./engine-status";
import { dismissUncertain, listUncertain, resendUncertain } from "./engine-uncertain";
import { campaignFromPost, listFeedPosts } from "./feed";
import { resendConfirmation, sendConfirmations } from "./confirm";
import { campaignStats, campaignsStats } from "./stats-campaign";
import { clickRedirect } from "./click-redirect";
import { mailchannelsHook, resendHook } from "./hook-routes";
import { addSuppression, deleteSuppression, listSuppressions } from "./suppression-admin";
import { suppressionImportPreview, suppressionImportRun } from "./suppression-import";
import { suppressionStats } from "./suppression-stats";
import { getSettings, smtpTest } from "./settings";

export const ROUTES: Route[] = [
  { method: "GET", path: "/api/health", handler: health, needsSubscribers: false },
  // GO-124: subscribers admin API
  { method: "GET", path: "/api/subscribers", handler: listSubscribers },
  { method: "GET", path: "/api/subscribers.csv", handler: exportSubscribersCSV },
  { method: "GET", path: "/api/subscribers/:id", handler: getSubscriber },
  { method: "PATCH", path: "/api/subscribers/:id", handler: patchSubscriber },
  { method: "DELETE", path: "/api/subscribers/:id", handler: eraseSubscriber },
  { method: "POST", path: "/api/subscribers/:id/unsubscribe", handler: unsubscribeSubscriber },
  { method: "GET", path: "/api/subscribers/:id/export", handler: exportSubscriber },
  { method: "POST", path: "/api/import/preview", handler: importPreview },
  { method: "POST", path: "/api/import", handler: importRun },
  { method: "GET", path: "/api/stats/subscribers", handler: subscriberStats },
  // end GO-124
  // GO-125: campaigns
  { method: "GET", path: "/api/campaigns", handler: listCampaigns },
  { method: "POST", path: "/api/campaigns", handler: createCampaign },
  { method: "GET", path: "/api/campaigns/:id", handler: getCampaign },
  { method: "PATCH", path: "/api/campaigns/:id", handler: updateCampaign },
  { method: "DELETE", path: "/api/campaigns/:id", handler: deleteCampaign },
  { method: "POST", path: "/api/campaigns/:id/duplicate", handler: duplicateCampaign },
  { method: "POST", path: "/api/campaigns/:id/preview", handler: previewCampaign },
  { method: "POST", path: "/api/campaigns/:id/test", handler: testCampaign },
  { method: "POST", path: "/api/campaigns/:id/schedule", handler: scheduleCampaign },
  { method: "POST", path: "/api/campaigns/:id/unschedule", handler: unscheduleCampaign },
  { method: "POST", path: "/api/campaigns/:id/pause", handler: pauseCampaign },
  { method: "POST", path: "/api/campaigns/:id/resume", handler: resumeCampaign },
  { method: "POST", path: "/api/campaigns/:id/cancel", handler: cancelCampaign },
  // end GO-125
  // GO-126: send engine
  { method: "GET", path: "/api/engine/status", handler: engineStatus },
  { method: "GET", path: "/api/campaigns/:id/progress", handler: campaignProgress },
  // end GO-126
  // Uncertain sends: in flight when a consumer died; the admin re-sends or dismisses
  { method: "GET", path: "/api/campaigns/:id/uncertain", handler: listUncertain },
  { method: "POST", path: "/api/campaigns/:id/uncertain/resend", handler: resendUncertain },
  { method: "POST", path: "/api/campaigns/:id/uncertain/dismiss", handler: dismissUncertain },
  // end uncertain sends
  // GO-130: campaign draft from a post; confirmation mail for pending subscribers
  { method: "GET", path: "/api/feed/posts", handler: listFeedPosts },
  { method: "POST", path: "/api/campaigns/from-post", handler: campaignFromPost },
  { method: "POST", path: "/api/subscribers/confirmations", handler: sendConfirmations },
  { method: "POST", path: "/api/subscribers/:id/confirmation", handler: resendConfirmation },
  // end GO-130
  // GO-129: campaign statistics; the click redirect is public (no person in it)
  { method: "GET", path: "/api/campaigns/:id/stats", handler: campaignStats },
  { method: "GET", path: "/api/stats/campaigns", handler: campaignsStats },
  { method: "GET", path: "/hooks/c/:campaign/:link", handler: clickRedirect, needsSubscribers: false },
  { method: "HEAD", path: "/hooks/c/:campaign/:link", handler: clickRedirect, needsSubscribers: false },
  // end GO-129
  // GO-127: bounces, complaints, suppression list; the hooks are public and verify a signature
  { method: "GET", path: "/api/suppressions", handler: listSuppressions },
  { method: "POST", path: "/api/suppressions", handler: addSuppression },
  { method: "POST", path: "/api/suppressions/import/preview", handler: suppressionImportPreview },
  { method: "POST", path: "/api/suppressions/import", handler: suppressionImportRun },
  { method: "DELETE", path: "/api/suppressions/:hash", handler: deleteSuppression },
  { method: "GET", path: "/api/stats/suppressions", handler: suppressionStats },
  { method: "POST", path: "/hooks/resend", handler: resendHook },
  { method: "POST", path: "/hooks/mailchannels", handler: mailchannelsHook },
  // end GO-127
  // GO-128: the panel's read-only settings view and its SMTP connection test
  { method: "GET", path: "/api/settings", handler: getSettings, needsSubscribers: false },
  { method: "POST", path: "/api/settings/smtp-test", handler: (c) => smtpTest(c), needsSubscribers: false },
  // end GO-128
];
