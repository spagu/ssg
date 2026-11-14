// The route registry: one line per route, paths relative to the base path.
// Later features append here — e.g.
//   { method: "GET", path: "/api/campaigns/:id", handler: getCampaign },
//   { method: "POST", path: "/hooks/resend", handler: resendHook },
// Anything under /api/ is behind admin auth, its permission and the CSRF header
// rule; anything under /hooks/ is public (permission PUBLIC) and must verify
// its own signature (see router.ts). A state-changing /api/ route names its
// audit action, or `audit: false` when it changes nothing (a preview).

import type { Route } from "./router";
import { PUBLIC, SESSION } from "./permissions";
import { listAudit } from "./audit";
import { me } from "./me";
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
import { moduleManifest } from "./module";
import { moduleSummary } from "./module-summary";

// GO-137: who may call what. One permission per level of trust (module.json);
// a route doing two things needs both (the suppression import writes
// suppressions in bulk: import AND suppress). Every route states its need
// deliberately — test/route-registry.test.ts refuses one without.
const READ = "newsletter:read";
const EDIT = "newsletter:edit";
const SEND = "newsletter:send";
const IMPORT = "newsletter:import";
const EXPORT = "newsletter:export";
const ERASE = "newsletter:erase";
const SUPPRESS = "newsletter:suppress";
const SETTINGS = "newsletter:settings";
const IMPORT_SUPPRESS = [IMPORT, SUPPRESS];

export const ROUTES: Route[] = [
  { method: "GET", path: "/api/health", handler: health, needsSubscribers: false, permission: READ },
  // GO-124: subscribers admin API
  { method: "GET", path: "/api/subscribers", handler: listSubscribers, permission: READ },
  { method: "GET", path: "/api/subscribers.csv", handler: exportSubscribersCSV, permission: EXPORT },
  { method: "GET", path: "/api/subscribers/:id", handler: getSubscriber, permission: READ },
  { method: "PATCH", path: "/api/subscribers/:id", handler: patchSubscriber, permission: EDIT, audit: "subscriber.edit" },
  { method: "DELETE", path: "/api/subscribers/:id", handler: eraseSubscriber, permission: ERASE, audit: "subscriber.erase" },
  { method: "POST", path: "/api/subscribers/:id/unsubscribe", handler: unsubscribeSubscriber, permission: EDIT, audit: "subscriber.unsubscribe" },
  { method: "GET", path: "/api/subscribers/:id/export", handler: exportSubscriber, permission: EXPORT },
  { method: "POST", path: "/api/import/preview", handler: importPreview, permission: IMPORT, audit: false },
  { method: "POST", path: "/api/import", handler: importRun, permission: IMPORT, audit: "subscribers.import" },
  { method: "GET", path: "/api/stats/subscribers", handler: subscriberStats, permission: READ },
  // end GO-124
  // GO-125: campaigns
  { method: "GET", path: "/api/campaigns", handler: listCampaigns, permission: READ },
  { method: "POST", path: "/api/campaigns", handler: createCampaign, permission: EDIT, audit: "campaign.create" },
  { method: "GET", path: "/api/campaigns/:id", handler: getCampaign, permission: READ },
  { method: "PATCH", path: "/api/campaigns/:id", handler: updateCampaign, permission: EDIT, audit: "campaign.edit" },
  { method: "DELETE", path: "/api/campaigns/:id", handler: deleteCampaign, permission: EDIT, audit: "campaign.delete" },
  { method: "POST", path: "/api/campaigns/:id/duplicate", handler: duplicateCampaign, permission: EDIT, audit: "campaign.duplicate" },
  { method: "POST", path: "/api/campaigns/:id/preview", handler: previewCampaign, permission: READ, audit: false },
  { method: "POST", path: "/api/campaigns/:id/test", handler: testCampaign, permission: SEND, audit: "campaign.test" },
  { method: "POST", path: "/api/campaigns/:id/schedule", handler: scheduleCampaign, permission: SEND, audit: "campaign.schedule" },
  { method: "POST", path: "/api/campaigns/:id/unschedule", handler: unscheduleCampaign, permission: SEND, audit: "campaign.unschedule" },
  { method: "POST", path: "/api/campaigns/:id/pause", handler: pauseCampaign, permission: SEND, audit: "campaign.pause" },
  { method: "POST", path: "/api/campaigns/:id/resume", handler: resumeCampaign, permission: SEND, audit: "campaign.resume" },
  { method: "POST", path: "/api/campaigns/:id/cancel", handler: cancelCampaign, permission: SEND, audit: "campaign.cancel" },
  // end GO-125
  // GO-126: send engine
  { method: "GET", path: "/api/engine/status", handler: engineStatus, permission: READ },
  { method: "GET", path: "/api/campaigns/:id/progress", handler: campaignProgress, permission: READ },
  // end GO-126
  // Uncertain sends: in flight when a consumer died; the admin re-sends or dismisses
  { method: "GET", path: "/api/campaigns/:id/uncertain", handler: listUncertain, permission: READ },
  { method: "POST", path: "/api/campaigns/:id/uncertain/resend", handler: resendUncertain, permission: SEND, audit: "campaign.uncertain_resend" },
  { method: "POST", path: "/api/campaigns/:id/uncertain/dismiss", handler: dismissUncertain, permission: SEND, audit: "campaign.uncertain_dismiss" },
  // end uncertain sends
  // GO-130: campaign draft from a post; confirmation mail for pending subscribers
  { method: "GET", path: "/api/feed/posts", handler: listFeedPosts, permission: READ },
  { method: "POST", path: "/api/campaigns/from-post", handler: campaignFromPost, permission: EDIT, audit: "campaign.from_post" },
  { method: "POST", path: "/api/subscribers/confirmations", handler: sendConfirmations, permission: SEND, audit: "subscribers.confirmations" },
  { method: "POST", path: "/api/subscribers/:id/confirmation", handler: resendConfirmation, permission: SEND, audit: "subscriber.confirmation" },
  // end GO-130
  // GO-129: campaign statistics; the click redirect is public (no person in it)
  { method: "GET", path: "/api/campaigns/:id/stats", handler: campaignStats, permission: READ },
  { method: "GET", path: "/api/stats/campaigns", handler: campaignsStats, permission: READ },
  { method: "GET", path: "/hooks/c/:campaign/:link", handler: clickRedirect, needsSubscribers: false, permission: PUBLIC },
  { method: "HEAD", path: "/hooks/c/:campaign/:link", handler: clickRedirect, needsSubscribers: false, permission: PUBLIC },
  // end GO-129
  // GO-127: bounces, complaints, suppression list; the hooks are public and verify a signature
  { method: "GET", path: "/api/suppressions", handler: listSuppressions, permission: READ },
  { method: "POST", path: "/api/suppressions", handler: addSuppression, permission: SUPPRESS, audit: "suppression.add" },
  { method: "POST", path: "/api/suppressions/import/preview", handler: suppressionImportPreview, permission: IMPORT_SUPPRESS, audit: false },
  { method: "POST", path: "/api/suppressions/import", handler: suppressionImportRun, permission: IMPORT_SUPPRESS, audit: "suppressions.import" },
  { method: "DELETE", path: "/api/suppressions/:hash", handler: deleteSuppression, permission: SUPPRESS, audit: "suppression.lift" },
  { method: "GET", path: "/api/stats/suppressions", handler: suppressionStats, permission: READ },
  { method: "POST", path: "/hooks/resend", handler: resendHook, permission: PUBLIC },
  { method: "POST", path: "/hooks/mailchannels", handler: mailchannelsHook, permission: PUBLIC },
  // end GO-127
  // GO-128: the panel's read-only settings view and its SMTP connection test
  { method: "GET", path: "/api/settings", handler: getSettings, needsSubscribers: false, permission: READ },
  { method: "POST", path: "/api/settings/smtp-test", handler: (c) => smtpTest(c), needsSubscribers: false, permission: SETTINGS, audit: "settings.smtp_test" },
  // end GO-128
  // GO-143: the module manifest (module.json)
  { method: "GET", path: "/api/module", handler: moduleManifest, needsSubscribers: false, permission: READ },
  // end GO-143
  // GO-148: the module summary — counts, what waits for the admin, configuration problems
  { method: "GET", path: "/api/module/summary", handler: moduleSummary, needsSubscribers: false, permission: READ },
  // end GO-148
  // GO-137: who is calling, and the audit trail
  { method: "GET", path: "/api/me", handler: me, needsSubscribers: false, permission: SESSION },
  { method: "GET", path: "/api/audit", handler: listAudit, needsSubscribers: false, permission: SETTINGS },
  // end GO-137
];
